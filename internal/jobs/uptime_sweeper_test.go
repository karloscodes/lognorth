package jobs

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"lognorth/internal/apikeys"
	"lognorth/internal/uptime"
)

func TestUptimeSweeper(t *testing.T) {
	t.Run("probes slow apps at the same time, and records each one", func(t *testing.T) {
		db := setupProcessorDB(t)
		slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			time.Sleep(300 * time.Millisecond)
		}))
		defer slow.Close()
		for i := range 5 {
			key, _, _ := apikeys.Create(db, fmt.Sprintf("app %d", i))
			apikeys.SetPingURL(db, key.ID, slow.URL)
		}
		db.Model(&apikeys.APIKey{}).Where("1 = 1").Update("created_at", time.Now().Add(-time.Hour))
		start := time.Now()

		err := NewUptimeSweeper().ProcessBatch(newJobCtx(db))

		if err != nil {
			t.Fatalf("ProcessBatch: %v", err)
		}
		// One after another, the five probes take 1.5 s.
		if took := time.Since(start); took > time.Second {
			t.Errorf("the sweep took %s, want the probes to run at the same time", took)
		}
		var recorded int64
		db.Model(&uptime.Check{}).Where("up = ?", true).Count(&recorded)
		if recorded != 5 {
			t.Errorf("recorded %d checks that are up, want 5", recorded)
		}
	})

	t.Run("marks an app down after three failed sweeps", func(t *testing.T) {
		db := setupProcessorDB(t)
		failing := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadGateway)
		}))
		defer failing.Close()
		key, _, _ := apikeys.Create(db, "shop")
		apikeys.SetPingURL(db, key.ID, failing.URL)
		db.Model(&apikeys.APIKey{}).Where("id = ?", key.ID).Update("created_at", time.Now().Add(-time.Hour))
		sweeper := NewUptimeSweeper()

		for range 3 {
			sweeper.ProcessBatch(newJobCtx(db))
		}

		app, _ := apikeys.FindByID(db, key.ID)
		if !app.IsDown || app.ConsecutiveFails != 3 {
			t.Errorf("is_down %v after %d fails, want down after 3", app.IsDown, app.ConsecutiveFails)
		}
	})
}
