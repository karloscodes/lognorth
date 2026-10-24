package tests

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"lognorth/internal/alerts"
	"lognorth/internal/settings"
)

// webhookInbox records what LogNorth POSTs to a webhook.
type webhookInbox struct {
	mu     sync.Mutex
	auth   []string
	bodies []map[string]string
}

func webhookReceiver(t *testing.T, status int) (string, *webhookInbox) {
	t.Helper()
	inbox := &webhookInbox{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]string
		json.NewDecoder(r.Body).Decode(&body)
		inbox.mu.Lock()
		inbox.auth = append(inbox.auth, r.Header.Get("Authorization"))
		inbox.bodies = append(inbox.bodies, body)
		inbox.mu.Unlock()
		w.WriteHeader(status)
	}))
	t.Cleanup(srv.Close)
	return srv.URL, inbox
}

func TestWebhookAlerts(t *testing.T) {
	t.Run("posts the alert as JSON with the bearer token", func(t *testing.T) {
		url, inbox := webhookReceiver(t, http.StatusOK)
		s := &settings.Settings{NotifyChannel: "webhook", WebhookURL: url, WebhookToken: "crsr_secret"}

		err := alerts.Send(s, "[shop] Spike on /checkout", "8% errors, normally 1%.\n\n/lognorth:investigate /checkout")

		if err != nil {
			t.Fatalf("send: %v", err)
		}
		if len(inbox.bodies) != 1 {
			t.Fatalf("webhook got %d requests, want 1", len(inbox.bodies))
		}
		body := inbox.bodies[0]
		if inbox.auth[0] != "Bearer crsr_secret" {
			t.Errorf("Authorization = %q, want the saved token", inbox.auth[0])
		}
		if body["source"] != "lognorth" || body["title"] != "[shop] Spike on /checkout" || !strings.Contains(body["message"], "/lognorth:investigate /checkout") {
			t.Errorf("body = %v, want source, title, and the message with the investigate line", body)
		}
		if !strings.HasPrefix(body["text"], "[shop] Spike on /checkout\n\n") {
			t.Errorf("text = %q, want the title then the message, for Slack-style receivers", body["text"])
		}
	})

	t.Run("sends no Authorization header without a token", func(t *testing.T) {
		url, inbox := webhookReceiver(t, http.StatusNoContent)
		s := &settings.Settings{NotifyChannel: "webhook", WebhookURL: url}

		err := alerts.Send(s, "title", "message")

		if err != nil || inbox.auth[0] != "" {
			t.Errorf("err = %v, Authorization = %q; want success and no header", err, inbox.auth[0])
		}
	})

	t.Run("reports a receiver that refuses the alert", func(t *testing.T) {
		url, _ := webhookReceiver(t, http.StatusUnauthorized)
		s := &settings.Settings{NotifyChannel: "webhook", WebhookURL: url, WebhookToken: "wrong"}

		err := alerts.Send(s, "title", "message")

		if err == nil || !strings.Contains(err.Error(), "401") {
			t.Errorf("err = %v, want a failure that names the 401", err)
		}
	})

	t.Run("counts as configured only with a URL", func(t *testing.T) {
		s := &settings.Settings{NotifyChannel: "webhook", WebhookToken: "crsr_secret"}

		if alerts.CanNotify(s) {
			t.Error("CanNotify = true without a URL")
		}
		s.WebhookURL = "https://api2.cursor.sh/automations/webhook/abc"
		if !alerts.CanNotify(s) {
			t.Error("CanNotify = false with a URL")
		}
	})
}
