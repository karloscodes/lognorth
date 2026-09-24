package tests

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"gorm.io/gorm"

	"lognorth/internal"
	"lognorth/internal/apikeys"
	"lognorth/internal/config"
	"lognorth/internal/events"
	"lognorth/internal/settings"
	"lognorth/internal/users"
)

// =============================================================================
// SECURITY TESTS
// These build the real application with every route and middleware, because
// the bugs they guard were in the route wiring, not in the handlers.
// =============================================================================

type securityApp struct {
	t   *testing.T
	app *internal.Application
	db  *gorm.DB
}

func setupSecurityApp(t *testing.T) *securityApp {
	t.Helper()

	t.Setenv("LOGNORTH_ENV", "test")
	t.Setenv("LOGNORTH_STORAGE_PATH", t.TempDir())
	config.Reset()
	t.Cleanup(config.Reset)

	app, err := internal.NewApp()
	if err != nil {
		t.Fatalf("Failed to build the app: %v", err)
	}

	return &securityApp{t: t, app: app, db: app.DBManager.GetConnection()}
}

// do sends one request through the real router. A zero-value header map means
// no extra headers.
func (sa *securityApp) do(method, path string, form url.Values, headers map[string]string) *http.Response {
	sa.t.Helper()

	var body io.Reader
	if form != nil {
		body = strings.NewReader(form.Encode())
	}
	req := httptest.NewRequest(method, path, body)
	if form != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	res, err := sa.app.Server.App().Test(req, -1)
	if err != nil {
		sa.t.Fatalf("%s %s failed: %v", method, path, err)
	}
	return res
}

// login creates the owner and returns their session cookie.
func (sa *securityApp) login() string {
	sa.t.Helper()

	if err := users.CreateUser(sa.db, "owner@example.com", "correct-horse"); err != nil {
		sa.t.Fatalf("Failed to create the owner: %v", err)
	}
	res := sa.do("POST", "/login",
		url.Values{"email": {"owner@example.com"}, "password": {"correct-horse"}},
		map[string]string{"Sec-Fetch-Site": "same-origin"})

	for _, c := range res.Cookies() {
		if c.Name == "lognorth_session" && c.Value != "" {
			return c.Name + "=" + c.Value
		}
	}
	sa.t.Fatalf("login returned %d and no session cookie", res.StatusCode)
	return ""
}

func TestReadAPIRequiresLogin(t *testing.T) {
	t.Run("without a session, events are not served", func(t *testing.T) {
		sa := setupSecurityApp(t)
		sa.login()

		res := sa.do("GET", "/api/v1/events", nil, nil)

		if res.StatusCode == http.StatusOK {
			body, _ := io.ReadAll(res.Body)
			t.Fatalf("GET /api/v1/events without a session returned 200: %s", body)
		}
		if loc := res.Header.Get("Location"); loc != "/login" {
			t.Errorf("Location = %q, want the login page", loc)
		}
	})

	t.Run("with a session, events are served", func(t *testing.T) {
		sa := setupSecurityApp(t)
		cookie := sa.login()

		res := sa.do("GET", "/api/v1/events", nil, map[string]string{"Cookie": cookie})

		if res.StatusCode != http.StatusOK {
			t.Fatalf("status = %d, want 200 for the logged-in owner", res.StatusCode)
		}
		var payload map[string]any
		if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
			t.Fatalf("response is not JSON: %v", err)
		}
		if _, ok := payload["events"]; !ok {
			t.Errorf("payload = %v, want an events list", payload)
		}
	})

	t.Run("the old read routes are gone", func(t *testing.T) {
		sa := setupSecurityApp(t)
		sa.login()

		for _, path := range []string{"/api/v1/issues", "/api/v1/api-keys", "/api/v1/events/live", "/api/v1/events/1"} {
			res := sa.do("GET", path, nil, nil)

			if res.StatusCode == http.StatusOK {
				t.Errorf("GET %s without a session returned 200, want it gone", path)
			}
		}
	})
}

func TestCrossSiteWritesAreBlocked(t *testing.T) {
	t.Run("a cross-site form post to settings is rejected", func(t *testing.T) {
		sa := setupSecurityApp(t)
		cookie := sa.login()

		res := sa.do("POST", "/settings/agent-key/regenerate", url.Values{},
			map[string]string{"Cookie": cookie, "Sec-Fetch-Site": "cross-site"})

		if res.StatusCode != http.StatusForbidden {
			t.Errorf("status = %d, want 403 for a cross-site post", res.StatusCode)
		}
	})

	t.Run("a same-origin post is allowed", func(t *testing.T) {
		sa := setupSecurityApp(t)
		cookie := sa.login()

		res := sa.do("POST", "/settings/agent-key/regenerate", url.Values{},
			map[string]string{"Cookie": cookie, "Sec-Fetch-Site": "same-origin"})

		if res.StatusCode == http.StatusForbidden {
			t.Errorf("status = 403, want same-origin posts to pass")
		}
	})

	t.Run("SDK ingest without browser headers still works", func(t *testing.T) {
		sa := setupSecurityApp(t)

		res := sa.do("POST", "/api/v1/events", url.Values{}, map[string]string{"Authorization": "Bearer lgn-wrong"})

		if res.StatusCode == http.StatusForbidden {
			t.Errorf("status = 403, want ingest to skip the browser check (a wrong key is a 401)")
		}
	})
}

func TestSetup(t *testing.T) {
	sameOrigin := map[string]string{"Sec-Fetch-Site": "same-origin"}
	owner := url.Values{"email": {"owner@example.com"}, "password": {"correct-horse"}, "confirm_password": {"correct-horse"}}

	t.Run("creates the owner and logs them in", func(t *testing.T) {
		sa := setupSecurityApp(t)

		res := sa.do("POST", "/setup", owner, sameOrigin)

		if loc := res.Header.Get("Location"); loc != "/" {
			t.Errorf("Location = %q, want the home page after setup", loc)
		}
		if _, err := users.FindByEmail(sa.db, "owner@example.com"); err != nil {
			t.Errorf("owner was not created: %v", err)
		}
	})

	t.Run("keeps the email when the passwords do not match", func(t *testing.T) {
		sa := setupSecurityApp(t)
		form := url.Values{"email": {"owner@example.com"}, "password": {"correct-horse"}, "confirm_password": {"other-horse"}}

		res := sa.do("POST", "/setup", form, sameOrigin)

		body, _ := io.ReadAll(res.Body)
		if !strings.Contains(string(body), "Passwords do not match") || !strings.Contains(string(body), "owner@example.com") {
			t.Errorf("body does not show the error with the email kept")
		}
		var count int64
		sa.db.Model(&users.User{}).Count(&count)
		if count != 0 {
			t.Errorf("users = %d, want none", count)
		}
	})

	t.Run("refuses a second admin once the owner exists", func(t *testing.T) {
		sa := setupSecurityApp(t)
		sa.do("POST", "/setup", owner, sameOrigin)

		attacker := url.Values{"email": {"attacker@example.com"}, "password": {"attacker-pass"}, "confirm_password": {"attacker-pass"}}
		res := sa.do("POST", "/setup", attacker, sameOrigin)

		var count int64
		sa.db.Model(&users.User{}).Count(&count)
		if count != 1 {
			t.Errorf("users = %d, want only the owner", count)
		}
		if loc := res.Header.Get("Location"); loc != "/login" {
			t.Errorf("Location = %q, want /login once setup is done", loc)
		}
	})
}

func TestSettingsJSONHidesSecrets(t *testing.T) {
	s := settings.Settings{SMTPPassword: "smtp-secret", TelegramBotToken: "123:bot-secret", AgentKey: "lgn-agent-secret"}

	out, err := json.Marshal(s)

	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}
	for _, secret := range []string{"smtp-secret", "bot-secret", "lgn-agent-secret"} {
		if strings.Contains(string(out), secret) {
			t.Errorf("settings JSON contains %q: %s", secret, out)
		}
	}
}

func TestIngestLimitsBatchSize(t *testing.T) {
	sa := setupSecurityApp(t)
	var key string
	if _, k, err := apikeys.Create(sa.db, "shop"); err == nil {
		key = k
	}
	events := strings.Repeat(`{"message":"GET / 200"},`, 1001)
	body := `{"events":[` + strings.TrimSuffix(events, ",") + `]}`

	req := httptest.NewRequest("POST", "/api/v1/events/batch", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Content-Type", "application/json")
	res, err := sa.app.Server.App().Test(req, -1)

	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if res.StatusCode != http.StatusRequestEntityTooLarge {
		t.Errorf("status = %d, want 413 for 1,001 events", res.StatusCode)
	}
}

func TestAppPages(t *testing.T) {
	t.Run("the events poll returns only the app it asks for", func(t *testing.T) {
		sa := setupSecurityApp(t)
		cookie := sa.login()
		shop, _, _ := apikeys.Create(sa.db, "shop")
		blog, _, _ := apikeys.Create(sa.db, "blog")
		events.Create(sa.db, &events.Event{APIKeyID: shop.ID, Message: "GET /cart 200"})
		events.Create(sa.db, &events.Event{APIKeyID: blog.ID, Message: "GET /post 200"})

		res := sa.do("GET", fmt.Sprintf("/api/v1/events?app_id=%d", shop.ID), nil, map[string]string{"Cookie": cookie})

		var payload struct {
			Events []events.Event `json:"events"`
		}
		json.NewDecoder(res.Body).Decode(&payload)
		if len(payload.Events) != 1 || payload.Events[0].APIKeyID != shop.ID {
			t.Errorf("events = %+v, want only the shop event", payload.Events)
		}
	})

	t.Run("the delete form removes the app", func(t *testing.T) {
		sa := setupSecurityApp(t)
		cookie := sa.login()
		shop, _, _ := apikeys.Create(sa.db, "shop")

		sa.do("POST", fmt.Sprintf("/apps/api-keys/%d/delete", shop.ID), url.Values{},
			map[string]string{"Cookie": cookie, "Sec-Fetch-Site": "same-origin"})

		if _, err := apikeys.FindByID(sa.db, shop.ID); err == nil {
			t.Error("the app still exists after the delete form")
		}
	})
}

func TestLoginLastsNinetyDays(t *testing.T) {
	sa := setupSecurityApp(t)
	users.CreateUser(sa.db, "owner@example.com", "correct-horse")

	res := sa.do("POST", "/login",
		url.Values{"email": {"owner@example.com"}, "password": {"correct-horse"}},
		map[string]string{"Sec-Fetch-Site": "same-origin"})

	for _, c := range res.Cookies() {
		if c.Name != "lognorth_session" {
			continue
		}
		if days := c.MaxAge / 86400; days != 90 {
			t.Errorf("session lasts %d days, want 90", days)
		}
		return
	}
	t.Fatal("login set no session cookie")
}

func TestDemoHidesAPIKeys(t *testing.T) {
	t.Setenv("LOGNORTH_DEMO", "true")
	sa := setupSecurityApp(t)
	users.CreateUser(sa.db, "demo@lognorth.com", "demo")
	_, fullKey, _ := apikeys.Create(sa.db, "shop")

	res := sa.do("GET", "/apps", nil, nil)

	body, _ := io.ReadAll(res.Body)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("GET /apps = %d, want the page", res.StatusCode)
	}
	if strings.Contains(string(body), fullKey) {
		t.Error("the demo Apps page shows a working ingest key")
	}
}

func TestRevealAgentKey(t *testing.T) {
	sameOrigin := func(cookie string) map[string]string {
		return map[string]string{"Cookie": cookie, "Sec-Fetch-Site": "same-origin"}
	}

	t.Run("returns the key to the logged-in owner", func(t *testing.T) {
		sa := setupSecurityApp(t)
		cookie := sa.login()
		key, _ := settings.CreateAgentKey(sa.db)

		res := sa.do("POST", "/settings/agent-key/reveal", url.Values{}, sameOrigin(cookie))

		var payload struct{ Key string }
		json.NewDecoder(res.Body).Decode(&payload)
		if res.StatusCode != http.StatusOK || payload.Key != key {
			t.Errorf("status %d, key %q, want 200 and the agent key", res.StatusCode, payload.Key)
		}
	})

	t.Run("needs a login", func(t *testing.T) {
		sa := setupSecurityApp(t)
		sa.login()
		settings.CreateAgentKey(sa.db)

		res := sa.do("POST", "/settings/agent-key/reveal", url.Values{}, map[string]string{"Sec-Fetch-Site": "same-origin"})

		body, _ := io.ReadAll(res.Body)
		if strings.Contains(string(body), "lgn-agent-") {
			t.Errorf("status %d: the key was returned without a session", res.StatusCode)
		}
	})

	t.Run("refuses a cross-site request", func(t *testing.T) {
		sa := setupSecurityApp(t)
		cookie := sa.login()
		settings.CreateAgentKey(sa.db)

		res := sa.do("POST", "/settings/agent-key/reveal", url.Values{}, map[string]string{"Cookie": cookie, "Sec-Fetch-Site": "cross-site"})

		if res.StatusCode != http.StatusForbidden {
			t.Errorf("status = %d, want 403", res.StatusCode)
		}
	})
}
