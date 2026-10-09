package events

import (
	"testing"
	"time"

	"gorm.io/gorm"
)

func TestGetIssueReach(t *testing.T) {
	occur := func(t *testing.T, db *gorm.DB, at time.Time, user, release string) string {
		t.Helper()
		ctx := EventContext{ErrorClass: "TypeError", Error: "boom", ErrorFile: "app.ts", ErrorLine: 1, User: user, Release: release}
		event := &Event{APIKeyID: 1, Timestamp: at, IsError: true, Message: "boom"}
		if err := event.SetContext(&ctx); err != nil {
			t.Fatalf("SetContext: %v", err)
		}
		PopulateErrorFields(event, &ctx)
		if err := Create(db, event); err != nil {
			t.Fatalf("Create event: %v", err)
		}
		return event.IssueHash
	}
	now := time.Now()

	t.Run("counts distinct users and names the first and last release", func(t *testing.T) {
		db := newTestDB(t)
		hash := occur(t, db, now.Add(-3*time.Hour), "1", "a1b2c3d")
		occur(t, db, now.Add(-2*time.Hour), "2", "a1b2c3d")
		occur(t, db, now.Add(-1*time.Hour), "2", "e4f5a6b")

		reach, err := GetIssueReach(db, hash, 1)

		if err != nil {
			t.Fatalf("GetIssueReach: %v", err)
		}
		want := IssueReach{Users: 2, Releases: 2, FirstRelease: "a1b2c3d", LastRelease: "e4f5a6b"}
		if reach != want {
			t.Errorf("expected %+v, got %+v", want, reach)
		}
	})

	t.Run("is empty when the events carry no user or release", func(t *testing.T) {
		db := newTestDB(t)
		hash := occur(t, db, now, "", "")

		reach, err := GetIssueReach(db, hash, 1)

		if err != nil {
			t.Fatalf("GetIssueReach: %v", err)
		}
		if reach != (IssueReach{}) {
			t.Errorf("expected nothing, got %+v", reach)
		}
	})
}
