package tests

import (
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/karloscodes/cartridge"

	"lognorth/internal/apikeys"
	"lognorth/internal/events"
	"lognorth/internal/jobs"
)

// LogNorth has no limit on apps.
func TestApps(t *testing.T) {
	addApp := func(sa *securityApp, cookie, name string) *http.Response {
		return sa.do("POST", "/apps/api-keys", url.Values{"name": {name}},
			map[string]string{"Cookie": cookie, "Sec-Fetch-Site": "same-origin"})
	}
	activeApps := func(sa *securityApp) int64 {
		var n int64
		sa.db.Model(&apikeys.APIKey{}).Where("active = ?", true).Count(&n)
		return n
	}

	t.Run("an owner adds as many apps as they need", func(t *testing.T) {
		sa := setupSecurityApp(t)
		cookie := sa.login()

		for i := range 3 {
			addApp(sa, cookie, fmt.Sprintf("app-%d", i))
		}

		if n := activeApps(sa); n != 3 {
			t.Errorf("active apps = %d, want 3", n)
		}
	})

	t.Run("events from every app are processed", func(t *testing.T) {
		sa := setupSecurityApp(t)
		_, shopKey, _ := apikeys.Create(sa.db, "shop")
		_, blogKey, _ := apikeys.Create(sa.db, "blog")

		for _, key := range []string{shopKey, blogKey} {
			status := postEvent(t, sa, key, `{"message":"GET / 200","context":{"path":"/","status":200}}`)
			if status != http.StatusCreated {
				t.Errorf("ingest = %d, want 201", status)
			}
		}
		processEvents(t, sa)

		var processed int64
		sa.db.Model(&events.Event{}).Count(&processed)
		if processed != 2 {
			t.Errorf("processed events = %d, want both apps' events", processed)
		}
	})
}

// postEvent sends one event with an app key and returns the status.
func postEvent(t *testing.T, sa *securityApp, key, body string) int {
	t.Helper()
	req := httptest.NewRequest("POST", "/api/v1/events", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Content-Type", "application/json")
	res, err := sa.app.Server.Test(req)
	if err != nil {
		t.Fatalf("ingest failed: %v", err)
	}
	return res.StatusCode
}

// processEvents runs the event processor once, as the background job does.
func processEvents(t *testing.T, sa *securityApp) {
	t.Helper()
	ctx := &cartridge.JobContext{Context: t.Context(), Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), DB: sa.db}
	if err := jobs.NewEventProcessor().ProcessBatch(ctx); err != nil {
		t.Fatalf("processing failed: %v", err)
	}
}
