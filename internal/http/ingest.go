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
	// Check for error_type (top-level, legacy)
	if r.ErrorType != "" {
		return true
	}
	if r.Context == nil {
		return false
	}
	// Check for error_class in context (new SDKs)
	if ec, ok := r.Context["error_class"]; ok && ec != nil && ec != "" {
		return true
	}
	// Check for error field
	if err, ok := r.Context["error"]; ok && err != nil && err != "" {
		return true
	}
	// Check for status >= 500
	if status, ok := r.Context["status"]; ok {
		switch v := status.(type) {
		case float64:
			return v >= 500
		case int:
			return v >= 500
		}
	}
	return false
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

	// Validate API key
	validatedKey, err := apikeys.Validate(db, apiKey)
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

	// Store events in ingested_events table (fast write)
	created := 0
	errors := []string{}

	for i, req := range ingestEvents {
		// Validate required fields
		if err := req.Validate(); err != nil {
			errors = append(errors, fmt.Sprintf("event[%d]: %s", i, err.Error()))
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

		// Create ingested event (fast write, no processing)
		ingested := &events.IngestedEvent{
			APIKeyID:   validatedKey.ID,
			Message:    req.Message,
			TraceID:    req.TraceID,
			DurationMS: req.DurationMS,
			IsError:    req.DeriveIsError(),
			Context:    string(contextJSON),
			Timestamp:  timestamp,
		}

		if err := events.CreateIngested(db, ingested); err != nil {
			continue
		}

		created++
	}

	response := fiber.Map{"created": created}
	if len(errors) > 0 {
		response["errors"] = errors
	}

	return ctx.Ctx.Status(fiber.StatusCreated).JSON(response)
}

