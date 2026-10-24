package tests

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/karloscodes/cartridge"
	"github.com/karloscodes/cartridge/sqlite"
	"github.com/karloscodes/cartridge/testsupport"
	"gorm.io/gorm"

	"lognorth/internal/apikeys"
	"lognorth/internal/database"
	"lognorth/internal/events"
	"lognorth/internal/jobs"
)

func setupTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "test.db")

	manager := sqlite.NewManager(sqlite.Config{
		Path:      dbPath,
		EnableWAL: true,
	})
	db, err := manager.Connect()
	if err != nil {
		t.Fatalf("Failed to connect to database: %v", err)
	}

	if err := database.Migrate(db); err != nil {
		t.Fatalf("Failed to migrate: %v", err)
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

	validated, err := apikeys.ValidateAppKey(db, fullKey)

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
	key, fullKey, _ := apikeys.Create(db, "Production")
	server := newTestServer(t, testsupport.NewTestDBManager(db), mountIngest)
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

	resp, err := server.Test(req)

	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	if resp.StatusCode != 201 {
		respBody, _ := io.ReadAll(resp.Body)
		t.Fatalf("Expected status 201, got %d: %s", resp.StatusCode, string(respBody))
	}
	var stored []events.IngestedEvent
	db.Find(&stored)
	if len(stored) != 1 {
		t.Fatalf("Expected 1 stored event, got %d", len(stored))
	}
	if stored[0].Message != "POST /api/orders → 201" {
		t.Errorf("Expected message 'POST /api/orders → 201', got '%s'", stored[0].Message)
	}
	if stored[0].APIKeyID != key.ID {
		t.Errorf("Expected the event to belong to app %d, got %d", key.ID, stored[0].APIKeyID)
	}
}

func TestIngestionAPIRejectsBadKeys(t *testing.T) {
	post := func(t *testing.T, authorization string) (int, int64) {
		t.Helper()
		db := setupTestDB(t)
		apikeys.Create(db, "Production")
		server := newTestServer(t, testsupport.NewTestDBManager(db), mountIngest)
		req := httptest.NewRequest("POST", "/api/v1/events", bytes.NewReader([]byte(`{"message": "test"}`)))
		req.Header.Set("Content-Type", "application/json")
		if authorization != "" {
			req.Header.Set("Authorization", authorization)
		}

		resp, err := server.Test(req)

		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
		var stored int64
		db.Model(&events.IngestedEvent{}).Count(&stored)
		return resp.StatusCode, stored
	}

	t.Run("with no Authorization header", func(t *testing.T) {
		status, stored := post(t, "")

		if status != 401 {
			t.Errorf("Expected status 401, got %d", status)
		}
		if stored != 0 {
			t.Errorf("Expected nothing stored, got %d events", stored)
		}
	})

	t.Run("with a key that does not exist", func(t *testing.T) {
		status, stored := post(t, "Bearer lgn-not-a-real-key")

		if status != 401 {
			t.Errorf("Expected status 401, got %d", status)
		}
		if stored != 0 {
			t.Errorf("Expected nothing stored, got %d events", stored)
		}
	})
}

func TestIngestionBatch(t *testing.T) {
	db := setupTestDB(t)
	_, fullKey, _ := apikeys.Create(db, "Production")
	server := newTestServer(t, testsupport.NewTestDBManager(db), mountIngest)
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

	resp, _ := server.Test(req)

	if resp.StatusCode != 201 {
		t.Fatalf("Expected status 201, got %d", resp.StatusCode)
	}
	var result map[string]any
	json.NewDecoder(resp.Body).Decode(&result)
	if result["created"].(float64) != 3 {
		t.Errorf("Expected 3 created, got %v", result["created"])
	}
	var stored, failed int64
	db.Model(&events.IngestedEvent{}).Count(&stored)
	db.Model(&events.IngestedEvent{}).Where("is_error = ?", true).Count(&failed)
	if stored != 3 {
		t.Errorf("Expected 3 stored events, got %d", stored)
	}
	if failed != 1 {
		t.Errorf("Expected the one 500 to be stored as an error, got %d", failed)
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
	// The processor keeps the issues table; this test writes events directly.
	events.RecountAllIssues(db)

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

	server := newTestServer(t, testsupport.NewTestDBManager(db), mountIngest)

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

	resp, err := server.Test(req)
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

	resp, err = server.Test(req)
	if err != nil {
		t.Fatalf("Error request failed: %v", err)
	}
	if resp.StatusCode != 201 {
		t.Fatalf("Expected status 201 for error event, got %d", resp.StatusCode)
	}

	// Verify events wait in ingested_events
	var waiting int64
	db.Model(&events.IngestedEvent{}).Count(&waiting)
	if waiting != 2 {
		t.Errorf("Expected 2 waiting events, got %d", waiting)
	}

	// Verify events table is empty (not processed yet)
	_, total, _ := events.List(db, events.ListFilters{Limit: 10})
	if total != 0 {
		t.Errorf("Expected 0 processed events before job runs, got %d", total)
	}

	// Run the real event processor job
	jobCtx := &cartridge.JobContext{Context: t.Context(), Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), DB: db}
	if err := jobs.NewEventProcessor().ProcessBatch(jobCtx); err != nil {
		t.Fatalf("Event processor failed: %v", err)
	}
	db.Model(&events.IngestedEvent{}).Count(&waiting)
	if waiting != 0 {
		t.Errorf("Expected the processor to empty ingested_events, got %d left", waiting)
	}

	// Verify events are now in events table
	eventList, total, _ := events.List(db, events.ListFilters{Limit: 10})
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
