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

// onboardingCookie opens /setup and returns the onboarding session cookie.
func (sa *securityApp) onboardingCookie() string {
	sa.t.Helper()

	res := sa.do("GET", "/setup", nil, nil)
	for _, c := range res.Cookies() {
		if c.Name == "_onboarding_session" {
			return c.Name + "=" + c.Value
		}
	}
	sa.t.Fatal("GET /setup set no onboarding cookie")
	return ""
}

func TestSetupCreatesTheOwner(t *testing.T) {
	sa := setupSecurityApp(t)
	headers := map[string]string{"Sec-Fetch-Site": "same-origin", "Cookie": sa.onboardingCookie()}

	sa.do("POST", "/setup/user", url.Values{"email": {"owner@example.com"}}, headers)
	res := sa.do("POST", "/setup/password",
		url.Values{"password": {"correct-horse"}, "confirm_password": {"correct-horse"}}, headers)

	if loc := res.Header.Get("Location"); loc != "/" {
		t.Errorf("Location = %q, want the home page after setup", loc)
	}
	if _, err := users.FindByEmail(sa.db, "owner@example.com"); err != nil {
		t.Errorf("owner was not created: %v", err)
	}
}

func TestSetupClosesAfterTheFirstUser(t *testing.T) {
	sa := setupSecurityApp(t)

	// An attacker opens /setup on the fresh install and keeps the cookie.
	attacker := sa.onboardingCookie()

	// The owner finishes setup.
	if err := users.CreateUser(sa.db, "owner@example.com", "correct-horse"); err != nil {
		t.Fatalf("Failed to create the owner: %v", err)
	}

	// The attacker replays the setup steps with the old cookie.
	withCookie := map[string]string{"Sec-Fetch-Site": "same-origin", "Cookie": attacker}
	sa.do("POST", "/setup/user", url.Values{"email": {"attacker@example.com"}}, withCookie)
	res := sa.do("POST", "/setup/password",
		url.Values{"password": {"attacker-pass"}, "confirm_password": {"attacker-pass"}}, withCookie)

	var count int64
	sa.db.Model(&users.User{}).Count(&count)
	if count != 1 {
		t.Errorf("users = %d, want only the owner", count)
	}
	if loc := res.Header.Get("Location"); loc != "/login" {
		t.Errorf("Location = %q, want /login once setup is done", loc)
	}
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
