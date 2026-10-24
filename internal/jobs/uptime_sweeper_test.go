package jobs

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"lognorth/internal/apikeys"
	"lognorth/internal/events"
	"lognorth/internal/uptime"
)

func TestDownBody(t *testing.T) {
	key := &apikeys.APIKey{ID: 4, Name: "shop", PingURL: "https://shop.example.com/up"}

	t.Run("names the issues that surged and points the agent at the first", func(t *testing.T) {
		surges := []events.Surge{
			{Hash: "abc", ErrorClass: "Redis::CannotConnectError", Error: "connection refused", Count: 12},
			{Hash: "def", Error: "timeout", Count: 4},
		}

		body := downBody(key, "https://logs.example.com", surges)

		for _, want := range []string{
			"shop isn't responding.",
			"12x Redis::CannotConnectError: connection refused",
			"https://logs.example.com/issues/abc",
			"4x timeout",
			"/lognorth:investigate abc",
		} {
			if !strings.Contains(body, want) {
				t.Errorf("expected %q in:\n%s", want, body)
			}
		}
	})

	t.Run("says nothing about errors when none surged", func(t *testing.T) {
		body := downBody(key, "https://logs.example.com", nil)

		if strings.Contains(body, "surged") || strings.Contains(body, "investigate") {
			t.Errorf("expected no error section, got:\n%s", body)
		}
		if !strings.Contains(body, "https://logs.example.com/apps/4/endpoints") {
			t.Errorf("expected a link to the app's endpoints, got:\n%s", body)
		}
	})
}

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
