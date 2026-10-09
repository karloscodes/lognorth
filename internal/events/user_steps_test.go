package events

import (
	"testing"
	"time"

	"gorm.io/gorm"
)

func TestStepsBefore(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	request := func(t *testing.T, db *gorm.DB, user, path string, at time.Time) *Event {
		t.Helper()
		ctx := EventContext{Method: "GET", Path: path, Status: 200, User: user}
		event := &Event{APIKeyID: 1, Timestamp: at, Message: "GET " + path}
		event.SetContext(&ctx)
		if err := Create(db, event); err != nil {
			t.Fatalf("Create event: %v", err)
		}
		return event
	}
	failure := func(t *testing.T, db *gorm.DB, user string, at time.Time) *Event {
		t.Helper()
		ctx := EventContext{Error: "boom", ErrorClass: "NoMethodError", User: user}
		event := &Event{APIKeyID: 1, Timestamp: at, IsError: true, Message: "boom"}
		event.SetContext(&ctx)
		if err := Create(db, event); err != nil {
			t.Fatalf("Create event: %v", err)
		}
		return event
	}

	t.Run("lists the user's requests before the error, oldest first", func(t *testing.T) {
		db := newTestDB(t)
		request(t, db, "42", "/cart", now.Add(-3*time.Minute))
		request(t, db, "7", "/admin", now.Add(-2*time.Minute)) // another user
		request(t, db, "42", "/checkout", now.Add(-time.Minute))
		request(t, db, "42", "/old", now.Add(-time.Hour)) // too long before
		request(t, db, "42", "/later", now.Add(time.Minute))
		// An error inside a request carries the path but no method: not a step.
		inside := &Event{APIKeyID: 1, Timestamp: now.Add(-30 * time.Second), IsError: true, Message: "boom"}
		inside.SetContext(&EventContext{Error: "boom", Path: "/checkout", User: "42"})
		Create(db, inside)
		err := failure(t, db, "42", now)

		steps := StepsBefore(db, err)

		if len(steps) != 2 || steps[0].Message != "GET /cart" || steps[1].Message != "GET /checkout" {
			t.Fatalf("expected /cart then /checkout, got %+v", steps)
		}
	})

	t.Run("finds nothing when the error has no user", func(t *testing.T) {
		db := newTestDB(t)
		request(t, db, "", "/cart", now.Add(-time.Minute))

		if steps := StepsBefore(db, failure(t, db, "", now)); len(steps) != 0 {
			t.Fatalf("expected no steps, got %+v", steps)
		}
	})
}
