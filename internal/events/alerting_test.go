package events

import (
	"strings"
	"testing"
	"time"

	"lognorth/internal/settings"
)

func TestAlertBody(t *testing.T) {
	newEvent := func(release string) *Event {
		ctx := EventContext{ErrorClass: "TypeError", Error: "boom", ErrorFile: "app.ts", ErrorLine: 7, Release: release}
		event := &Event{Message: "boom", IsError: true}
		event.SetContext(&ctx)
		PopulateErrorFields(event, &ctx)
		return event
	}

	t.Run("names the release the error came in", func(t *testing.T) {
		body := alertBody(newEvent("a1b2c3d"), "https://logs.example.com")

		for _, want := range []string{"TypeError: boom", "at app.ts:7", "in release a1b2c3d", "/lognorth:investigate"} {
			if !strings.Contains(body, want) {
				t.Errorf("expected %q in:\n%s", want, body)
			}
		}
	})

	t.Run("leaves the release out when the SDK sent none", func(t *testing.T) {
		body := alertBody(newEvent(""), "https://logs.example.com")

		if strings.Contains(body, "release") {
			t.Errorf("expected no release line, got:\n%s", body)
		}
	})
}

func TestCheckNewErrorOrRegression(t *testing.T) {
	recur := func(t *testing.T, lastOccurrence time.Time) time.Time {
		t.Helper()
		db := newTestDB(t)
		db.Create(&IssueSeen{ErrorHash: "abc", LastOccurrenceAt: lastOccurrence})
		engine := &AlertEngine{db: db, settings: &settings.Settings{}}

		if err := engine.CheckNewErrorOrRegression(&Event{IsError: true, IssueHash: "abc"}, "shop"); err != nil {
			t.Fatalf("CheckNewErrorOrRegression: %v", err)
		}

		var seen IssueSeen
		db.Where("error_hash = ?", "abc").First(&seen)
		return seen.LastOccurrenceAt
	}

	t.Run("records a repeat within a minute without a write", func(t *testing.T) {
		tenSecondsAgo := time.Now().Add(-10 * time.Second)

		if got := recur(t, tenSecondsAgo); !got.Equal(tenSecondsAgo) {
			t.Errorf("last occurrence moved to %s, want it kept at %s", got, tenSecondsAgo)
		}
	})

	t.Run("moves the last occurrence after a minute", func(t *testing.T) {
		if got := recur(t, time.Now().Add(-2*time.Minute)); time.Since(got) > 5*time.Second {
			t.Errorf("last occurrence %s, want about now", got)
		}
	})
}
