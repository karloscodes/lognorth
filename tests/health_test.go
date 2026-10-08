package tests

import (
	"io"
	"net/http"
	"testing"
)

// Chasen asks /up, matcha and Docker ask /_health. Both answer with no login,
// before the owner has set up the first account.
func TestHealthPaths(t *testing.T) {
	sa := setupProductionApp(t)

	for _, path := range []string{"/up", "/_health"} {
		t.Run(path, func(t *testing.T) {
			res := sa.do("GET", path, nil, nil)

			body, _ := io.ReadAll(res.Body)
			if res.StatusCode != http.StatusOK || string(body) != "ok" {
				t.Errorf("GET %s = %d %q, want 200 ok", path, res.StatusCode, body)
			}
		})
	}
}
