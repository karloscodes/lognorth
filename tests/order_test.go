package tests

import (
	"testing"
	"time"

	"lognorth/internal/apikeys"
	"lognorth/internal/events"
)

// The log shows events by the time they happened, not when they arrived.
func TestLogOrdersByEventTime(t *testing.T) {
	setup := func(t *testing.T) (*securityApp, uint) {
		t.Helper()
		sa := setupSecurityApp(t)
		app, _, _ := apikeys.Create(sa.db, "shop")
		return sa, app.ID
	}
	messages := func(list []events.Event) []string {
		out := make([]string, len(list))
		for i, e := range list {
			out[i] = e.Message
		}
		return out
	}

	t.Run("a batch that arrives late after an outage sits at its own time, not on top", func(t *testing.T) {
		sa, app := setup(t)
		now := time.Now().UTC()
		events.Create(sa.db, &events.Event{APIKeyID: app, Message: "logged after the outage", Timestamp: now})
		events.Create(sa.db, &events.Event{APIKeyID: app, Message: "logged during the outage, delivered late", Timestamp: now.Add(-10 * time.Minute)})

		list, _, err := events.List(sa.db, events.ListFilters{APIKeyID: app, Limit: 10})

		if err != nil {
			t.Fatalf("list failed: %v", err)
		}
		want := []string{"logged after the outage", "logged during the outage, delivered late"}
		if got := messages(list); got[0] != want[0] || got[1] != want[1] {
			t.Errorf("order = %v, want %v", got, want)
		}
	})

	t.Run("events with the same time keep the order they were logged in", func(t *testing.T) {
		sa, app := setup(t)
		same := time.Now().UTC().Truncate(time.Millisecond)
		for _, m := range []string{"first", "second", "third"} {
			events.Create(sa.db, &events.Event{APIKeyID: app, Message: m, Timestamp: same})
		}

		list, _, _ := events.List(sa.db, events.ListFilters{APIKeyID: app, Limit: 10})

		got := messages(list)
		if len(got) != 3 || got[0] != "third" || got[1] != "second" || got[2] != "first" {
			t.Errorf("order = %v, want newest logged first: [third second first]", got)
		}
	})
}
