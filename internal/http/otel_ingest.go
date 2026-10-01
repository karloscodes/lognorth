package http

import (
	"encoding/hex"
	"encoding/json"
	"maps"
	"net/http"
	"strings"
	"time"

	"github.com/karloscodes/cartridge"
	"google.golang.org/protobuf/proto"

	collectorlogs "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	logspb "go.opentelemetry.io/proto/otlp/logs/v1"
	resourcepb "go.opentelemetry.io/proto/otlp/resource/v1"

	"lognorth/internal/apikeys"
	"lognorth/internal/events"
)

// OtelIngestAction handles POST /v1/logs
// Accepts OTLP HTTP/protobuf log exports (the default for all OTel SDKs).
func OtelIngestAction(ctx *cartridge.Context) error {
	db := ctx.DB()

	// Auth via Authorization header (set via OTEL_EXPORTER_OTLP_HEADERS)
	authHeader := ctx.Get("Authorization")
	apiKey := strings.TrimPrefix(authHeader, "Bearer ")
	if apiKey == "" {
		return ctx.Status(http.StatusUnauthorized).JSON(cartridge.Map{
			"error": "API key required",
		})
	}

	validatedKey, err := apikeys.ValidateAppKey(db, apiKey)
	if err != nil {
		return ctx.Status(http.StatusUnauthorized).JSON(cartridge.Map{
			"error": "Invalid API key",
		})
	}

	body, err := ingestBody(ctx)
	if err != nil {
		return rejectBody(ctx, err)
	}

	// Decode protobuf
	var req collectorlogs.ExportLogsServiceRequest
	if err := proto.Unmarshal(body, &req); err != nil {
		return ctx.Status(http.StatusBadRequest).JSON(cartridge.Map{
			"error": "Invalid OTLP protobuf",
		})
	}

	var toStore []*events.IngestedEvent

	for _, rl := range req.GetResourceLogs() {
		resourceAttrs := flattenProtoAttributes(rl.GetResource())

		for _, sl := range rl.GetScopeLogs() {
			for _, lr := range sl.GetLogRecords() {
				if ingested := convertProtoLogRecord(lr, resourceAttrs, validatedKey.ID); ingested != nil {
					toStore = append(toStore, ingested)
				}
			}
		}
	}

	// On a failed write, say so: OTel exporters retry on 503.
	if err := events.CreateIngested(db, toStore); err != nil {
		ctx.Logger.Error("Failed to store OTel events", "error", err)
		return ctx.Status(http.StatusServiceUnavailable).JSON(cartridge.Map{"error": "Could not store events. Retry."})
	}

	return ctx.Status(http.StatusOK).JSON(cartridge.Map{"created": len(toStore)})
}

// OtelSinkAction handles POST /v1/traces and /v1/metrics
// Accepts the request and discards it. Returns 200 so OTel SDKs don't error.
func OtelSinkAction(ctx *cartridge.Context) error {
	return ctx.Status(http.StatusOK).JSON(cartridge.Map{})
}

// convertProtoLogRecord transforms an OTLP protobuf LogRecord into an IngestedEvent.
func convertProtoLogRecord(lr *logspb.LogRecord, resourceAttrs map[string]any, apiKeyID uint) *events.IngestedEvent {
	// Extract message from body
	message := unwrapAnyValue(lr.GetBody())
	msgStr, ok := message.(string)
	if !ok || msgStr == "" {
		return nil
	}

	// Timestamp: prefer time_unix_nano, fall back to observed_time_unix_nano
	var timestamp time.Time
	if lr.GetTimeUnixNano() > 0 {
		timestamp = time.Unix(0, int64(lr.GetTimeUnixNano()))
	} else if lr.GetObservedTimeUnixNano() > 0 {
		timestamp = time.Unix(0, int64(lr.GetObservedTimeUnixNano()))
	} else {
		timestamp = time.Now()
	}

	// Build context: resource attributes first, then log attributes (log wins)
	context := make(map[string]any, len(resourceAttrs)+len(lr.GetAttributes()))
	maps.Copy(context, resourceAttrs)
	for _, kv := range lr.GetAttributes() {
		context[kv.GetKey()] = unwrapAnyValue(kv.GetValue())
	}

	// Map OTel semantic conventions for errors
	mapErrorAttribute(context, "exception.message", "error")
	mapErrorAttribute(context, "error.message", "error")
	mapErrorAttribute(context, "exception.type", "error_class")
	mapErrorAttribute(context, "error.type", "error_class")

	// Error detection: an ERROR severity or above, or the shared context rule
	isError := lr.GetSeverityNumber() >= logspb.SeverityNumber_SEVERITY_NUMBER_ERROR || events.IsErrorContext(context)

	contextJSON, err := json.Marshal(context)
	if err != nil {
		contextJSON = []byte("{}")
	}

	// TraceID: 16 bytes → hex string, strip all-zeros
	traceID := ""
	if len(lr.GetTraceId()) > 0 {
		traceID = hex.EncodeToString(lr.GetTraceId())
		if isAllZeros(traceID) {
			traceID = ""
		}
	}

	return &events.IngestedEvent{
		APIKeyID:  apiKeyID,
		Message:   msgStr,
		TraceID:   traceID,
		IsError:   isError,
		Context:   string(contextJSON),
		Timestamp: timestamp,
	}
}

// flattenProtoAttributes extracts resource attributes into a plain map.
func flattenProtoAttributes(resource *resourcepb.Resource) map[string]any {
	if resource == nil {
		return nil
	}
	m := make(map[string]any, len(resource.GetAttributes()))
	for _, kv := range resource.GetAttributes() {
		m[kv.GetKey()] = unwrapAnyValue(kv.GetValue())
	}
	return m
}

// unwrapAnyValue extracts a plain Go value from an OTel protobuf AnyValue.
func unwrapAnyValue(v *commonpb.AnyValue) any {
	if v == nil {
		return nil
	}
	switch val := v.GetValue().(type) {
	case *commonpb.AnyValue_StringValue:
		return val.StringValue
	case *commonpb.AnyValue_IntValue:
		return val.IntValue
	case *commonpb.AnyValue_BoolValue:
		return val.BoolValue
	case *commonpb.AnyValue_DoubleValue:
		return val.DoubleValue
	case *commonpb.AnyValue_ArrayValue:
		out := make([]any, len(val.ArrayValue.GetValues()))
		for i, av := range val.ArrayValue.GetValues() {
			out[i] = unwrapAnyValue(av)
		}
		return out
	case *commonpb.AnyValue_KvlistValue:
		out := make(map[string]any, len(val.KvlistValue.GetValues()))
		for _, kv := range val.KvlistValue.GetValues() {
			out[kv.GetKey()] = unwrapAnyValue(kv.GetValue())
		}
		return out
	case *commonpb.AnyValue_BytesValue:
		return val.BytesValue
	default:
		return nil
	}
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
