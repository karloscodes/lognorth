package internal

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/karloscodes/cartridge"
	cartridgemiddleware "github.com/karloscodes/cartridge/middleware"
	"github.com/karloscodes/cartridge/testsupport"
)

// The proxy sits on the Docker network; a client that skips it does not.
const (
	proxyPeer  = "10.0.0.5:41000"
	directPeer = "198.51.100.7:41000"
)

func newClientIPServer(t *testing.T, mount func(*cartridge.Server)) *cartridge.Server {
	t.Helper()

	cfg := cartridge.DefaultServerConfig()
	cfg.Config = testsupport.NewTestConfig()
	cfg.Logger = testsupport.NewTestLogger()
	cfg.DBManager = testsupport.NewTestDBManager(testsupport.SetupTestDB(t))
	server, err := cartridge.NewServer(cfg)
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}
	mount(server)

	return server
}

func keyFor(t *testing.T, peer, forwarded string) string {
	t.Helper()
	server := newClientIPServer(t, func(s *cartridge.Server) {
		s.Get("/", func(c *cartridge.Context) error { return c.SendString(clientIP(c)) })
	})

	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = peer
	if forwarded != "" {
		req.Header.Set("X-Forwarded-For", forwarded)
	}
	res := httptest.NewRecorder()
	server.ServeHTTP(res, req)

	return res.Body.String()
}

func TestClientIP(t *testing.T) {
	t.Run("behind the proxy, the last entry is the client", func(t *testing.T) {
		if key := keyFor(t, proxyPeer, "6.6.6.6, 203.0.113.9"); key != "203.0.113.9" {
			t.Errorf("key = %q, want the entry the proxy appended", key)
		}
	})

	t.Run("a request that skipped the proxy uses the connection address", func(t *testing.T) {
		if key := keyFor(t, directPeer, "6.6.6.6"); key != "198.51.100.7" {
			t.Errorf("key = %q, want the connection address, not the header", key)
		}
	})

	t.Run("no header uses the connection address", func(t *testing.T) {
		if key := keyFor(t, proxyPeer, ""); key != "10.0.0.5" {
			t.Errorf("key = %q, want the connection address", key)
		}
	})

	t.Run("an IPv6 loopback proxy counts as the proxy", func(t *testing.T) {
		if key := keyFor(t, "[::1]:41000", "203.0.113.9"); key != "203.0.113.9" {
			t.Errorf("key = %q, want the entry the proxy appended", key)
		}
	})
}

func TestRateLimitCannotBeDodgedWithAFakeForwardedFor(t *testing.T) {
	limited := &cartridge.RouteConfig{
		EnableSecFetchSite: cartridge.Bool(false),
		CustomMiddleware: []cartridge.HandlerFunc{cartridgemiddleware.RateLimiter(
			cartridgemiddleware.WithMax(2),
			cartridgemiddleware.WithDuration(time.Minute),
			cartridgemiddleware.WithKeyGenerator(clientIP),
		)},
	}
	post := func(t *testing.T, peer string, forwarded []string) int {
		t.Helper()
		server := newClientIPServer(t, func(s *cartridge.Server) {
			s.Post("/login", func(c *cartridge.Context) error { return c.SendString("ok") }, limited)
		})

		var last int
		for _, header := range forwarded {
			req := httptest.NewRequest("POST", "/login", nil)
			req.RemoteAddr = peer
			req.Header.Set("X-Forwarded-For", header)
			res := httptest.NewRecorder()
			server.ServeHTTP(res, req)
			last = res.Code
		}
		return last
	}

	t.Run("behind the proxy, rotating the first entry does not reset the budget", func(t *testing.T) {
		last := post(t, proxyPeer, []string{"1.1.1.1, 203.0.113.9", "2.2.2.2, 203.0.113.9", "3.3.3.3, 203.0.113.9"})

		if last != http.StatusTooManyRequests {
			t.Errorf("third request = %d, want 429", last)
		}
	})

	t.Run("without the proxy, a forged header does not reset the budget", func(t *testing.T) {
		last := post(t, directPeer, []string{"1.1.1.1", "2.2.2.2", "3.3.3.3"})

		if last != http.StatusTooManyRequests {
			t.Errorf("third request = %d, want 429: the header of a direct client must not count", last)
		}
	})
}
