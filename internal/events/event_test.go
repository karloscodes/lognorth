package events

import (
	"testing"
	"time"

	"gorm.io/gorm"
)

func setupTestDB(t *testing.T) *gorm.DB {
	return newTestDB(t)
}

func TestIsErrorContext(t *testing.T) {
	cases := []struct {
		name string
		ctx  map[string]any
		want bool
	}{
		{"error field present", map[string]any{"error": "something broke"}, true},
		{"error class present", map[string]any{"error_class": "TimeoutError"}, true},
		{"status 500", map[string]any{"status": float64(500)}, true},
		{"status 503 as int", map[string]any{"status": 503}, true},
		{"status 404 is not an error", map[string]any{"status": float64(404)}, false},
		{"empty error is not an error", map[string]any{"error": ""}, false},
		{"empty context is not an error", map[string]any{}, false},
		{"nil context is not an error", nil, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := IsErrorContext(c.ctx); got != c.want {
				t.Errorf("IsErrorContext(%v) = %v, want %v", c.ctx, got, c.want)
			}
		})
	}
}

func TestEventContext_GetSet(t *testing.T) {
	event := &Event{}
	ctx := &EventContext{
		Method: "POST",
		Path:   "/api/orders",
		Status: 201,
		Error:  "",
	}

	err := event.SetContext(ctx)

	if err != nil {
		t.Fatalf("SetContext failed: %v", err)
	}
	if event.Context == "" {
		t.Error("Context should be set")
	}

	parsed, err := event.GetContext()

	if err != nil {
		t.Fatalf("GetContext failed: %v", err)
	}
	if parsed.Method != "POST" {
		t.Errorf("expected Method 'POST', got %s", parsed.Method)
	}
	if parsed.Path != "/api/orders" {
		t.Errorf("expected Path '/api/orders', got %s", parsed.Path)
	}
	if parsed.Status != 201 {
		t.Errorf("expected Status 201, got %d", parsed.Status)
	}
}

func TestEventContext_EmptyContext(t *testing.T) {
	event := &Event{Context: ""}

	ctx, err := event.GetContext()

	if err != nil {
		t.Fatalf("GetContext failed on empty: %v", err)
	}
	if ctx == nil {
		t.Error("should return empty context, not nil")
	}
}

func TestCreate(t *testing.T) {
	db := setupTestDB(t)

	event := &Event{
		APIKeyID:   1,
		Message:    "GET /api/users → 200",
		TraceID:    "req-123",
		DurationMS: 45,
		IsError:    false,
	}

	err := Create(db, event)

	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	if event.ID == 0 {
		t.Error("expected ID to be set")
	}
	if event.Timestamp.IsZero() {
		t.Error("expected Timestamp to be set")
	}
}

func TestCreate_WithTimestamp(t *testing.T) {
	db := setupTestDB(t)
	customTime := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)

	event := &Event{
		APIKeyID:  1,
		Message:   "Custom timestamp",
		Timestamp: customTime,
	}

	Create(db, event)

	if !event.Timestamp.Equal(customTime) {
		t.Error("should preserve custom timestamp")
	}
}

func TestList_Filters(t *testing.T) {
	db := setupTestDB(t)

	// Create test events
	Create(db, &Event{APIKeyID: 1, Message: "success", IsError: false})
	Create(db, &Event{APIKeyID: 1, Message: "error", IsError: true})
	Create(db, &Event{APIKeyID: 2, Message: "other app", IsError: false})

	t.Run("filter by api key", func(t *testing.T) {
		events, total, _ := List(db, ListFilters{APIKeyID: 1})

		if total != 2 {
			t.Errorf("expected 2 events for api key 1, got %d", total)
		}
		if len(events) != 2 {
			t.Errorf("expected 2 events, got %d", len(events))
		}
	})

	t.Run("filter errors only", func(t *testing.T) {
		isError := true
		events, total, _ := List(db, ListFilters{IsError: &isError})

		if total != 1 {
			t.Errorf("expected 1 error, got %d", total)
		}
		if len(events) != 1 || events[0].Message != "error" {
			t.Error("should return only error events")
		}
	})

	t.Run("search", func(t *testing.T) {
		events, total, _ := List(db, ListFilters{Search: "other"})

		if total != 1 {
			t.Errorf("expected 1 match, got %d", total)
		}
		if len(events) != 1 || events[0].Message != "other app" {
			t.Error("should find matching event")
		}
	})
}

func TestDeleteOlderThan(t *testing.T) {
	db := setupTestDB(t)

	// Create old and new events
	oldEvent := &Event{
		APIKeyID:  1,
		Message:   "old",
		Timestamp: time.Now().Add(-48 * time.Hour),
	}
	newEvent := &Event{
		APIKeyID:  1,
		Message:   "new",
		Timestamp: time.Now(),
	}
	db.Create(oldEvent)
	db.Create(newEvent)

	deleted, err := DeleteOlderThan(db, 24*time.Hour)

	if err != nil {
		t.Fatalf("DeleteOlderThan failed: %v", err)
	}
	if deleted != 1 {
		t.Errorf("expected 1 deleted, got %d", deleted)
	}

	var remaining []Event
	db.Find(&remaining)
	if len(remaining) != 1 || remaining[0].Message != "new" {
		t.Error("should keep only new event")
	}
}

func TestGetStats(t *testing.T) {
	db := setupTestDB(t)

	Create(db, &Event{APIKeyID: 1, Message: "success", IsError: false})
	Create(db, &Event{APIKeyID: 1, Message: "error1", IsError: true})
	Create(db, &Event{APIKeyID: 1, Message: "error2", IsError: true})

	stats := GetStats(db, 1)

	if stats.Total != 3 {
		t.Errorf("expected total 3, got %d", stats.Total)
	}
	if stats.Errors != 2 {
		t.Errorf("expected errors 2, got %d", stats.Errors)
	}
	if stats.LastSeen == nil {
		t.Error("expected LastSeen to be set")
	}
}
