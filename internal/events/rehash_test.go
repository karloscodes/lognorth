package events

import (
	"testing"
	"time"
)

// Data stored before the messages were normalized: each id made its own issue.
func TestRehashIssues(t *testing.T) {
	db := newTestDB(t)
	now := time.Now().UTC().Truncate(time.Second)
	old := func(hash, id string, at time.Time) {
		t.Helper()
		event := &Event{APIKeyID: 1, Message: "GET /users → 404", IsError: true, Timestamp: at, Context: `{"path":"/users"}`,
			ErrorClass: "ActiveRecord::RecordNotFound", ErrorMessage: "Couldn't find User with 'id'=" + id,
			ErrorFile: "users_controller.rb", ErrorLine: 12,
			ErrorHash1: hash + "-1", ErrorHash2: hash + "-2", ErrorHash3: hash, IssueHash: hash}
		if err := Create(db, event); err != nil {
			t.Fatal(err)
		}
	}
	old("old-1001", "1001", now.Add(-3*time.Hour))
	old("old-1002", "1002", now.Add(-2*time.Hour))
	old("old-1003", "1003", now.Add(-time.Hour))
	apart := &Event{APIKeyID: 1, Message: "failed", IsError: true, Timestamp: now,
		Context: `{"error_class":"PG::ConnectionBad","error":"connection refused"}`}
	Create(db, apart)
	if err := RecountAllIssues(db); err != nil {
		t.Fatal(err)
	}
	db.Create(&IssueSeen{ErrorHash: "old-1001", SeenAt: now.Add(-3 * time.Hour), LastOccurrenceAt: now.Add(-3 * time.Hour)})
	db.Create(&IssueSeen{ErrorHash: "old-1002", SeenAt: now.Add(-90 * time.Minute), LastOccurrenceAt: now.Add(-2 * time.Hour), Muted: true})
	_, _, want := computeErrorHashes("ActiveRecord::RecordNotFound", "Couldn't find User with 'id'=1", "users_controller.rb", 12, 1)

	if err := RehashIssues(db); err != nil {
		t.Fatalf("RehashIssues: %v", err)
	}

	t.Run("moves the events to the issue without the id", func(t *testing.T) {
		var hashes []string
		db.Model(&Event{}).Where("error_class = ?", "ActiveRecord::RecordNotFound").Distinct("issue_hash").Pluck("issue_hash", &hashes)
		if len(hashes) != 1 || hashes[0] != want {
			t.Errorf("issue hashes = %v, want only %s", hashes, want)
		}
	})

	t.Run("keeps one issue for them, and the other issues as they were", func(t *testing.T) {
		var issues []Issue
		db.Order("hash").Find(&issues)
		if len(issues) != 2 {
			t.Fatalf("issues = %+v, want the merged one and the other one", issues)
		}
		merged, _ := findIssue(db, want)
		if merged == nil || merged.Count != 3 || !merged.FirstSeen.Equal(now.Add(-3*time.Hour)) {
			t.Errorf("merged issue = %+v, want 3 errors since 3 hours ago", merged)
		}
		if other, _ := findIssue(db, apart.IssueHash); other == nil || other.Count != 1 {
			t.Errorf("the issue without an id = %+v, want it as it was", other)
		}
	})

	t.Run("merges the seen state: the latest times, and muted if any was", func(t *testing.T) {
		var seen []IssueSeen
		db.Find(&seen)
		if len(seen) != 1 || seen[0].ErrorHash != want {
			t.Fatalf("issues_seen = %+v, want one row for %s", seen, want)
		}
		if !seen[0].Muted || !seen[0].SeenAt.Equal(now.Add(-90*time.Minute)) || !seen[0].LastOccurrenceAt.Equal(now.Add(-2*time.Hour)) {
			t.Errorf("seen = %+v, want muted, seen 90 min ago, last occurrence 2 h ago", seen[0])
		}
	})

	t.Run("a second run changes nothing", func(t *testing.T) {
		if err := RehashIssues(db); err != nil {
			t.Fatal(err)
		}
		var issues, seen int64
		db.Model(&Issue{}).Count(&issues)
		db.Model(&IssueSeen{}).Count(&seen)
		if issues != 2 || seen != 1 {
			t.Errorf("issues %d, seen %d after a second run, want 2 and 1", issues, seen)
		}
	})
}
