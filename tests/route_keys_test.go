package tests

import (
	"fmt"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/karloscodes/cartridge"

	"lognorth/internal/apikeys"
	"lognorth/internal/events"
	"lognorth/internal/jobs"
)

// A route with an id in it is one endpoint: the processor keys its requests
// on /users/:id, so the endpoint has a history to alert against.
func TestARouteWithIDsAlertsAsOneEndpoint(t *testing.T) {
	tdb := setupAlertingTestDB(t)
	app, _, _ := apikeys.Create(tdb.db, "Shop")
	tdb.seedDays(app.ID, "/users/:id", 200, 2)
	bucket := events.LastCompleteBucketTime()
	var batch []*events.IngestedEvent
	for id := 1; id <= 200; id++ {
		status := 200
		if id%2 == 0 {
			status = 500
		}
		batch = append(batch, &events.IngestedEvent{
			APIKeyID: app.ID, Message: "GET /users", IsError: status >= 500, Timestamp: bucket.Add(time.Minute),
			Context: fmt.Sprintf(`{"method":"GET","path":"/users/%d","status":%d}`, id, status),
		})
	}
	events.CreateIngested(tdb.db, batch)
	ctx := &cartridge.JobContext{Context: t.Context(), Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), DB: tdb.db}
	if err := jobs.NewEventProcessor().ProcessBatch(ctx); err != nil {
		t.Fatalf("processor: %v", err)
	}

	tdb.runAnomalyChecker()

	if state := tdb.getAlertState(app.ID, "/users/:id"); state.Severity != "critical" {
		t.Errorf("/users/:id at 50%% errors on a 1%% history: severity %q, want critical", state.Severity)
	}
}
