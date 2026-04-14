package events

import (
	"os"
	"testing"
	"time"

	"github.com/karloscodes/cartridge/sqlite"
	"gorm.io/gorm"
)

func setupIssueTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	dbPath := "test_issue_" + t.Name() + ".db"
	t.Cleanup(func() {
		os.Remove(dbPath)
		os.Remove(dbPath + "-wal")
		os.Remove(dbPath + "-shm")
	})

	manager := sqlite.NewManager(sqlite.Config{Path: dbPath, EnableWAL: true})
	db, err := manager.Connect()
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}

	if err := AutoMigrate(db); err != nil {
		t.Fatalf("Failed to migrate events: %v", err)
	}
	if err := AutoMigrateIssues(db); err != nil {
		t.Fatalf("Failed to migrate issues: %v", err)
	}

	return db
}

// =============================================================================
// hashError - Core grouping logic
// =============================================================================

func TestHashError(t *testing.T) {
	t.Run("groups by error_type and location", func(t *testing.T) {
		ctx := map[string]any{
			"error_type":     "TypeError",
			"error_location": "checkout.ts:42",
			"error":          "Cannot read property of undefined",
		}

		hash := hashError(ctx, 1)

		// Same error_type+location = same hash
		ctx2 := map[string]any{
			"error_type":     "TypeError",
			"error_location": "checkout.ts:42",
			"error":          "Different error message",
		}
		hash2 := hashError(ctx2, 1)

		if hash != hash2 {
			t.Error("same error_type+location should produce same hash")
		}
	})

	t.Run("falls back to error message", func(t *testing.T) {
		ctx := map[string]any{
			"error": "Connection refused",
		}

		hash := hashError(ctx, 1)

		ctx2 := map[string]any{
			"error": "Connection refused",
		}
		hash2 := hashError(ctx2, 1)

		if hash != hash2 {
			t.Error("same error message should produce same hash")
		}
	})

	t.Run("different error messages produce different hashes", func(t *testing.T) {
		ctx1 := map[string]any{"error": "Error A"}
		ctx2 := map[string]any{"error": "Error B"}

		hash1 := hashError(ctx1, 1)
		hash2 := hashError(ctx2, 1)

		if hash1 == hash2 {
			t.Error("different errors should produce different hashes")
		}
	})

	t.Run("same error in different apps produces different hashes", func(t *testing.T) {
		ctx := map[string]any{"error": "Connection refused"}

		hash1 := hashError(ctx, 1)
		hash2 := hashError(ctx, 2)

		if hash1 == hash2 {
			t.Error("same error in different apps should have different hashes")
		}
	})

	t.Run("error_type takes precedence over error", func(t *testing.T) {
		// With error_type
		ctx1 := map[string]any{
			"error_type":     "DatabaseError",
			"error_location": "db.go:10",
			"error":          "connection failed",
		}
		// Same error_type, different error message
		ctx2 := map[string]any{
			"error_type":     "DatabaseError",
			"error_location": "db.go:10",
			"error":          "timeout exceeded",
		}

		hash1 := hashError(ctx1, 1)
		hash2 := hashError(ctx2, 1)

		if hash1 != hash2 {
			t.Error("error_type+location should take precedence, producing same hash")
		}
	})
}

// =============================================================================
// computeErrorHashes - Three-tier grouping
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
// splitPaths - Helper function
// =============================================================================

func TestSplitPaths(t *testing.T) {
	t.Run("empty string", func(t *testing.T) {
		result := splitPaths("")
		if len(result) != 0 {
			t.Errorf("expected empty slice, got %v", result)
		}
	})

	t.Run("single path", func(t *testing.T) {
		result := splitPaths("/api/users")
		if len(result) != 1 || result[0] != "/api/users" {
			t.Errorf("expected ['/api/users'], got %v", result)
		}
	})

	t.Run("multiple paths", func(t *testing.T) {
		result := splitPaths("/api/users,/api/orders,/api/products")
		if len(result) != 3 {
			t.Errorf("expected 3 paths, got %d", len(result))
		}
	})

	t.Run("trims whitespace", func(t *testing.T) {
		result := splitPaths("/api/users , /api/orders ")
		if len(result) != 2 || result[0] != "/api/users" || result[1] != "/api/orders" {
			t.Errorf("expected trimmed paths, got %v", result)
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
		ErrorClass:   class,
		Error:        message,
		ErrorFile:    file,
		ErrorLine:    line,
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

func TestMarkIssueSeen_ClearsUnseenInListing(t *testing.T) {
	db := setupIssueTestDB(t)

	makeErrorEvent(t, db, 1, "TypeError", "undefined is not a function", "app.ts", 42)

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
