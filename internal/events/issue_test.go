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
