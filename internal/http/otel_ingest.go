package http

import (
	"encoding/json"
	"maps"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/karloscodes/cartridge"

	"lognorth/internal/apikeys"
	"lognorth/internal/events"
)

// OTLP JSON structures — only the fields we care about.
// See https://opentelemetry.io/docs/specs/otlp/#otlphttp-request

type otlpLogsRequest struct {
	ResourceLogs []otlpResourceLogs `json:"resourceLogs"`
}

type otlpResourceLogs struct {
	Resource  otlpResource    `json:"resource"`
	ScopeLogs []otlpScopeLogs `json:"scopeLogs"`
}

type otlpResource struct {
	Attributes []otlpKeyValue `json:"attributes"`
}

type otlpScopeLogs struct {
	LogRecords []otlpLogRecord `json:"logRecords"`
}

type otlpLogRecord struct {
	TimeUnixNano   string         `json:"timeUnixNano"`
	SeverityNumber int            `json:"severityNumber"`
	Body           otlpAnyValue   `json:"body"`
	Attributes     []otlpKeyValue `json:"attributes"`
	TraceID        string         `json:"traceId"`
}

type otlpKeyValue struct {
	Key   string       `json:"key"`
	Value otlpAnyValue `json:"value"`
}

type otlpAnyValue struct {
	StringValue *string     `json:"stringValue,omitempty"`
	IntValue    *string     `json:"intValue,omitempty"`
	BoolValue   *bool       `json:"boolValue,omitempty"`
	DoubleValue *float64    `json:"doubleValue,omitempty"`
	ArrayValue  *otlpArray  `json:"arrayValue,omitempty"`
	KvlistValue *otlpKVList `json:"kvlistValue,omitempty"`
}

type otlpArray struct {
	Values []otlpAnyValue `json:"values"`
}

type otlpKVList struct {
	Values []otlpKeyValue `json:"values"`
}

// unwrap extracts a plain Go value from an OTel typed value.
func (v otlpAnyValue) unwrap() any {
	switch {
	case v.StringValue != nil:
		return *v.StringValue
	case v.IntValue != nil:
		if n, err := strconv.ParseInt(*v.IntValue, 10, 64); err == nil {
			return n
		}
		return *v.IntValue
	case v.BoolValue != nil:
		return *v.BoolValue
	case v.DoubleValue != nil:
		return *v.DoubleValue
	case v.ArrayValue != nil:
		out := make([]any, len(v.ArrayValue.Values))
		for i, av := range v.ArrayValue.Values {
			out[i] = av.unwrap()
		}
		return out
	case v.KvlistValue != nil:
		out := make(map[string]any, len(v.KvlistValue.Values))
		for _, kv := range v.KvlistValue.Values {
			out[kv.Key] = kv.Value.unwrap()
		}
		return out
	default:
		return nil
	}
}

// flattenAttributes converts OTel KeyValue pairs into a plain map.
func flattenAttributes(attrs []otlpKeyValue) map[string]any {
	m := make(map[string]any, len(attrs))
	for _, kv := range attrs {
		m[kv.Key] = kv.Value.unwrap()
	}
	return m
}

// OtelIngestAction handles POST /api/v1/otel/logs
// Accepts OTLP JSON log exports and converts them to IngestedEvents.
func OtelIngestAction(ctx *cartridge.Context) error {
	db := ctx.DB()

	// Auth — same as regular ingest
	authHeader := ctx.Ctx.Get("Authorization")
	apiKey := strings.TrimPrefix(authHeader, "Bearer ")
	if apiKey == "" {
		return ctx.Ctx.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": "API key required",
		})
	}

	validatedKey, err := apikeys.ValidateAppKey(db, apiKey)
	if err != nil {
		return ctx.Ctx.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": "Invalid API key",
		})
	}

	// Parse OTLP JSON
	var req otlpLogsRequest
	if err := json.Unmarshal(ctx.Ctx.Body(), &req); err != nil {
		return ctx.Ctx.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid OTLP JSON",
		})
	}

	created := 0
	var errors []string

	for _, rl := range req.ResourceLogs {
		resourceAttrs := flattenAttributes(rl.Resource.Attributes)

		for _, sl := range rl.ScopeLogs {
			for _, lr := range sl.LogRecords {
				ingested, errMsg := convertLogRecord(lr, resourceAttrs, validatedKey.ID)
				if errMsg != "" {
					errors = append(errors, errMsg)
					continue
				}

				if err := events.CreateIngested(db, ingested); err != nil {
					continue
				}
				created++
			}
		}
	}

	response := fiber.Map{"created": created}
	if len(errors) > 0 {
		response["errors"] = errors
	}

	return ctx.Ctx.Status(fiber.StatusCreated).JSON(response)
}

// convertLogRecord transforms a single OTel log record into an IngestedEvent.
func convertLogRecord(lr otlpLogRecord, resourceAttrs map[string]any, apiKeyID uint) (*events.IngestedEvent, string) {
	// Extract message from body
	message := ""
	if lr.Body.StringValue != nil {
		message = *lr.Body.StringValue
	}
	if message == "" {
		return nil, "log record missing body.stringValue"
	}

	// Parse timestamp
	var timestamp time.Time
	if lr.TimeUnixNano != "" {
		if nanos, err := strconv.ParseInt(lr.TimeUnixNano, 10, 64); err == nil && nanos > 0 {
			timestamp = time.Unix(0, nanos)
		}
	}
	if timestamp.IsZero() {
		timestamp = time.Now()
	}

	// Build context: resource attributes first, then log attributes (log wins on conflict)
	context := make(map[string]any, len(resourceAttrs)+len(lr.Attributes))
	maps.Copy(context, resourceAttrs)
	maps.Copy(context, flattenAttributes(lr.Attributes))

	// Map OTel semantic conventions for errors
	mapErrorAttribute(context, "exception.message", "error")
	mapErrorAttribute(context, "error.message", "error")
	mapErrorAttribute(context, "exception.type", "error_class")
	mapErrorAttribute(context, "error.type", "error_class")

	// Determine is_error
	isError := lr.SeverityNumber >= 17 // OTel ERROR=17, FATAL=21

	// Also apply existing LogNorth error detection rules
	if !isError {
		if e, ok := context["error"]; ok && e != nil && e != "" {
			isError = true
		}
	}
	if !isError {
		if ec, ok := context["error_class"]; ok && ec != nil && ec != "" {
			isError = true
		}
	}
	if !isError {
		if status, ok := context["status"]; ok {
			switch v := status.(type) {
			case float64:
				isError = v >= 500
			case int64:
				isError = v >= 500
			}
		}
	}

	contextJSON, err := json.Marshal(context)
	if err != nil {
		contextJSON = []byte("{}")
	}

	// Decode traceId from hex if present
	traceID := lr.TraceID
	// OTel sends traceId as hex — keep as-is, it's already a string identifier
	// Strip all-zero trace IDs (means "no trace")
	if traceID != "" && isAllZeros(traceID) {
		traceID = ""
	}

	return &events.IngestedEvent{
		APIKeyID:  apiKeyID,
		Message:   message,
		TraceID:   traceID,
		IsError:   isError,
		Context:   string(contextJSON),
		Timestamp: timestamp,
	}, ""
}

// mapErrorAttribute copies an OTel semantic convention key to a LogNorth context key.
// Only copies if the source exists and the target doesn't (don't overwrite).
func mapErrorAttribute(context map[string]any, otelKey, lognorthKey string) {
	if v, ok := context[otelKey]; ok && v != nil && v != "" {
		if _, exists := context[lognorthKey]; !exists {
			context[lognorthKey] = v
		}
	}
}

func isAllZeros(s string) bool {
	for _, c := range s {
		if c != '0' {
			return false
		}
	}
	return len(s) > 0
}

