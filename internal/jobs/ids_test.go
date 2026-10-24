package jobs

import (
	"fmt"
	"testing"
	"time"

	"lognorth/internal/alerts"
	"lognorth/internal/apikeys"
	"lognorth/internal/events"
)

// Fifty failed lookups that differ only by the id in the message are one
// issue, and one alert.
func TestEventProcessor_IDsInMessagesMakeOneIssue(t *testing.T) {
	db := setupProcessorDB(t)
	key, _, _ := apikeys.Create(db, "Shop")
	received := webhookAlerts(t, db)
	var batch []*events.IngestedEvent
	for id := 1000; id < 1050; id++ {
		batch = append(batch, &events.IngestedEvent{
			APIKeyID: key.ID, Message: "GET /users/:id → 404", IsError: true,
			Context: fmt.Sprintf(`{"method":"GET","path":"/users/%d","status":404,"error":"Couldn't find User with 'id'=%d","error_class":"ActiveRecord::RecordNotFound","error_file":"users_controller.rb","error_line":12}`, id, id),
		})
	}
	events.CreateIngested(db, batch)

	if err := NewEventProcessor().ProcessBatch(newJobCtx(db)); err != nil {
		t.Fatalf("ProcessBatch: %v", err)
	}
	if err := alerts.Deliver(db, time.Now().Add(time.Minute)); err != nil {
		t.Fatalf("Deliver: %v", err)
	}

	var issues []events.Issue
	db.Find(&issues)
	if len(issues) != 1 || issues[0].Count != 50 {
		t.Errorf("issues = %d (count %d), want 1 issue with 50 errors", len(issues), func() int64 {
			if len(issues) > 0 {
				return issues[0].Count
			}
			return 0
		}())
	}
	if got := received(); len(got) != 1 {
		t.Errorf("alerts = %d, want 1: %q", len(got), got)
	}
}
