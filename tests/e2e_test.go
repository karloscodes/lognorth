package tests

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/karloscodes/cartridge/sqlite"
	"gorm.io/gorm"

	"lognorth/internal/apikeys"
	"lognorth/internal/events"
)

func setupTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	dbPath := "test_" + t.Name() + ".db"
	t.Cleanup(func() {
		os.Remove(dbPath)
		os.Remove(dbPath + "-wal")
		os.Remove(dbPath + "-shm")
	})

	manager := sqlite.NewManager(sqlite.Config{
		Path:      dbPath,
		EnableWAL: true,
	})
	db, err := manager.Connect()
	if err != nil {
		t.Fatalf("Failed to connect to database: %v", err)
	}

	if err := apikeys.AutoMigrate(db); err != nil {
		t.Fatalf("Failed to migrate apikeys: %v", err)
	}
	if err := events.AutoMigrate(db); err != nil {
		t.Fatalf("Failed to migrate events: %v", err)
	}
	if err := events.AutoMigrateIngested(db); err != nil {
		t.Fatalf("Failed to migrate ingested_events: %v", err)
	}
	if err := events.AutoMigrateIssues(db); err != nil {
		t.Fatalf("Failed to migrate issues: %v", err)
	}

	return db
}

func TestAPIKeyCreation(t *testing.T) {
	db := setupTestDB(t)

	apiKey, fullKey, err := apikeys.Create(db, "Production")

	if err != nil {
		t.Fatalf("Failed to create API key: %v", err)
	}
	if apiKey.ID == 0 {
		t.Error("Expected API key ID to be set")
	}
	if fullKey == "" {
		t.Error("Expected full key to be returned")
	}
	if fullKey[:4] != "lgn-" {
		t.Errorf("Expected key prefix 'lgn-', got '%s'", fullKey[:4])
	}
}

func TestAPIKeyValidation(t *testing.T) {
	db := setupTestDB(t)

	_, fullKey, _ := apikeys.Create(db, "Production")

	validated, err := apikeys.Validate(db, fullKey)

	if err != nil {
		t.Fatalf("Failed to validate API key: %v", err)
	}
	if validated.Name != "Production" {
		t.Errorf("Expected name 'Production', got '%s'", validated.Name)
	}
}

func TestEventCreation(t *testing.T) {
	db := setupTestDB(t)

	event := &events.Event{
		Message:    "GET /api/users → 200",
		TraceID:    "req-123",
		DurationMS: 45,
		IsError:    false,
	}
	event.SetContext(&events.EventContext{
		Method: "GET",
		Path:   "/api/users",
		Status: 200,
	})
	err := events.Create(db, event)

	if err != nil {
		t.Fatalf("Failed to create event: %v", err)
	}
	if event.ID == 0 {
		t.Error("Expected event ID to be set")
	}
}

func TestEventListing(t *testing.T) {
	db := setupTestDB(t)

	for i := range 5 {
		event := &events.Event{
			Message:    "GET /api/test → 200",
			TraceID:    "req-" + string(rune('0'+i)),
			DurationMS: 10 + i,
			IsError:    false,
		}
		events.Create(db, event)
	}

	eventList, total, err := events.List(db, events.ListFilters{Limit: 10})

	if err != nil {
		t.Fatalf("Failed to list events: %v", err)
	}
	if total != 5 {
		t.Errorf("Expected 5 events, got %d", total)
	}
	if len(eventList) != 5 {
		t.Errorf("Expected 5 events returned, got %d", len(eventList))
	}
}

func TestIngestionAPI(t *testing.T) {
	db := setupTestDB(t)

	_, fullKey, _ := apikeys.Create(db, "Production")

	app := fiber.New()
	app.Post("/api/v1/events", func(c *fiber.Ctx) error {
		authHeader := c.Get("Authorization")
		if authHeader == "" {
			return c.Status(401).JSON(fiber.Map{"error": "API key required"})
		}
		apiKey := authHeader[7:] // Remove "Bearer "

		_, err := apikeys.Validate(db, apiKey)
		if err != nil {
			return c.Status(401).JSON(fiber.Map{"error": "Invalid API key"})
		}

		var req struct {
			Message    string         `json:"message"`
			TraceID    string         `json:"trace_id"`
			DurationMS int            `json:"duration_ms"`
			Context    map[string]any `json:"context"`
		}
		if err := c.BodyParser(&req); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
		}

		// Derive is_error from context
		isError := false
		if req.Context != nil {
			if _, ok := req.Context["error"]; ok {
				isError = true
			}
			if status, ok := req.Context["status"].(float64); ok && status >= 500 {
				isError = true
			}
		}

		contextJSON, _ := json.Marshal(req.Context)
		event := &events.Event{
			Message:    req.Message,
			TraceID:    req.TraceID,
			DurationMS: req.DurationMS,
			IsError:    isError,
			Context:    string(contextJSON),
		}
		if err := events.Create(db, event); err != nil {
			return c.Status(500).JSON(fiber.Map{"error": "Failed to create event"})
		}

		return c.Status(201).JSON(fiber.Map{"created": 1})
	})

	body := []byte(`{
		"message": "POST /api/orders → 201",
		"trace_id": "test-req-1",
		"duration_ms": 123,
		"context": {
			"method": "POST",
			"path": "/api/orders",
			"status": 201
		}
	}`)
	req := httptest.NewRequest("POST", "/api/v1/events", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+fullKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)

	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	if resp.StatusCode != 201 {
		respBody, _ := io.ReadAll(resp.Body)
		t.Fatalf("Expected status 201, got %d: %s", resp.StatusCode, string(respBody))
	}

	eventList, total, _ := events.List(db, events.ListFilters{Limit: 10})
	if total != 1 {
		t.Errorf("Expected 1 event in database, got %d", total)
	}
	if eventList[0].Message != "POST /api/orders → 201" {
		t.Errorf("Expected message 'POST /api/orders → 201', got '%s'", eventList[0].Message)
	}
}

func TestIngestionAPIWithoutAuth(t *testing.T) {
	app := fiber.New()
	app.Post("/api/v1/events", func(c *fiber.Ctx) error {
		authHeader := c.Get("Authorization")
		if authHeader == "" {
			return c.Status(401).JSON(fiber.Map{"error": "API key required"})
		}
		return c.SendStatus(200)
	})

	body := []byte(`{"message": "test"}`)
	req := httptest.NewRequest("POST", "/api/v1/events", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, _ := app.Test(req)

	if resp.StatusCode != 401 {
		t.Errorf("Expected status 401, got %d", resp.StatusCode)
	}
}

func TestIngestionBatch(t *testing.T) {
	db := setupTestDB(t)

	_, fullKey, _ := apikeys.Create(db, "Production")

	app := fiber.New()
	app.Post("/api/v1/events", func(c *fiber.Ctx) error {
		authHeader := c.Get("Authorization")
		apiKey := authHeader[7:]

		_, err := apikeys.Validate(db, apiKey)
		if err != nil {
			return c.Status(401).JSON(fiber.Map{"error": "Invalid API key"})
		}

		var batch struct {
			Events []struct {
				Message    string         `json:"message"`
				TraceID    string         `json:"trace_id"`
				DurationMS int            `json:"duration_ms"`
				Context    map[string]any `json:"context"`
			} `json:"events"`
		}
		if err := c.BodyParser(&batch); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
		}

		created := 0
		for _, e := range batch.Events {
			isError := false
			if e.Context != nil {
				if _, ok := e.Context["error"]; ok {
					isError = true
				}
				if status, ok := e.Context["status"].(float64); ok && status >= 500 {
					isError = true
				}
			}
			contextJSON, _ := json.Marshal(e.Context)
			event := &events.Event{
				Message:    e.Message,
				TraceID:    e.TraceID,
				DurationMS: e.DurationMS,
				IsError:    isError,
				Context:    string(contextJSON),
			}
			if err := events.Create(db, event); err == nil {
				created++
			}
		}

		return c.Status(201).JSON(fiber.Map{"created": created})
	})

	body := []byte(`{
		"events": [
			{"message": "GET /a → 200", "trace_id": "req-1", "duration_ms": 10, "context": {"status": 200}},
			{"message": "POST /b → 500", "trace_id": "req-2", "duration_ms": 20, "context": {"status": 500, "error": "Server error"}},
			{"message": "PUT /c → 400", "trace_id": "req-3", "duration_ms": 30, "context": {"status": 400}}
		]
	}`)
	req := httptest.NewRequest("POST", "/api/v1/events", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+fullKey)
	req.Header.Set("Content-Type", "application/json")

	resp, _ := app.Test(req)

	if resp.StatusCode != 201 {
		t.Fatalf("Expected status 201, got %d", resp.StatusCode)
	}

	var result map[string]any
	json.NewDecoder(resp.Body).Decode(&result)
	if result["created"].(float64) != 3 {
		t.Errorf("Expected 3 created, got %v", result["created"])
	}

	_, total, _ := events.List(db, events.ListFilters{Limit: 10})
	if total != 3 {
		t.Errorf("Expected 3 events in database, got %d", total)
	}
}

func TestIssuesGrouping(t *testing.T) {
	db := setupTestDB(t)

	// Create 3 events with "Payment failed" error
	for i := range 3 {
		event := &events.Event{
			Message:    "POST /api/checkout → 500",
			TraceID:    "req-" + string(rune('a'+i)),
			DurationMS: 100,
			IsError:    true,
		}
		event.SetContext(&events.EventContext{
			Method: "POST",
			Path:   "/api/checkout",
			Status: 500,
			Error:  "Payment failed: card declined",
		})
		events.Create(db, event)
	}

	// Create 2 events with "Database timeout" error
	for i := range 2 {
		event := &events.Event{
			Message:    "GET /api/users → 500",
			TraceID:    "req-db-" + string(rune('0'+i)),
			DurationMS: 50,
			IsError:    true,
		}
		event.SetContext(&events.EventContext{
			Method: "GET",
			Path:   "/api/users",
			Status: 500,
			Error:  "Database connection timeout",
		})
		events.Create(db, event)
	}

	issues, err := events.ListIssuesWithState(db, 0, 10) // 0 = all apps

	if err != nil {
		t.Fatalf("Failed to list issues: %v", err)
	}
	if len(issues) != 2 {
		t.Errorf("Expected 2 issues, got %d", len(issues))
	}

	var paymentIssue, dbIssue *events.IssueWithState
	for i := range issues {
		if issues[i].Error == "Payment failed: card declined" {
			paymentIssue = &issues[i]
		}
		if issues[i].Error == "Database connection timeout" {
			dbIssue = &issues[i]
		}
	}

	if paymentIssue == nil {
		t.Error("Expected payment issue")
	} else if paymentIssue.Count != 3 {
		t.Errorf("Expected payment issue count 3, got %d", paymentIssue.Count)
	}

	if dbIssue == nil {
		t.Error("Expected database issue")
	} else if dbIssue.Count != 2 {
		t.Errorf("Expected database issue count 2, got %d", dbIssue.Count)
	}
}

func TestCompleteSystemFlow(t *testing.T) {
	db := setupTestDB(t)

	_, fullKey, _ := apikeys.Create(db, "Test App")

	// Set up minimal app with real ingestion handler
	app := fiber.New()
	app.Post("/api/v1/events", func(c *fiber.Ctx) error {
		authHeader := c.Get("Authorization")
		if authHeader == "" {
			return c.Status(401).JSON(fiber.Map{"error": "API key required"})
		}
		apiKey := authHeader[7:]

		validatedKey, err := apikeys.Validate(db, apiKey)
		if err != nil {
			return c.Status(401).JSON(fiber.Map{"error": "Invalid API key"})
		}

		var req struct {
			Message    string         `json:"message"`
			TraceID    string         `json:"trace_id"`
			DurationMS int            `json:"duration_ms"`
			Context    map[string]any `json:"context"`
		}
		if err := c.BodyParser(&req); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
		}

		isError := false
		if req.Context != nil {
			if _, ok := req.Context["error"]; ok {
				isError = true
			}
			if status, ok := req.Context["status"].(float64); ok && status >= 500 {
				isError = true
			}
		}

		contextJSON, _ := json.Marshal(req.Context)
		ingested := &events.IngestedEvent{
			APIKeyID:   validatedKey.ID,
			Message:    req.Message,
			TraceID:    req.TraceID,
			DurationMS: req.DurationMS,
			IsError:    isError,
			Context:    string(contextJSON),
		}

		if err := events.CreateIngested(db, ingested); err != nil {
			return c.Status(500).JSON(fiber.Map{"error": "Failed to create event"})
		}

		return c.Status(201).JSON(fiber.Map{"created": 1})
	})

	// Send success event
	successBody := []byte(`{
		"message": "GET /api/users → 200",
		"trace_id": "req-success-1",
		"duration_ms": 45,
		"context": {
			"method": "GET",
			"path": "/api/users",
			"status": 200
		}
	}`)
	req := httptest.NewRequest("POST", "/api/v1/events", bytes.NewReader(successBody))
	req.Header.Set("Authorization", "Bearer "+fullKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("Success request failed: %v", err)
	}
	if resp.StatusCode != 201 {
		t.Fatalf("Expected status 201 for success event, got %d", resp.StatusCode)
	}

	// Send error event
	errorBody := []byte(`{
		"message": "POST /api/checkout → 500",
		"trace_id": "req-error-1",
		"duration_ms": 150,
		"context": {
			"method": "POST",
			"path": "/api/checkout",
			"status": 500,
			"error": "Payment failed: card declined"
		}
	}`)
	req = httptest.NewRequest("POST", "/api/v1/events", bytes.NewReader(errorBody))
	req.Header.Set("Authorization", "Bearer "+fullKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err = app.Test(req)
	if err != nil {
		t.Fatalf("Error request failed: %v", err)
	}
	if resp.StatusCode != 201 {
		t.Fatalf("Expected status 201 for error event, got %d", resp.StatusCode)
	}

	// Verify events are in ingested_events table (unprocessed)
	unprocessedCount, err := events.CountUnprocessed(db)
	if err != nil {
		t.Fatalf("Failed to count unprocessed: %v", err)
	}
	if unprocessedCount != 2 {
		t.Errorf("Expected 2 unprocessed events, got %d", unprocessedCount)
	}

	// Verify events table is empty (not processed yet)
	eventList, total, _ := events.List(db, events.ListFilters{Limit: 10})
	if total != 0 {
		t.Errorf("Expected 0 processed events before job runs, got %d", total)
	}

	// Run the event processor job manually
	ingested, _ := events.GetUnprocessed(db, 100)
	var processedIDs []uint
	for _, ie := range ingested {
		event := &events.Event{
			APIKeyID:   ie.APIKeyID,
			Message:    ie.Message,
			TraceID:    ie.TraceID,
			DurationMS: ie.DurationMS,
			IsError:    ie.IsError,
			Context:    ie.Context,
			Timestamp:  ie.Timestamp,
		}
		if err := events.Create(db, event); err != nil {
			t.Fatalf("Failed to create processed event: %v", err)
		}
		processedIDs = append(processedIDs, ie.ID)
	}
	if err := events.MarkProcessed(db, processedIDs); err != nil {
		t.Fatalf("Failed to mark as processed: %v", err)
	}

	// Verify events are now in events table
	eventList, total, _ = events.List(db, events.ListFilters{Limit: 10})
	if total != 2 {
		t.Errorf("Expected 2 processed events, got %d", total)
	}

	// Verify error vs non-error
	var errorCount, successCount int
	for _, e := range eventList {
		if e.IsError {
			errorCount++
		} else {
			successCount++
		}
	}
	if errorCount != 1 {
		t.Errorf("Expected 1 error event, got %d", errorCount)
	}
	if successCount != 1 {
		t.Errorf("Expected 1 success event, got %d", successCount)
	}

	// Verify issues grouping (only error should create issue)
	issues, err := events.ListIssuesWithState(db, 0, 10)
	if err != nil {
		t.Fatalf("Failed to list issues: %v", err)
	}
	if len(issues) != 1 {
		t.Errorf("Expected 1 issue, got %d", len(issues))
	}
	if len(issues) > 0 && issues[0].Error != "Payment failed: card declined" {
		t.Errorf("Expected issue 'Payment failed: card declined', got '%s'", issues[0].Error)
	}
}

func TestAgentAPIAuth(t *testing.T) {
	db := setupTestDB(t)

	// Create app key (should NOT work for agent API)
	_, appKey, _ := apikeys.Create(db, "Test App")

	// Create agent key (should work for agent API)
	_, agentKey, _ := apikeys.CreateAgent(db)

	// Create test event
	event := &events.Event{
		Message: "Test error",
		IsError: true,
	}
	event.SetContext(&events.EventContext{Error: "test error"})
	events.Create(db, event)

	app := fiber.New()
	app.Get("/api/v1/agent/events", func(c *fiber.Ctx) error {
		authHeader := c.Get("Authorization")
		if authHeader == "" {
			return c.Status(401).JSON(fiber.Map{"error": "API key required"})
		}
		if len(authHeader) < 8 {
			return c.Status(401).JSON(fiber.Map{"error": "Invalid agent key"})
		}
		apiKey := authHeader[7:]

		// Must be an agent key, not an app key
		_, err := apikeys.ValidateAgentKey(db, apiKey)
		if err != nil {
			return c.Status(401).JSON(fiber.Map{"error": "Invalid agent key"})
		}

		eventList, total, _ := events.List(db, events.ListFilters{Limit: 50})
		return c.JSON(fiber.Map{"events": eventList, "total": total})
	})

	t.Run("requires auth", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/v1/agent/events", nil)

		resp, _ := app.Test(req)

		if resp.StatusCode != 401 {
			t.Errorf("Expected 401, got %d", resp.StatusCode)
		}
	})

	t.Run("rejects invalid key", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/v1/agent/events", nil)
		req.Header.Set("Authorization", "Bearer invalid_key")

		resp, _ := app.Test(req)

		if resp.StatusCode != 401 {
			t.Errorf("Expected 401, got %d", resp.StatusCode)
		}
	})

	t.Run("rejects app key", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/v1/agent/events", nil)
		req.Header.Set("Authorization", "Bearer "+appKey)

		resp, _ := app.Test(req)

		if resp.StatusCode != 401 {
			t.Errorf("Expected 401 for app key, got %d", resp.StatusCode)
		}
	})

	t.Run("accepts agent key", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/v1/agent/events", nil)
		req.Header.Set("Authorization", "Bearer "+agentKey)

		resp, _ := app.Test(req)

		if resp.StatusCode != 200 {
			body, _ := io.ReadAll(resp.Body)
			t.Errorf("Expected 200, got %d: %s", resp.StatusCode, string(body))
		}

		var result map[string]any
		json.NewDecoder(resp.Body).Decode(&result)
		if _, ok := result["events"]; !ok {
			t.Error("Expected 'events' in response")
		}
	})
}
