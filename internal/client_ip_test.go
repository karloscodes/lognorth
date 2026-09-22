package internal

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	cartridgemiddleware "github.com/karloscodes/cartridge/middleware"
)

// newProxiedApp mimics production: the app trusts X-Forwarded-For from the
// given proxies. app.Test connects from 0.0.0.0.
func newProxiedApp(trusted []string) *fiber.App {
	return fiber.New(fiber.Config{
		ProxyHeader:             fiber.HeaderXForwardedFor,
		TrustedProxies:          trusted,
		EnableTrustedProxyCheck: true,
	})
}

func keyFor(t *testing.T, trusted []string, forwarded string) string {
	t.Helper()
	app := newProxiedApp(trusted)
	app.Get("/", func(c *fiber.Ctx) error { return c.SendString(clientIP(c)) })

	req := httptest.NewRequest("GET", "/", nil)
	if forwarded != "" {
		req.Header.Set("X-Forwarded-For", forwarded)
	}
	res, err := app.Test(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	body, _ := io.ReadAll(res.Body)
	return string(body)
}

func TestClientIP(t *testing.T) {
	t.Run("behind the proxy, the last entry is the client", func(t *testing.T) {
		if key := keyFor(t, []string{"0.0.0.0"}, "6.6.6.6, 203.0.113.9"); key != "203.0.113.9" {
			t.Errorf("key = %q, want the entry the proxy appended", key)
		}
	})

	t.Run("a request that skipped the proxy uses the connection address", func(t *testing.T) {
		if key := keyFor(t, []string{"10.0.0.0/8"}, "6.6.6.6"); key != "0.0.0.0" {
			t.Errorf("key = %q, want the connection address, not the header", key)
		}
	})

	t.Run("no header uses the connection address", func(t *testing.T) {
		if key := keyFor(t, []string{"0.0.0.0"}, ""); key != "0.0.0.0" {
			t.Errorf("key = %q, want the connection address", key)
		}
	})
}

func TestRateLimitCannotBeDodgedWithAFakeForwardedFor(t *testing.T) {
	app := newProxiedApp([]string{"0.0.0.0"})
	app.Use(cartridgemiddleware.RateLimiter(
		cartridgemiddleware.WithMax(2),
		cartridgemiddleware.WithDuration(time.Minute),
		cartridgemiddleware.WithKeyGenerator(clientIP),
	))
	app.Post("/login", func(c *fiber.Ctx) error { return c.SendString("ok") })

	var last int
	for _, fake := range []string{"1.1.1.1", "2.2.2.2", "3.3.3.3"} {
		req := httptest.NewRequest("POST", "/login", nil)
		req.Header.Set("X-Forwarded-For", fake+", 203.0.113.9") // the client rotates the first entry
		res, err := app.Test(req)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		last = res.StatusCode
	}

	if last != http.StatusTooManyRequests {
		t.Errorf("third request = %d, want 429: a fake first entry must not reset the budget", last)
	}
}
