// Package http provides HTTP handlers for LogNorth
package http

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/karloscodes/cartridge"

	"lognorth/internal/apikeys"
	"lognorth/internal/events"
)

// IngestRequest represents an incoming event from an SDK
// See docs/EVENT_MODEL.md for the full specification
//
// Required fields:
//   - message: What happened (human readable)
//
// Optional fields:
//   - timestamp: ISO 8601 timestamp (defaults to now)
//   - duration_ms: How long it took
//   - trace_id: Correlation ID for grouping related events
//   - context: All metadata (arbitrary key-value)
//
// Note: is_error is derived automatically, not sent by client:
//   - context.error exists → is_error = true
//   - context.status >= 500 → is_error = true
type IngestRequest struct {
	Message    string         `json:"message"`
	Timestamp  string         `json:"timestamp,omitempty"`
	DurationMS int            `json:"duration_ms,omitempty"`
	TraceID    string         `json:"trace_id,omitempty"`
	Context    map[string]any `json:"context,omitempty"`

	// Error grouping (SDK extracts from Error object)
	// Legacy top-level fields — new SDKs send these inside context instead
	ErrorType     string `json:"error_type,omitempty"`     // e.g., "TypeError"
	ErrorLocation string `json:"error_location,omitempty"` // e.g., "checkout.ts:42"
}

// maxEventsPerRequest caps one request. The SDKs send batches of 10.
const maxEventsPerRequest = 1000

// IngestBatchRequest represents a batch of events
type IngestBatchRequest struct {
	Events []IngestRequest `json:"events"`
}

// Validate checks required fields and returns an error message if invalid
func (r *IngestRequest) Validate() error {
	if r.Message == "" {
		return fmt.Errorf("message is required")
	}
	return nil
}

// DeriveIsError determines if this is an error event
// Based on error_type, context.error_class, context.error, or context.status >= 500
func (r *IngestRequest) DeriveIsError() bool {
	// error_type is the legacy top-level field; everything else lives in context.
	return r.ErrorType != "" || events.IsErrorContext(r.Context)
}

// IngestAction handles POST /api/v1/events
// Events are written to the ingested_events table for async processing
func IngestAction(ctx *cartridge.Context) error {
	db := ctx.DB()

	// Extract API key from Authorization header
	authHeader := ctx.Ctx.Get("Authorization")
	apiKey := strings.TrimPrefix(authHeader, "Bearer ")
	if apiKey == "" {
		return ctx.Ctx.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": "API key required",
		})
	}

	// Validate API key (must be an app key, not agent key)
	validatedKey, err := apikeys.ValidateAppKey(db, apiKey)
	if err != nil {
		return ctx.Ctx.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": "Invalid API key",
		})
	}

	// Parse request body (single event or batch)
	body := ctx.Ctx.Body()

	var ingestEvents []IngestRequest

	// Try batch format first
	var batch IngestBatchRequest
	if err := json.Unmarshal(body, &batch); err == nil && len(batch.Events) > 0 {
		ingestEvents = batch.Events
	} else {
		// Try single event
		var single IngestRequest
		if err := json.Unmarshal(body, &single); err != nil {
			return ctx.Ctx.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": "Invalid request body",
			})
		}
		ingestEvents = []IngestRequest{single}
	}

	if len(ingestEvents) > maxEventsPerRequest {
		return ctx.Ctx.Status(fiber.StatusRequestEntityTooLarge).JSON(fiber.Map{
			"error": fmt.Sprintf("at most %d events per request", maxEventsPerRequest),
		})
	}

	// Validate everything first, then store the batch in one transaction.
	var toStore []*events.IngestedEvent
	errors := []string{}

	for i, req := range ingestEvents {
		// Validate required fields
		if err := req.Validate(); err != nil {
			errors = append(errors, fmt.Sprintf("event[%d]: %s", i, err.Error()))
			continue
		}

		// Drop known scanner/bot noise
		if reqPath, ok := req.Context["path"].(string); ok && events.ShouldIgnorePath(reqPath) {
			continue
		}

		// Parse timestamp
		var timestamp time.Time
		if req.Timestamp != "" {
			if t, err := time.Parse(time.RFC3339, req.Timestamp); err == nil {
				timestamp = t
			}
		}
		if timestamp.IsZero() {
			timestamp = time.Now()
		}

		// Merge legacy top-level error fields into context
		if req.ErrorType != "" {
			if req.Context == nil {
				req.Context = make(map[string]any)
			}
			req.Context["error_type"] = req.ErrorType
			req.Context["error_location"] = req.ErrorLocation
			// Also set error_class from error_type for backward compat
			if req.Context["error_class"] == nil {
				req.Context["error_class"] = req.ErrorType
			}
		}

		// Build context JSON
		contextJSON, err := json.Marshal(req.Context)
		if err != nil {
			contextJSON = []byte("{}")
		}

		toStore = append(toStore, &events.IngestedEvent{
			APIKeyID:   validatedKey.ID,
			Message:    req.Message,
			TraceID:    req.TraceID,
			DurationMS: req.DurationMS,
			IsError:    req.DeriveIsError(),
			Context:    string(contextJSON),
			Timestamp:  timestamp,
		})
	}

	// On a failed write, say so: SDKs retry on 5xx, and a 201 would drop the events.
	if err := events.CreateIngested(db, toStore); err != nil {
		ctx.Logger.Error("Failed to store events", "error", err)
		return ctx.Ctx.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"error": "Could not store events. Retry.",
		})
	}

	response := fiber.Map{"created": len(toStore)}
	if len(errors) > 0 {
		response["errors"] = errors
	}

	return ctx.Ctx.Status(fiber.StatusCreated).JSON(response)
}
