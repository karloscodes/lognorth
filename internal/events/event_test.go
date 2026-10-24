package events

import (
	"strings"
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

func TestList_SearchMatchesTheTermAsWritten(t *testing.T) {
	db := setupTestDB(t)
	Create(db, &Event{APIKeyID: 1, Message: "disk at 100%"})
	Create(db, &Event{APIKeyID: 1, Message: "1000 rows"})
	Create(db, &Event{APIKeyID: 1, Message: "user_id missing"})
	Create(db, &Event{APIKeyID: 1, Message: "userXid missing"})
	Create(db, &Event{APIKeyID: 1, Message: `C:\temp full`})

	for search, want := range map[string]string{
		"100%":    "disk at 100%",
		"user_id": "user_id missing",
		`C:\temp`: `C:\temp full`,
	} {
		t.Run(search, func(t *testing.T) {
			events, total, _ := List(db, ListFilters{APIKeyID: 1, Search: search})

			if total != 1 || len(events) != 1 || events[0].Message != want {
				t.Errorf("found %d: %+v, want only %q", total, events, want)
			}
		})
	}
}

func TestList_Total(t *testing.T) {
	db := setupTestDB(t)
	for _, message := range []string{"one", "two", "three"} {
		Create(db, &Event{APIKeyID: 1, Message: message})
	}

	for _, c := range []struct {
		name          string
		limit, offset int
		page          int
	}{
		{"on a full page", 2, 0, 2},
		{"on the last page, not full", 2, 2, 1},
		{"on a page past the end", 2, 5, 0},
		{"on one page that holds everything", 50, 0, 3},
	} {
		t.Run(c.name, func(t *testing.T) {
			events, total, err := List(db, ListFilters{APIKeyID: 1, Limit: c.limit, Offset: c.offset})

			if err != nil || len(events) != c.page || total != 3 {
				t.Errorf("page %d, total %d, err %v; want page %d and total 3", len(events), total, err, c.page)
			}
		})
	}
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
	countHours(t, db)

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

// An app's error count reads the error index, not every event of the app:
// at 1.2M events that is 4 ms instead of 350 ms. A bound is_error = ? would
// silently stop SQLite from using the partial index.
func TestErrorQueriesReadTheErrorIndex(t *testing.T) {
	db := setupTestDB(t)
	errorsOnly := true
	queries := map[string]*gorm.DB{
		"the error count of GetStats": db.Session(&gorm.Session{DryRun: true}).Model(&Event{}).Where("is_error = 1").Where("api_key_id = ?", 1).Count(new(int64)),
		"the error filter of List":    db.Session(&gorm.Session{DryRun: true}).Model(&Event{}).Where("api_key_id = ?", 1).Where(isErrorIs(errorsOnly)).Count(new(int64)),
	}

	for name, query := range queries {
		t.Run(name, func(t *testing.T) {
			var plan []struct{ Detail string }
			db.Raw("EXPLAIN QUERY PLAN "+query.Statement.SQL.String(), query.Statement.Vars...).Scan(&plan)

			if len(plan) == 0 || !strings.Contains(plan[0].Detail, "idx_events_errors") {
				t.Errorf("plan = %+v, want a search of idx_events_errors", plan)
			}
		})
	}
}

func TestGetEndpointStats(t *testing.T) {
	t.Run("sums the stored buckets per endpoint, most broken first", func(t *testing.T) {
		db := setupTestDB(t)
		now := time.Now().UTC().Truncate(5 * time.Minute)
		db.Create(&EndpointStat{APIKeyID: 1, Path: "/ok", Bucket: now, Total: 90, Errors: 0, AvgDuration: 10})
		db.Create(&EndpointStat{APIKeyID: 1, Path: "/pay", Bucket: now, Total: 10, Errors: 2, AvgDuration: 100})
		db.Create(&EndpointStat{APIKeyID: 1, Path: "/pay", Bucket: now.Add(-time.Hour), Total: 30, Errors: 2, AvgDuration: 200})

		stats, err := GetEndpointStats(db, 1, 10)

		if err != nil || len(stats) != 2 {
			t.Fatalf("stats = %+v, err = %v, want 2 endpoints", stats, err)
		}
		pay := stats[0]
		if pay.Path != "/pay" || pay.Total != 40 || pay.Errors != 4 || pay.ErrorRate != 10 || pay.AvgDuration != 175 {
			t.Errorf("/pay = %+v, want 40 requests, 4 errors, 10%%, 175ms weighted average", pay)
		}
	})

	t.Run("leaves out buckets older than the window", func(t *testing.T) {
		db := setupTestDB(t)
		old := time.Now().UTC().Add(-EndpointStatsWindow - time.Hour)
		db.Create(&EndpointStat{APIKeyID: 1, Path: "/old", Bucket: old, Total: 5, Errors: 5})

		stats, _ := GetEndpointStats(db, 1, 10)

		if len(stats) != 0 {
			t.Errorf("stats = %+v, want none: the bucket is outside the window", stats)
		}
	})
}

// countHours counts the stored events into app_hours, as the processor does
// for the events it moves. The tests here write events directly.
func countHours(t *testing.T, db *gorm.DB) {
	t.Helper()
	if err := RecountAppHours(db, time.Time{}); err != nil {
		t.Fatalf("RecountAppHours: %v", err)
	}
}

func TestGetHistogram(t *testing.T) {
	sum := func(buckets []HistogramBucket) (total, errors int64) {
		for _, b := range buckets {
			total += b.Total
			errors += b.Errors
		}
		return total, errors
	}

	t.Run("counts the events of the hour that is not over yet", func(t *testing.T) {
		db := setupTestDB(t)
		Create(db, &Event{APIKeyID: 1, Message: "now", Timestamp: time.Now()})
		Create(db, &Event{APIKeyID: 1, Message: "failed now", IsError: true, Timestamp: time.Now()})
		countHours(t, db)

		buckets := GetHistogram(db, 1, time.Time{}, time.Time{})

		if total, errors := sum(buckets); total != 2 || errors != 1 {
			t.Errorf("total %d, errors %d; want 2 and 1", total, errors)
		}
	})

	t.Run("counts the newest day of a 30-day range", func(t *testing.T) {
		db := setupTestDB(t)
		now := time.Now()
		Create(db, &Event{APIKeyID: 1, Message: "now", Timestamp: now})
		Create(db, &Event{APIKeyID: 1, Message: "29 days ago", Timestamp: now.Add(-29 * 24 * time.Hour)})
		countHours(t, db)

		buckets := GetHistogram(db, 1, now.Add(-30*24*time.Hour), time.Time{})

		if total, _ := sum(buckets); total != 2 {
			t.Errorf("total %d, want 2", total)
		}
	})

	t.Run("for all time starts at the first event of the app", func(t *testing.T) {
		db := setupTestDB(t)
		now := time.Now()
		Create(db, &Event{APIKeyID: 1, Message: "40 days ago", Timestamp: now.Add(-40 * 24 * time.Hour)})
		Create(db, &Event{APIKeyID: 1, Message: "now", Timestamp: now})
		Create(db, &Event{APIKeyID: 2, Message: "other app", Timestamp: now.Add(-90 * 24 * time.Hour)})
		countHours(t, db)

		buckets := GetHistogram(db, 1, time.Time{}, time.Time{})

		if total, _ := sum(buckets); total != 2 {
			t.Errorf("total %d, want 2", total)
		}
	})

	t.Run("puts each event in the bucket of its time", func(t *testing.T) {
		db := setupTestDB(t)
		start := time.Now().UTC().Truncate(time.Hour).Add(-5 * time.Hour)
		Create(db, &Event{APIKeyID: 1, Message: "first hour", Timestamp: start.Add(10 * time.Minute)})
		Create(db, &Event{APIKeyID: 1, Message: "third hour", Timestamp: start.Add(2*time.Hour + 59*time.Minute)})
		countHours(t, db)

		buckets := GetHistogram(db, 1, start, time.Time{})

		if len(buckets) < 3 || buckets[0].Total != 1 || buckets[1].Total != 0 || buckets[2].Total != 1 {
			t.Errorf("buckets = %+v, want 1, 0, 1 in the first three hours", buckets)
		}
		if !buckets[0].Timestamp.Equal(start) {
			t.Errorf("first bucket at %s, want %s", buckets[0].Timestamp, start)
		}
	})
}
