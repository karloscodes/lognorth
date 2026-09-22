package tests

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"gorm.io/gorm"

	"lognorth/internal/events"
	"lognorth/internal/settings"
)

// ntfyInbox is a local ntfy server that records the titles it receives.
type ntfyInbox struct {
	mu     sync.Mutex
	titles []string
}

func (in *ntfyInbox) received() []string {
	in.mu.Lock()
	defer in.mu.Unlock()
	return append([]string(nil), in.titles...)
}

// setupErrorAlerts returns a database whose notifications go to a local ntfy inbox.
func setupErrorAlerts(t *testing.T) (*gorm.DB, *ntfyInbox) {
	t.Helper()
	db := setupTestDB(t)

	inbox := &ntfyInbox{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		inbox.mu.Lock()
		inbox.titles = append(inbox.titles, r.Header.Get("Title"))
		inbox.mu.Unlock()
	}))
	t.Cleanup(srv.Close)

	s, err := settings.Get(db)
	if err != nil {
		t.Fatalf("settings: %v", err)
	}
	s.NotifyChannel = "ntfy"
	s.NtfyURL = srv.URL
	s.QuietHoursEnabled = false
	if err := settings.Update(db, s); err != nil {
		t.Fatalf("update settings: %v", err)
	}
	return db, inbox
}

// occur stores one error event and runs the per-error alert check on it.
func occur(t *testing.T, db *gorm.DB, context string) *events.Event {
	t.Helper()
	event := &events.Event{APIKeyID: 1, Message: "POST /pay 500", IsError: true, Context: context}
	if err := events.Create(db, event); err != nil {
		t.Fatalf("create event: %v", err)
	}
	engine, err := events.NewAlertEngine(db)
	if err != nil {
		t.Fatalf("alert engine: %v", err)
	}
	if err := engine.CheckNewErrorOrRegression(event, "shop"); err != nil {
		t.Fatalf("alert check: %v", err)
	}
	return event
}

const cardDeclined = `{"error":"card declined","error_class":"PaymentError","error_file":"pay.rb","error_line":41}`

func TestErrorAlerts(t *testing.T) {
	t.Run("a new issue alerts once", func(t *testing.T) {
		db, inbox := setupErrorAlerts(t)

		occur(t, db, cardDeclined)
		occur(t, db, cardDeclined)

		if got := inbox.received(); len(got) != 1 || !strings.Contains(got[0], "new: PaymentError: card declined") {
			t.Errorf("titles = %q, want one new-error alert", got)
		}
	})

	t.Run("the alert uses the hash the issue list shows", func(t *testing.T) {
		db, _ := setupErrorAlerts(t)

		event := occur(t, db, cardDeclined)

		issues, _ := events.ListIssuesWithState(db, 0, 10)
		if len(issues) != 1 || issues[0].Hash != event.IssueHash {
			t.Errorf("issue hash %v, want the alerted hash %s", issues, event.IssueHash)
		}
	})

	t.Run("a muted issue does not alert when it comes back", func(t *testing.T) {
		db, inbox := setupErrorAlerts(t)
		event := occur(t, db, cardDeclined)
		events.MuteIssue(db, event.IssueHash)
		db.Model(&events.IssueSeen{}).Where("error_hash = ?", event.IssueHash).
			Update("last_occurrence_at", time.Now().Add(-48*time.Hour))

		occur(t, db, cardDeclined)

		if got := inbox.received(); len(got) != 1 {
			t.Errorf("titles = %q, want only the first alert: the issue is muted", got)
		}
	})

	t.Run("an unmuted issue alerts when it comes back after a quiet day", func(t *testing.T) {
		db, inbox := setupErrorAlerts(t)
		event := occur(t, db, cardDeclined)
		db.Model(&events.IssueSeen{}).Where("error_hash = ?", event.IssueHash).
			Update("last_occurrence_at", time.Now().Add(-48*time.Hour))

		occur(t, db, cardDeclined)

		if got := inbox.received(); len(got) != 2 || !strings.Contains(got[1], "back after 2d") {
			t.Errorf("titles = %q, want a regression alert", got)
		}
	})

	t.Run("errors with a class and no message are separate issues", func(t *testing.T) {
		db, inbox := setupErrorAlerts(t)

		occur(t, db, `{"error_class":"TimeoutError"}`)
		occur(t, db, `{"error_class":"ParseError"}`)

		if got := inbox.received(); len(got) != 2 {
			t.Errorf("titles = %q, want two new-error alerts", got)
		}
	})

	t.Run("a multi-line error message gives a one-line title", func(t *testing.T) {
		db, inbox := setupErrorAlerts(t)

		occur(t, db, `{"error":"undefined method 'id'\nfor nil:NilClass","error_class":"NoMethodError"}`)

		got := inbox.received()
		if len(got) != 1 || strings.Contains(got[0], "\n") || !strings.Contains(got[0], "undefined method 'id'") {
			t.Errorf("titles = %q, want one single-line alert", got)
		}
	})

	t.Run("setting up a channel later does not alert on known issues", func(t *testing.T) {
		db, inbox := setupErrorAlerts(t)
		s, _ := settings.Get(db)
		channel := s.NotifyChannel
		s.NotifyChannel = ""
		settings.Update(db, s)
		occur(t, db, cardDeclined)

		s.NotifyChannel = channel
		settings.Update(db, s)
		occur(t, db, cardDeclined)

		if got := inbox.received(); len(got) != 0 {
			t.Errorf("titles = %q, want no alert for an issue seen before the channel existed", got)
		}
	})
}
