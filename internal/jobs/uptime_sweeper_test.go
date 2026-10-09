package jobs

import (
	"strings"
	"testing"

	"lognorth/internal/apikeys"
	"lognorth/internal/events"
)

func TestDownBody(t *testing.T) {
	key := &apikeys.APIKey{ID: 4, Name: "shop", PingURL: "https://shop.example.com/up"}

	t.Run("names the issues that surged and points the agent at the first", func(t *testing.T) {
		surges := []events.Surge{
			{Hash: "abc", ErrorClass: "Redis::CannotConnectError", Error: "connection refused", Count: 12},
			{Hash: "def", Error: "timeout", Count: 4},
		}

		body := downBody(key, "https://logs.example.com", surges)

		for _, want := range []string{
			"shop isn't responding.",
			"12x Redis::CannotConnectError: connection refused",
			"https://logs.example.com/issues/abc",
			"4x timeout",
			"/lognorth:investigate abc",
		} {
			if !strings.Contains(body, want) {
				t.Errorf("expected %q in:\n%s", want, body)
			}
		}
	})

	t.Run("says nothing about errors when none surged", func(t *testing.T) {
		body := downBody(key, "https://logs.example.com", nil)

		if strings.Contains(body, "surged") || strings.Contains(body, "investigate") {
			t.Errorf("expected no error section, got:\n%s", body)
		}
		if !strings.Contains(body, "https://logs.example.com/apps/4/endpoints") {
			t.Errorf("expected a link to the app's endpoints, got:\n%s", body)
		}
	})
}
