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

	t.Run("its domain shows the page even when BASE_URL names that domain", func(t *testing.T) {
		// Chasen sets BASE_URL to the domain added last, so it can be the page's.
		t.Setenv("BASE_URL", "https://status.shop.example")
		sa, _ := setup(t)

		res := sa.do("GET", "http://status.shop.example/", nil, nil)

		html := body(res)
		if res.StatusCode != http.StatusOK || !strings.Contains(html, `"component":"Status"`) {
			t.Errorf("expected the Shop status page, got %d to %q: %.200s", res.StatusCode, res.Header.Get("Location"), html)
		}
	})

	t.Run("a page can't take the domain the dashboard is open on", func(t *testing.T) {
		sa, cookie := setup(t)

		res := sa.do("POST", "http://logs.shop.example/settings/status",
			url.Values{"name": {"Blog"}, "domain": {"logs.shop.example"}},
			map[string]string{"Cookie": cookie, "Sec-Fetch-Site": "same-origin"})

		if _, ok := status.PageBySlug(sa.db, "blog"); ok || res.StatusCode != http.StatusFound {
			t.Errorf("expected the page refused, got %d and a page made: %v", res.StatusCode, ok)
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

	t.Run("the settings give the domain LogNorth answers on as the DNS target", func(t *testing.T) {
		sa, cookie := setup(t)

		res := sa.do("GET", "http://logs.shop.example:3000/settings/status", nil, map[string]string{"Cookie": cookie})

		html := body(res)
		if res.StatusCode != http.StatusOK || !strings.Contains(html, `"host":"logs.shop.example"`) || !strings.Contains(html, `"domain":"status.shop.example"`) {
			t.Errorf("expected the page's domain and logs.shop.example as the CNAME target, got %d: %.300s", res.StatusCode, html)
		}
	})
}
