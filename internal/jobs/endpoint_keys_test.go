package jobs

import (
	"fmt"
	"testing"

	"lognorth/internal/apikeys"
	"lognorth/internal/events"
)

func TestEventProcessor_KeysStatsOnTheEndpoint(t *testing.T) {
	db := setupProcessorDB(t)
	key, _, _ := apikeys.Create(db, "shop")
	var batch []*events.IngestedEvent
	for id := 1; id <= 1000; id++ {
		status := 200
		if id%4 == 0 {
			status = 500
		}
		batch = append(batch, &events.IngestedEvent{
			APIKeyID: key.ID, Message: "GET /users", IsError: status >= 500,
			Context: fmt.Sprintf(`{"method":"GET","path":"/users/%d","status":%d}`, id, status),
		})
	}
	for _, slug := range []string{"hello-world", "second-post"} {
		batch = append(batch, &events.IngestedEvent{
			APIKeyID: key.ID, Message: "GET /posts",
			Context: fmt.Sprintf(`{"method":"GET","path":"/posts/%s","route":"/posts/:slug","status":200}`, slug),
		})
	}
	events.CreateIngested(db, batch)

	if err := NewEventProcessor().ProcessBatch(newJobCtx(db)); err != nil {
		t.Fatalf("ProcessBatch: %v", err)
	}

	var rows []struct {
		Path          string
		Total, Errors int64
	}
	db.Model(&events.EndpointStat{}).Select("path, SUM(total) AS total, SUM(errors) AS errors").Group("path").Order("path").Scan(&rows)
	want := "[{/posts/:slug 2 0} {/users/:id 1000 250}]"
	if got := fmt.Sprint(rows); got != want {
		t.Errorf("endpoints = %s, want %s", got, want)
	}
}
