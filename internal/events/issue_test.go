package events

import (
	"strings"
	"testing"
	"time"

	"gorm.io/gorm"
)

func setupIssueTestDB(t *testing.T) *gorm.DB {
	return newTestDB(t)
}

// =============================================================================

func TestComputeErrorHashes(t *testing.T) {
	t.Run("same inputs produce same hashes", func(t *testing.T) {
		h1a, h2a, h3a := computeErrorHashes("TypeError", "undefined is not a function", "app.ts", 42, 1)
		h1b, h2b, h3b := computeErrorHashes("TypeError", "undefined is not a function", "app.ts", 42, 1)

		if h1a != h1b || h2a != h2b || h3a != h3b {
			t.Error("same inputs should produce same hashes")
		}
	})

	t.Run("same class+message different file produces same h1 different h2 h3", func(t *testing.T) {
		h1a, h2a, h3a := computeErrorHashes("TypeError", "undefined is not a function", "app.ts", 42, 1)
		h1b, h2b, h3b := computeErrorHashes("TypeError", "undefined is not a function", "other.ts", 10, 1)

		if h1a != h1b {
			t.Error("same class+message should produce same h1")
		}
		if h2a == h2b {
			t.Error("different file should produce different h2")
		}
		if h3a == h3b {
			t.Error("different file+line should produce different h3")
		}
	})

	t.Run("no class uses message only", func(t *testing.T) {
		h1a, _, _ := computeErrorHashes("", "connection refused", "db.go", 10, 1)
		h1b, _, _ := computeErrorHashes("", "connection refused", "db.go", 10, 1)

		if h1a != h1b {
			t.Error("same message-only should produce same h1")
		}

		// Different from when class is present
		h1c, _, _ := computeErrorHashes("NetError", "connection refused", "db.go", 10, 1)
		if h1a == h1c {
			t.Error("message-only vs class+message should differ")
		}
	})

	t.Run("no file or line leaves h2 h3 empty", func(t *testing.T) {
		h1, h2, h3 := computeErrorHashes("TypeError", "bad stuff", "", 0, 1)

		if h1 == "" {
			t.Error("h1 should always be set")
		}
		if h2 != "" {
			t.Error("h2 should be empty when no file")
		}
		if h3 != "" {
			t.Error("h3 should be empty when no file/line")
		}
	})

	t.Run("file but no line leaves h3 empty", func(t *testing.T) {
		h1, h2, h3 := computeErrorHashes("TypeError", "bad stuff", "app.ts", 0, 1)

		if h1 == "" || h2 == "" {
			t.Error("h1 and h2 should be set")
		}
		if h3 != "" {
			t.Error("h3 should be empty when no line")
		}
	})

	t.Run("different apps produce different hashes", func(t *testing.T) {
		h1a, _, _ := computeErrorHashes("TypeError", "bad stuff", "app.ts", 42, 1)
		h1b, _, _ := computeErrorHashes("TypeError", "bad stuff", "app.ts", 42, 2)

		if h1a == h1b {
			t.Error("different apps should produce different hashes")
		}
	})
}

// =============================================================================
// scoreIssue - Prioritization logic
// =============================================================================

func TestScoreIssue(t *testing.T) {
	t.Run("active and unseen scores highest", func(t *testing.T) {
		issue := IssueWithState{Active: true, Unseen: true}
		score := scoreIssue(issue)
		if score != 3 {
			t.Errorf("expected score 3, got %d", score)
		}
	})

	t.Run("active only scores 2", func(t *testing.T) {
		issue := IssueWithState{Active: true, Unseen: false}
		score := scoreIssue(issue)
		if score != 2 {
			t.Errorf("expected score 2, got %d", score)
		}
	})

	t.Run("unseen only scores 1", func(t *testing.T) {
		issue := IssueWithState{Active: false, Unseen: true}
		score := scoreIssue(issue)
		if score != 1 {
			t.Errorf("expected score 1, got %d", score)
		}
	})

	t.Run("inactive and seen scores 0", func(t *testing.T) {
		issue := IssueWithState{Active: false, Unseen: false}
		score := scoreIssue(issue)
		if score != 0 {
			t.Errorf("expected score 0, got %d", score)
		}
	})
}

// =============================================================================
// MarkIssueSeen
// =============================================================================

func TestMarkIssueSeen(t *testing.T) {
	db := setupIssueTestDB(t)

	t.Run("creates new record", func(t *testing.T) {
		err := MarkIssueSeen(db, "hash123")
		if err != nil {
			t.Fatalf("MarkIssueSeen failed: %v", err)
		}

		var seen IssueSeen
		db.Where("error_hash = ?", "hash123").First(&seen)
		if seen.ID == 0 {
			t.Error("expected record to be created")
		}
	})

	t.Run("updates existing record", func(t *testing.T) {
		// Create initial record
		MarkIssueSeen(db, "hash456")
		var first IssueSeen
		db.Where("error_hash = ?", "hash456").First(&first)
		firstSeenAt := first.SeenAt

		time.Sleep(10 * time.Millisecond)

		// Update
		MarkIssueSeen(db, "hash456")
		var second IssueSeen
		db.Where("error_hash = ?", "hash456").First(&second)

		if !second.SeenAt.After(firstSeenAt) {
			t.Error("expected SeenAt to be updated")
		}
		if first.ID != second.ID {
			t.Error("expected same record to be updated, not new one created")
		}
	})
}

// =============================================================================
// MarkAllIssuesSeen + ListIssuesWithState — end-to-end seen state
// =============================================================================

// makeErrorEvent creates and stores an error event with computed three-tier hashes,
// matching what the ingest pipeline does for normal events.
func makeErrorEvent(t *testing.T, db *gorm.DB, apiKeyID uint, class, message, file string, line int) *Event {
	t.Helper()
	ctx := EventContext{
		ErrorClass: class,
		Error:      message,
		ErrorFile:  file,
		ErrorLine:  line,
	}
	event := &Event{
		APIKeyID:     apiKeyID,
		Timestamp:    time.Now(),
		IsError:      true,
		Message:      message,
		ErrorClass:   class,
		ErrorMessage: message,
		ErrorFile:    file,
		ErrorLine:    line,
	}
	if err := event.SetContext(&ctx); err != nil {
		t.Fatalf("SetContext: %v", err)
	}
	PopulateErrorFields(event, &ctx)
	if err := Create(db, event); err != nil {
		t.Fatalf("Create event: %v", err)
	}
	return event
}

func TestMarkAllIssuesSeen_ClearsUnseenInListing(t *testing.T) {
	db := setupIssueTestDB(t)

	makeErrorEvent(t, db, 1, "TypeError", "undefined is not a function", "app.ts", 42)
	makeErrorEvent(t, db, 1, "RuntimeError", "boom", "worker.go", 10)
	countIssues(t, db)

	issues, err := ListIssuesWithState(db, 1, 50)
	if err != nil {
		t.Fatalf("ListIssuesWithState: %v", err)
	}
	if len(issues) != 2 {
		t.Fatalf("expected 2 issues, got %d", len(issues))
	}
	for _, i := range issues {
		if !i.Unseen {
			t.Fatalf("expected issue %s to be unseen before MarkAllIssuesSeen", i.Error)
		}
	}

	if err := MarkAllIssuesSeen(db, 1); err != nil {
		t.Fatalf("MarkAllIssuesSeen: %v", err)
	}

	issues, err = ListIssuesWithState(db, 1, 50)
	if err != nil {
		t.Fatalf("ListIssuesWithState (after): %v", err)
	}
	for _, i := range issues {
		if i.Unseen {
			t.Errorf("issue %s still unseen after MarkAllIssuesSeen — hash mismatch between mark and list", i.Error)
		}
	}
}

func TestListIssuesWithState_TimestampsAreISO8601UTC(t *testing.T) {
	db := setupIssueTestDB(t)
	makeErrorEvent(t, db, 1, "TypeError", "boom", "app.ts", 1)
	countIssues(t, db)

	issues, err := ListIssuesWithState(db, 1, 50)
	if err != nil {
		t.Fatalf("ListIssuesWithState: %v", err)
	}
	if len(issues) != 1 {
		t.Fatalf("expected 1 issue, got %d", len(issues))
	}

	// Browsers' new Date() needs the ISO 8601 'T' separator and a TZ suffix
	// (Z or ±hh:mm). SQLite's MIN/MAX returns "2026-04-14 11:45:00.108+00:00"
	// (space separator), which is parsed inconsistently across browsers — so
	// the API must normalize before sending.
	for _, field := range []string{issues[0].FirstSeen, issues[0].LastSeen} {
		if !strings.Contains(field, "T") {
			t.Errorf("timestamp %q missing T separator (would parse as local time in some browsers)", field)
		}
		if !strings.HasSuffix(field, "Z") {
			t.Errorf("timestamp %q missing Z suffix (TZ ambiguous to JS Date)", field)
		}
	}
}

func TestGetIssueOccurrencesByHour_HourIsISO8601UTC(t *testing.T) {
	db := setupIssueTestDB(t)
	makeErrorEvent(t, db, 1, "TypeError", "boom", "app.ts", 1)
	countIssues(t, db)

	issues, err := ListIssuesWithState(db, 1, 50)
	if err != nil || len(issues) != 1 {
		t.Fatalf("ListIssuesWithState: %v / %d issues", err, len(issues))
	}

	points, err := GetIssueOccurrencesByHour(db, issues[0].Hash, 1, 7)
	if err != nil {
		t.Fatalf("GetIssueOccurrencesByHour: %v", err)
	}
	if len(points) == 0 {
		t.Fatalf("expected at least one occurrence point")
	}

	// The frontend builds the heatmap day-bucket axis in the viewer's local TZ.
	// If the server emitted the bucket as a TZ-less string ("2026-04-14 11"),
	// JS would parse it inconsistently and events near midnight UTC would land
	// in the wrong day cell. Force RFC3339 UTC.
	for _, p := range points {
		if _, err := time.Parse(time.RFC3339, p.Hour); err != nil {
			t.Errorf("hour %q is not RFC3339 parseable: %v", p.Hour, err)
		}
		if !strings.HasSuffix(p.Hour, "Z") {
			t.Errorf("hour %q missing Z suffix (browser would assume local time)", p.Hour)
		}
	}
}

func TestMarkIssueSeen_ClearsUnseenInListing(t *testing.T) {
	db := setupIssueTestDB(t)

	makeErrorEvent(t, db, 1, "TypeError", "undefined is not a function", "app.ts", 42)
	countIssues(t, db)

	issues, err := ListIssuesWithState(db, 1, 50)
	if err != nil {
		t.Fatalf("ListIssuesWithState: %v", err)
	}
	if len(issues) != 1 || !issues[0].Unseen {
		t.Fatalf("expected 1 unseen issue, got %+v", issues)
	}
	hash := issues[0].Hash

	if err := MarkIssueSeen(db, hash); err != nil {
		t.Fatalf("MarkIssueSeen: %v", err)
	}

	issues, err = ListIssuesWithState(db, 1, 50)
	if err != nil {
		t.Fatalf("ListIssuesWithState (after): %v", err)
	}
	if issues[0].Unseen {
		t.Errorf("issue still unseen after MarkIssueSeen with GroupHash %q", hash)
	}
}

func TestRecentCounts(t *testing.T) {
	now := time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)

	at := func(t *testing.T, db *gorm.DB, when time.Time) string {
		t.Helper()
		event := makeErrorEvent(t, db, 1, "TypeError", "boom", "app.ts", 1)
		if err := db.Model(event).Update("timestamp", when).Error; err != nil {
			t.Fatalf("set timestamp: %v", err)
		}
		return event.IssueHash
	}

	t.Run("counts each day in the window, oldest first", func(t *testing.T) {
		db := setupIssueTestDB(t)
		hash := at(t, db, now.Add(-2*time.Hour))
		at(t, db, now.Add(-3*time.Hour))
		at(t, db, now.AddDate(0, 0, -3))

		daily := recentCounts(db, 1, []string{hash}, now)[hash].daily

		if len(daily) != 14 {
			t.Fatalf("expected 14 days, got %d", len(daily))
		}
		if daily[13] != 2 {
			t.Errorf("today: expected 2, got %d", daily[13])
		}
		if daily[10] != 1 {
			t.Errorf("3 days ago: expected 1, got %d", daily[10])
		}
	})

	t.Run("ignores errors older than the window", func(t *testing.T) {
		db := setupIssueTestDB(t)
		hash := at(t, db, now.AddDate(0, 0, -20))

		counts := recentCounts(db, 1, []string{hash}, now)

		if counts[hash] != nil {
			t.Errorf("expected no counts, got %+v", counts[hash])
		}
	})

	t.Run("the issue list carries the counts", func(t *testing.T) {
		db := setupIssueTestDB(t)
		makeErrorEvent(t, db, 1, "TypeError", "boom", "app.ts", 1)
		countIssues(t, db)

		issues, err := ListIssuesWithState(db, 1, 50)

		if err != nil {
			t.Fatalf("ListIssuesWithState: %v", err)
		}
		var sum int64
		for _, n := range issues[0].Daily {
			sum += n
		}
		if len(issues[0].Daily) != 14 || sum != 1 {
			t.Errorf("expected 14 days summing to 1, got %v", issues[0].Daily)
		}
	})
}

func TestGetIssuePaths(t *testing.T) {
	onPath := func(t *testing.T, db *gorm.DB, path string) string {
		t.Helper()
		ctx := EventContext{ErrorClass: "TypeError", Error: "boom", ErrorFile: "app.ts", ErrorLine: 1, Path: path}
		event := &Event{APIKeyID: 1, Timestamp: time.Now(), IsError: true, Message: "boom",
			ErrorClass: "TypeError", ErrorMessage: "boom", ErrorFile: "app.ts", ErrorLine: 1}
		if err := event.SetContext(&ctx); err != nil {
			t.Fatalf("SetContext: %v", err)
		}
		PopulateErrorFields(event, &ctx)
		if err := Create(db, event); err != nil {
			t.Fatalf("Create event: %v", err)
		}
		return event.IssueHash
	}

	t.Run("counts errors per path, most first", func(t *testing.T) {
		db := setupIssueTestDB(t)
		hash := onPath(t, db, "/checkout")
		onPath(t, db, "/checkout")
		onPath(t, db, "/cart")

		paths, err := GetIssuePaths(db, hash, 1, 20)

		if err != nil {
			t.Fatalf("GetIssuePaths: %v", err)
		}
		if len(paths) != 2 || paths[0].Path != "/checkout" || paths[0].Count != 2 || paths[1].Count != 1 {
			t.Errorf("expected /checkout 2 then /cart 1, got %+v", paths)
		}
	})

	t.Run("leaves out errors without a path", func(t *testing.T) {
		db := setupIssueTestDB(t)
		hash := onPath(t, db, "")

		paths, _ := GetIssuePaths(db, hash, 1, 20)

		if len(paths) != 0 {
			t.Errorf("expected no paths, got %+v", paths)
		}
	})
}
