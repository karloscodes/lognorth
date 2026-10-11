package tests

import (
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"lognorth/internal/apikeys"
	"lognorth/internal/status"
)

// A status page answers on its own domain and at /status/<slug>, through the
// real router: the route wiring is what keeps the dashboard off a status
// domain.
func TestStatusPages(t *testing.T) {
	setup := func(t *testing.T) (*securityApp, string) {
		sa := setupSecurityApp(t)
		cookie := sa.login()
		page, err := status.CreatePage(sa.db, "Shop", "status.shop.example", nil)
		if err != nil {
			t.Fatalf("create page: %v", err)
		}
		key, _, err := apikeys.Create(sa.db, "shop-web")
		if err != nil {
			t.Fatalf("create app: %v", err)
		}
		if err := status.SetAppPage(sa.db, key.ID, page.ID); err != nil {
			t.Fatalf("put the app on the page: %v", err)
		}
		return sa, cookie
	}
	body := func(res *http.Response) string {
		b, _ := io.ReadAll(res.Body)
		return string(b)
	}

	t.Run("its domain shows the page at /", func(t *testing.T) {
		sa, _ := setup(t)

		res := sa.do("GET", "http://status.shop.example/", nil, nil)

		html := body(res)
		if res.StatusCode != http.StatusOK || !strings.Contains(html, `"component":"Status"`) || !strings.Contains(html, "shop-web") {
			t.Errorf("expected the Shop status page, got %d: %.300s", res.StatusCode, html)
		}
	})

	t.Run("its domain never shows the dashboard or the login", func(t *testing.T) {
		sa, cookie := setup(t)

		for _, path := range []string{"/apps", "/login", "/settings/status", "/status/shop"} {
			res := sa.do("GET", "http://status.shop.example"+path, nil, map[string]string{"Cookie": cookie})

			if res.StatusCode != http.StatusFound || res.Header.Get("Location") != "/" {
				t.Errorf("GET %s on the status domain: got %d to %q, want a redirect to /", path, res.StatusCode, res.Header.Get("Location"))
			}
		}
		post := sa.do("POST", "http://status.shop.example/login",
			url.Values{"email": {"owner@example.com"}, "password": {"correct-horse"}},
			map[string]string{"Sec-Fetch-Site": "same-origin"})
		if post.StatusCode != http.StatusFound || post.Header.Get("Location") != "/" || len(post.Cookies()) > 0 {
			t.Errorf("POST /login on the status domain: got %d to %q, want a redirect to / and no session", post.StatusCode, post.Header.Get("Location"))
		}
	})

	t.Run("LogNorth's own domain keeps the dashboard, and has the page at /status/<slug>", func(t *testing.T) {
		sa, cookie := setup(t)

		home := sa.do("GET", "/", nil, map[string]string{"Cookie": cookie})
		page := sa.do("GET", "/status/shop", nil, nil)
		missing := sa.do("GET", "/status/blog", nil, nil)

		if html := body(home); home.StatusCode != http.StatusOK || strings.Contains(html, `"component":"Status"`) {
			t.Errorf("expected the dashboard at /, got %d: %.200s", home.StatusCode, html)
		}
		if html := body(page); page.StatusCode != http.StatusOK || !strings.Contains(html, "shop-web") {
			t.Errorf("expected the Shop page at /status/shop, got %d", page.StatusCode)
		}
		if missing.StatusCode != http.StatusNotFound {
			t.Errorf("expected 404 for a page that does not exist, got %d", missing.StatusCode)
		}
	})
}
