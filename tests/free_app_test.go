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
	"lognorth/internal/license"
)

// LogNorth is free for one app. A license adds unlimited apps.
func TestFreeForOneApp(t *testing.T) {
	unlicensed := func(t *testing.T) {
		t.Helper()
		license.SetConfigured(false)
		t.Cleanup(func() { license.SetConfigured(false) })
	}
	addApp := func(sa *securityApp, cookie, name string) *http.Response {
		return sa.do("POST", "/apps/api-keys", url.Values{"name": {name}},
			map[string]string{"Cookie": cookie, "Sec-Fetch-Site": "same-origin"})
	}
	activeApps := func(sa *securityApp) int64 {
		var n int64
		sa.db.Model(&apikeys.APIKey{}).Where("active = ?", true).Count(&n)
		return n
	}
	flashOf := func(res *http.Response) string {
		for _, c := range res.Cookies() {
			if c.Name == "flash" {
				return c.Value
			}
		}
		return ""
	}

	t.Run("the first app is free", func(t *testing.T) {
		sa := setupSecurityApp(t)
		unlicensed(t)
		cookie := sa.login()

		res := addApp(sa, cookie, "shop")

		if res.Header.Get("Location") != "/apps" {
			t.Errorf("Location = %q, want back to /apps", res.Header.Get("Location"))
		}
		if n := activeApps(sa); n != 1 {
			t.Errorf("active apps = %d, want 1", n)
		}
	})

	t.Run("a second app needs a license", func(t *testing.T) {
		sa := setupSecurityApp(t)
		unlicensed(t)
		cookie := sa.login()
		addApp(sa, cookie, "shop")

		res := addApp(sa, cookie, "blog")

		if n := activeApps(sa); n != 1 {
			t.Errorf("active apps = %d, want still 1", n)
		}
		if flashOf(res) == "" {
			t.Error("no message tells the owner why the app was not added")
		}
	})

	t.Run("the apps page offers the license instead of a second app", func(t *testing.T) {
		sa := setupSecurityApp(t)
		unlicensed(t)
		cookie := sa.login()
		addApp(sa, cookie, "shop")

		res := sa.do("GET", "/apps", nil, map[string]string{"Cookie": cookie})

		body, _ := io.ReadAll(res.Body)
		if !strings.Contains(string(body), `"canAddApp":false`) {
			t.Errorf("page props do not say a second app needs a license")
		}
	})

	t.Run("with a license there is no limit", func(t *testing.T) {
		sa := setupSecurityApp(t)
		license.SetConfigured(true)
		t.Cleanup(func() { license.SetConfigured(false) })
		cookie := sa.login()

		for i := range 3 {
			addApp(sa, cookie, fmt.Sprintf("app-%d", i))
		}

		if n := activeApps(sa); n != 3 {
			t.Errorf("active apps = %d, want 3", n)
		}
	})

	t.Run("after revoking the free app, a new one can be added", func(t *testing.T) {
		sa := setupSecurityApp(t)
		unlicensed(t)
		cookie := sa.login()
		addApp(sa, cookie, "shop")
		shop, _ := apikeys.FindByID(sa.db, 1)
		apikeys.Revoke(sa.db, shop.ID)

		addApp(sa, cookie, "shop-v2")

		if n := activeApps(sa); n != 1 {
			t.Errorf("active apps = %d, want the new one", n)
		}
	})

	t.Run("an install with several apps from before keeps them all, and all are processed", func(t *testing.T) {
		sa := setupSecurityApp(t)
		unlicensed(t)
		_, shopKey, _ := apikeys.Create(sa.db, "shop")
		_, blogKey, _ := apikeys.Create(sa.db, "blog")

		for _, key := range []string{shopKey, blogKey} {
			status := postEvent(t, sa, key, `{"message":"GET / 200","context":{"path":"/","status":200}}`)
			if status != http.StatusCreated {
				t.Errorf("ingest for an existing app = %d, want 201", status)
			}
		}
		processEvents(t, sa)

		var processed int64
		sa.db.Model(&events.Event{}).Count(&processed)
		if processed != 2 {
			t.Errorf("processed events = %d, want both apps' events", processed)
		}
		if n := activeApps(sa); n != 2 {
			t.Errorf("active apps = %d, want both kept", n)
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
