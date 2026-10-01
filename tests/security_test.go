package tests

import (
	"encoding/base64"
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
	return setupSecurityAppIn(t, "test")
}

// setupProductionApp builds the app as a server runs it: rate limits on,
// Secure cookies. Production logs to ./logs, so it runs in a temp directory.
func setupProductionApp(t *testing.T) *securityApp {
	t.Helper()
	t.Setenv("LOGNORTH_PRIVATE_KEY", strings.Repeat("k", 64))
	t.Chdir(t.TempDir())
	return setupSecurityAppIn(t, "production")
}

func setupSecurityAppIn(t *testing.T, env string) *securityApp {
	t.Helper()

	t.Setenv("LOGNORTH_ENV", env)
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

	res, err := sa.app.Server.Test(req)
	if err != nil {
		sa.t.Fatalf("%s %s failed: %v", method, path, err)
	}
	return res
}

// The proxy sits on the Docker network; a client that skips it does not.
const (
	proxyPeer  = "10.0.0.5:41000"
	directPeer = "198.51.100.7:41000"
)

// doFrom sends one request from a given connection address, which decides
// whether X-Forwarded-For counts.
func (sa *securityApp) doFrom(peer, method, path string, form url.Values, headers map[string]string) *http.Response {
	sa.t.Helper()

	var body io.Reader
	if form != nil {
		body = strings.NewReader(form.Encode())
	}
	req := httptest.NewRequest(method, path, body)
	req.RemoteAddr = peer
	if form != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	res := httptest.NewRecorder()
	sa.app.Server.ServeHTTP(res, req)

	return res.Result()
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
	s := settings.Settings{SMTPPassword: "smtp-secret", TelegramBotToken: "123:bot-secret", AgentKey: "lgn-agent-secret", WebhookToken: "crsr_webhook-secret"}

	out, err := json.Marshal(s)

	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}
	for _, secret := range []string{"smtp-secret", "bot-secret", "lgn-agent-secret", "webhook-secret"} {
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
	res, err := sa.app.Server.Test(req)

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

func TestProtectedRoutesRequireLogin(t *testing.T) {
	pages := []string{"/", "/apps", "/log", "/issues", "/apps/1/issues", "/apps/1/log", "/apps/1/log/1",
		"/apps/1/endpoints", "/trace/abc", "/apps/1/issues/abc", "/issues/abc", "/settings", "/settings/license",
		"/settings/alerts", "/settings/data", "/settings/developer", "/api/v1/events"}
	writes := []string{"/logout", "/apps/api-keys", "/apps/api-keys/1/rename", "/apps/api-keys/1/ping-url",
		"/apps/api-keys/1/toggle-ping", "/apps/api-keys/1/revoke", "/apps/api-keys/1/delete", "/issues/seen",
		"/issues/abc/seen", "/issues/abc/mute", "/issues/abc/unmute", "/settings", "/settings/test",
		"/settings/agent-key", "/settings/agent-key/regenerate", "/settings/agent-key/revoke"}

	t.Run("every page sends a visitor to the login page", func(t *testing.T) {
		sa := setupSecurityApp(t)
		sa.login()

		for _, path := range pages {
			res := sa.do("GET", path, nil, nil)

			if res.StatusCode != http.StatusFound || res.Header.Get("Location") != "/login" {
				t.Errorf("GET %s = %d to %q, want 302 to /login", path, res.StatusCode, res.Header.Get("Location"))
			}
		}
	})

	t.Run("a HEAD request gets the same answer as a GET", func(t *testing.T) {
		sa := setupSecurityApp(t)
		sa.login()

		res := sa.do("HEAD", "/settings/developer", nil, nil)

		if res.StatusCode != http.StatusFound || res.Header.Get("Location") != "/login" {
			t.Errorf("HEAD = %d to %q, want 302 to /login", res.StatusCode, res.Header.Get("Location"))
		}
	})

	t.Run("every write sends a visitor to the login page and changes nothing", func(t *testing.T) {
		sa := setupSecurityApp(t)
		sa.login()
		shop, _, _ := apikeys.Create(sa.db, "shop")

		for _, path := range writes {
			res := sa.do("POST", path, url.Values{"name": {"renamed"}}, map[string]string{"Sec-Fetch-Site": "same-origin"})

			if res.StatusCode != http.StatusFound || res.Header.Get("Location") != "/login" {
				t.Errorf("POST %s = %d to %q, want 302 to /login", path, res.StatusCode, res.Header.Get("Location"))
			}
		}
		if app, err := apikeys.FindByID(sa.db, shop.ID); err != nil || app.Name != "shop" {
			t.Errorf("app = %+v (%v), want the app untouched", app, err)
		}
	})

	t.Run("an app key or an agent key does not open the pages", func(t *testing.T) {
		sa := setupSecurityApp(t)
		sa.login()
		_, appKey, _ := apikeys.Create(sa.db, "shop")
		agentKey, _ := settings.CreateAgentKey(sa.db)

		for _, key := range []string{appKey, agentKey} {
			res := sa.do("GET", "/api/v1/events", nil, map[string]string{"Authorization": "Bearer " + key})

			if res.StatusCode != http.StatusFound {
				t.Errorf("status = %d, want 302: only a session reads events", res.StatusCode)
			}
		}
	})
}

func TestSessionCookie(t *testing.T) {
	open := func(sa *securityApp, cookie string) int {
		return sa.do("GET", "/settings/developer", nil, map[string]string{"Cookie": cookie}).StatusCode
	}

	t.Run("a tampered cookie is rejected", func(t *testing.T) {
		sa := setupSecurityApp(t)
		cookie := sa.login()
		last := cookie[len(cookie)-1:]
		swapped := "A"
		if last == "A" {
			swapped = "B"
		}

		status := open(sa, cookie[:len(cookie)-1]+swapped)

		if status != http.StatusFound {
			t.Errorf("status = %d, want 302 to the login page", status)
		}
		if open(sa, cookie) != http.StatusOK {
			t.Errorf("the untouched cookie must still open the page")
		}
	})

	t.Run("a forged cookie with no valid signature is rejected", func(t *testing.T) {
		sa := setupSecurityApp(t)
		sa.login()
		payload := base64.RawURLEncoding.EncodeToString([]byte(`{"user_id":"1","expires_at":"2099-01-01T00:00:00Z"}`))

		for _, forged := range []string{payload, payload + "." + strings.Repeat("A", 43), "not-a-session", ""} {
			status := open(sa, "lognorth_session="+forged)

			if status != http.StatusFound {
				t.Errorf("cookie %q: status = %d, want 302 to the login page", forged, status)
			}
		}
	})

	t.Run("a cookie signed by another install is rejected", func(t *testing.T) {
		other := setupProductionApp(t)
		cookie := other.login()
		t.Setenv("LOGNORTH_PRIVATE_KEY", strings.Repeat("z", 64))
		sa := setupSecurityAppIn(t, "production")
		users.CreateUser(sa.db, "owner@example.com", "correct-horse")

		status := open(sa, cookie)

		if status != http.StatusFound {
			t.Errorf("status = %d, want 302: the secret differs", status)
		}
	})

	t.Run("in production the session cookie is HttpOnly, Secure, and SameSite=Lax", func(t *testing.T) {
		sa := setupProductionApp(t)
		users.CreateUser(sa.db, "owner@example.com", "correct-horse")

		res := sa.do("POST", "/login",
			url.Values{"email": {"owner@example.com"}, "password": {"correct-horse"}},
			map[string]string{"Sec-Fetch-Site": "same-origin"})

		for _, c := range res.Cookies() {
			if c.Name != "lognorth_session" {
				continue
			}
			if !c.HttpOnly || !c.Secure || c.SameSite != http.SameSiteLaxMode || c.Path != "/" {
				t.Errorf("cookie = %+v, want HttpOnly, Secure, SameSite=Lax, Path=/", c)
			}
			return
		}
		t.Fatal("login set no session cookie")
	})

	t.Run("in production the flash cookie is HttpOnly and Secure", func(t *testing.T) {
		sa := setupProductionApp(t)
		users.CreateUser(sa.db, "owner@example.com", "correct-horse")

		res := sa.do("POST", "/login",
			url.Values{"email": {"owner@example.com"}, "password": {"wrong"}},
			map[string]string{"Sec-Fetch-Site": "same-origin"})

		for _, c := range res.Cookies() {
			if c.Name != "flash" {
				continue
			}
			if !c.HttpOnly || !c.Secure || c.SameSite != http.SameSiteLaxMode {
				t.Errorf("cookie = %+v, want HttpOnly, Secure, SameSite=Lax", c)
			}
			return
		}
		t.Fatal("a failed login set no flash cookie")
	})

	t.Run("after a logout, a copy of the cookie no longer opens the pages", func(t *testing.T) {
		sa := setupSecurityApp(t)
		cookie := sa.login()
		sa.do("POST", "/logout", url.Values{}, map[string]string{"Cookie": cookie, "Sec-Fetch-Site": "same-origin"})

		res := sa.do("GET", "/settings/developer", nil, map[string]string{"Cookie": cookie})

		if res.StatusCode != http.StatusFound || res.Header.Get("Location") != "/login" {
			t.Errorf("status = %d to %q, want 302 to /login", res.StatusCode, res.Header.Get("Location"))
		}
	})

	t.Run("after a logout, the login page does not bounce a copied cookie back", func(t *testing.T) {
		sa := setupSecurityApp(t)
		cookie := sa.login()
		sa.do("POST", "/logout", url.Values{}, map[string]string{"Cookie": cookie, "Sec-Fetch-Site": "same-origin"})

		res := sa.do("GET", "/login", nil, map[string]string{"Cookie": cookie})

		if res.StatusCode != http.StatusOK {
			t.Errorf("status = %d, want the login page", res.StatusCode)
		}
	})

	t.Run("a new login after a logout opens the pages", func(t *testing.T) {
		sa := setupSecurityApp(t)
		old := sa.login()
		sa.do("POST", "/logout", url.Values{}, map[string]string{"Cookie": old, "Sec-Fetch-Site": "same-origin"})
		res := sa.do("POST", "/login",
			url.Values{"email": {"owner@example.com"}, "password": {"correct-horse"}},
			map[string]string{"Sec-Fetch-Site": "same-origin"})
		fresh := ""
		for _, c := range res.Cookies() {
			if c.Name == "lognorth_session" {
				fresh = c.Name + "=" + c.Value
			}
		}

		status := open(sa, fresh)

		if status != http.StatusOK {
			t.Errorf("status = %d, want the page for the new session", status)
		}
	})

	t.Run("a new password ends the sessions the old one opened", func(t *testing.T) {
		sa := setupSecurityApp(t)
		cookie := sa.login()
		if err := users.SetPassword(sa.db, "owner@example.com", "a-new-password"); err != nil {
			t.Fatalf("SetPassword failed: %v", err)
		}

		status := open(sa, cookie)

		if status != http.StatusFound {
			t.Errorf("status = %d, want 302 to the login page", status)
		}
	})

	t.Run("a session of a deleted user opens nothing", func(t *testing.T) {
		sa := setupSecurityApp(t)
		cookie := sa.login()
		sa.db.Where("email = ?", "owner@example.com").Delete(&users.User{})
		users.CreateUser(sa.db, "other@example.com", "correct-horse")

		status := open(sa, cookie)

		if status != http.StatusFound {
			t.Errorf("status = %d, want 302 to the login page", status)
		}
	})

	t.Run("logout clears the cookie", func(t *testing.T) {
		sa := setupSecurityApp(t)
		cookie := sa.login()

		res := sa.do("POST", "/logout", url.Values{}, map[string]string{"Cookie": cookie, "Sec-Fetch-Site": "same-origin"})

		for _, c := range res.Cookies() {
			if c.Name == "lognorth_session" {
				if c.Value != "" || c.MaxAge >= 0 {
					t.Errorf("cookie = %+v, want it emptied and expired", c)
				}
				return
			}
		}
		t.Fatal("logout did not touch the session cookie")
	})
}

func TestBrowserWritesNeedTheSameOrigin(t *testing.T) {
	post := func(t *testing.T, path, secFetchSite string) int {
		t.Helper()
		sa := setupSecurityApp(t)
		headers := map[string]string{"Cookie": sa.login()}
		if secFetchSite != "" {
			headers["Sec-Fetch-Site"] = secFetchSite
		}
		return sa.do("POST", path, url.Values{}, headers).StatusCode
	}

	t.Run("a post with no Sec-Fetch-Site header is rejected", func(t *testing.T) {
		if status := post(t, "/settings/agent-key/regenerate", ""); status != http.StatusForbidden {
			t.Errorf("status = %d, want 403", status)
		}
	})

	t.Run("a post from another subdomain is rejected", func(t *testing.T) {
		if status := post(t, "/settings/agent-key/regenerate", "same-site"); status != http.StatusForbidden {
			t.Errorf("status = %d, want 403", status)
		}
	})

	t.Run("a cross-site login is rejected", func(t *testing.T) {
		if status := post(t, "/login", "cross-site"); status != http.StatusForbidden {
			t.Errorf("status = %d, want 403", status)
		}
	})

	t.Run("a cross-site logout is rejected", func(t *testing.T) {
		if status := post(t, "/logout", "cross-site"); status != http.StatusForbidden {
			t.Errorf("status = %d, want 403", status)
		}
	})

	t.Run("a GET does not run a write route", func(t *testing.T) {
		sa := setupSecurityApp(t)
		cookie := sa.login()
		agentKey, _ := settings.CreateAgentKey(sa.db)

		res := sa.do("GET", "/settings/agent-key/revoke", nil, map[string]string{"Cookie": cookie})

		if res.StatusCode == http.StatusOK {
			t.Errorf("status = 200, want the GET refused")
		}
		if err := settings.ValidateAgentKey(sa.db, agentKey); err != nil {
			t.Errorf("the agent key was revoked by a GET: %v", err)
		}
	})
}

func TestLoginRateLimit(t *testing.T) {
	wrong := url.Values{"email": {"owner@example.com"}, "password": {"wrong"}}
	right := url.Values{"email": {"owner@example.com"}, "password": {"correct-horse"}}
	sameOrigin := map[string]string{"Sec-Fetch-Site": "same-origin"}

	t.Run("the sixth attempt in a minute is refused, even with the right password", func(t *testing.T) {
		sa := setupProductionApp(t)
		users.CreateUser(sa.db, "owner@example.com", "correct-horse")
		for range 5 {
			sa.doFrom(directPeer, "POST", "/login", wrong, sameOrigin)
		}

		res := sa.doFrom(directPeer, "POST", "/login", right, sameOrigin)

		if res.StatusCode != http.StatusTooManyRequests {
			t.Errorf("status = %d, want 429", res.StatusCode)
		}
		if len(res.Cookies()) != 0 {
			t.Errorf("a refused attempt set cookies: %v", res.Cookies())
		}
	})

	t.Run("another client can still log in", func(t *testing.T) {
		sa := setupProductionApp(t)
		users.CreateUser(sa.db, "owner@example.com", "correct-horse")
		for range 6 {
			sa.doFrom(directPeer, "POST", "/login", wrong, sameOrigin)
		}

		res := sa.doFrom("198.51.100.99:41000", "POST", "/login", right, sameOrigin)

		if res.StatusCode != http.StatusFound || res.Header.Get("Location") != "/" {
			t.Errorf("status = %d to %q, want the login to pass", res.StatusCode, res.Header.Get("Location"))
		}
	})

	t.Run("a direct client cannot reset its budget with a forged X-Forwarded-For", func(t *testing.T) {
		sa := setupProductionApp(t)
		users.CreateUser(sa.db, "owner@example.com", "correct-horse")
		var last int

		for i := range 6 {
			headers := map[string]string{"Sec-Fetch-Site": "same-origin", "X-Forwarded-For": fmt.Sprintf("1.1.1.%d", i)}
			last = sa.doFrom(directPeer, "POST", "/login", wrong, headers).StatusCode
		}

		if last != http.StatusTooManyRequests {
			t.Errorf("sixth attempt = %d, want 429", last)
		}
	})

	t.Run("behind the proxy, rotating the first X-Forwarded-For entry does not reset the budget", func(t *testing.T) {
		sa := setupProductionApp(t)
		users.CreateUser(sa.db, "owner@example.com", "correct-horse")
		var last int

		for i := range 6 {
			headers := map[string]string{"Sec-Fetch-Site": "same-origin", "X-Forwarded-For": fmt.Sprintf("1.1.1.%d, 203.0.113.9", i)}
			last = sa.doFrom(proxyPeer, "POST", "/login", wrong, headers).StatusCode
		}

		if last != http.StatusTooManyRequests {
			t.Errorf("sixth attempt = %d, want 429", last)
		}
	})

	t.Run("behind the proxy, two clients have separate budgets", func(t *testing.T) {
		sa := setupProductionApp(t)
		users.CreateUser(sa.db, "owner@example.com", "correct-horse")
		for range 6 {
			sa.doFrom(proxyPeer, "POST", "/login", wrong, map[string]string{"Sec-Fetch-Site": "same-origin", "X-Forwarded-For": "203.0.113.9"})
		}

		res := sa.doFrom(proxyPeer, "POST", "/login", right, map[string]string{"Sec-Fetch-Site": "same-origin", "X-Forwarded-For": "203.0.113.10"})

		if res.StatusCode != http.StatusFound {
			t.Errorf("status = %d, want the second client to log in", res.StatusCode)
		}
	})
}

func TestAgentRateLimit(t *testing.T) {
	sa := setupProductionApp(t)
	agentKey, _ := settings.CreateAgentKey(sa.db)
	call := func() int {
		req := httptest.NewRequest("POST", "/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"ping"}`))
		req.RemoteAddr = directPeer
		req.Header.Set("Authorization", "Bearer "+agentKey)
		res := httptest.NewRecorder()
		sa.app.Server.ServeHTTP(res, req)
		return res.Code
	}
	for range 60 {
		if status := call(); status != http.StatusOK {
			t.Fatalf("a call inside the budget returned %d", status)
		}
	}

	status := call()

	if status != http.StatusTooManyRequests {
		t.Errorf("61st call = %d, want 429", status)
	}
}

func TestNoCrossOriginAccess(t *testing.T) {
	corsHeaders := []string{"Access-Control-Allow-Origin", "Access-Control-Allow-Credentials", "Access-Control-Allow-Methods", "Access-Control-Allow-Headers"}

	t.Run("a preflight from another origin gets no grant", func(t *testing.T) {
		sa := setupSecurityApp(t)

		for _, path := range []string{"/api/v1/events", "/mcp", "/login", "/settings", "/"} {
			res := sa.do("OPTIONS", path, nil, map[string]string{
				"Origin": "https://evil.example", "Access-Control-Request-Method": "POST", "Access-Control-Request-Headers": "authorization"})

			for _, h := range corsHeaders {
				if v := res.Header.Get(h); v != "" {
					t.Errorf("OPTIONS %s sets %s: %s", path, h, v)
				}
			}
		}
	})

	t.Run("a cross-origin read with the owner's session gets no grant", func(t *testing.T) {
		sa := setupSecurityApp(t)
		cookie := sa.login()

		res := sa.do("GET", "/api/v1/events", nil, map[string]string{"Cookie": cookie, "Origin": "https://evil.example"})

		for _, h := range corsHeaders {
			if v := res.Header.Get(h); v != "" {
				t.Errorf("response sets %s: %s", h, v)
			}
		}
	})
}

func TestSecurityHeaders(t *testing.T) {
	sa := setupSecurityApp(t)
	users.CreateUser(sa.db, "owner@example.com", "correct-horse")

	res := sa.do("GET", "/login", nil, nil)

	want := map[string]string{
		"X-Frame-Options":              "SAMEORIGIN",
		"X-Content-Type-Options":       "nosniff",
		"Referrer-Policy":              "same-origin",
		"Cross-Origin-Opener-Policy":   "same-origin",
		"Cross-Origin-Resource-Policy": "same-origin",
	}
	for header, value := range want {
		if got := res.Header.Get(header); got != value {
			t.Errorf("%s = %q, want %q", header, got, value)
		}
	}
}

func TestBodyLimit(t *testing.T) {
	sa := setupSecurityApp(t)
	_, key, _ := apikeys.Create(sa.db, "shop")
	body := `{"message":"` + strings.Repeat("a", 5*1024*1024) + `"}`
	req := httptest.NewRequest("POST", "/api/v1/events", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Content-Type", "application/json")

	res, err := sa.app.Server.Test(req)

	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if res.StatusCode != http.StatusRequestEntityTooLarge {
		t.Errorf("status = %d, want 413 for a 5 MB body", res.StatusCode)
	}
	var stored int64
	sa.db.Model(&events.IngestedEvent{}).Count(&stored)
	if stored != 0 {
		t.Errorf("stored = %d events, want none", stored)
	}
}

func TestDemo(t *testing.T) {
	setupDemo := func(t *testing.T) *securityApp {
		t.Helper()
		t.Setenv("LOGNORTH_DEMO", "true")
		sa := setupSecurityApp(t)
		users.CreateUser(sa.db, "demo@lognorth.com", "demo")
		return sa
	}

	t.Run("a write is turned away and changes nothing", func(t *testing.T) {
		sa := setupDemo(t)
		shop, _, _ := apikeys.Create(sa.db, "shop")

		res := sa.do("POST", fmt.Sprintf("/apps/api-keys/%d/delete", shop.ID), url.Values{},
			map[string]string{"Sec-Fetch-Site": "same-origin"})

		if res.StatusCode != http.StatusFound {
			t.Errorf("status = %d, want a redirect", res.StatusCode)
		}
		if _, err := apikeys.FindByID(sa.db, shop.ID); err != nil {
			t.Errorf("the demo deleted an app: %v", err)
		}
	})

	t.Run("a visitor gets 60 pages a minute", func(t *testing.T) {
		sa := setupDemo(t)
		for range 60 {
			sa.doFrom(directPeer, "GET", "/login", nil, nil)
		}

		res := sa.doFrom(directPeer, "GET", "/login", nil, nil)

		if res.StatusCode != http.StatusTooManyRequests {
			t.Errorf("61st page = %d, want 429", res.StatusCode)
		}
	})

	t.Run("the health check is never limited", func(t *testing.T) {
		sa := setupDemo(t)
		for range 70 {
			sa.doFrom(proxyPeer, "GET", "/_health", nil, nil)
		}

		res := sa.doFrom(proxyPeer, "GET", "/_health", nil, nil)

		if res.StatusCode != http.StatusOK {
			t.Errorf("status = %d, want 200: the proxy would mark the app down", res.StatusCode)
		}
	})
}
