package events

import (
	"slices"
	"strings"
	"testing"
)

// The fake secrets are joined at run time, so the source holds no key for
// gitleaks to report.
var (
	stripeKey  = "sk_" + "live_" + strings.Repeat("x", 24)
	privateKey = "-----BEGIN RSA PRIVATE " + "KEY-----\nMIIx\n-----END RSA PRIVATE " + "KEY-----"
)

func TestRedactSecrets(t *testing.T) {
	t.Run("replaces a key in a context value and names its kind", func(t *testing.T) {
		ie := &IngestedEvent{Message: "charge", Context: `{"path":"/checkout","response":"key ` + stripeKey + ` ok"}`}

		kinds := RedactSecrets(ie)

		if !slices.Equal(kinds, []string{"Stripe secret key"}) {
			t.Errorf("kinds = %v, want [Stripe secret key]", kinds)
		}
		if strings.Contains(ie.Context, stripeKey) {
			t.Errorf("context still holds the key: %s", ie.Context)
		}
		if !strings.Contains(ie.Context, `"key [redacted: Stripe secret key] ok"`) || !strings.Contains(ie.Context, `"path":"/checkout"`) {
			t.Errorf("context = %s, want the key replaced and the rest kept", ie.Context)
		}
	})

	t.Run("replaces a key in the message", func(t *testing.T) {
		ie := &IngestedEvent{Message: "calling Stripe with " + stripeKey, Context: `{}`}

		kinds := RedactSecrets(ie)

		if ie.Message != "calling Stripe with [redacted: Stripe secret key]" {
			t.Errorf("message = %q", ie.Message)
		}
		if !slices.Equal(kinds, []string{"Stripe secret key"}) {
			t.Errorf("kinds = %v, want [Stripe secret key]", kinds)
		}
	})

	t.Run("redacts values under secret names without naming a kind", func(t *testing.T) {
		ie := &IngestedEvent{Message: "POST /login", Context: `{
			"path": "/login",
			"params": {"email": "ana@example.com", "password": "hunter2"},
			"headers": {"Authorization": "Bearer abc123", "Accept": "text/html", "X-CSRF-Token": "t0k3n"},
			"cookies": {"session": "s3ss10n"}
		}`}

		kinds := RedactSecrets(ie)

		if len(kinds) != 0 {
			t.Errorf("kinds = %v, want none", kinds)
		}
		for _, secret := range []string{"hunter2", "abc123", "t0k3n", "s3ss10n"} {
			if strings.Contains(ie.Context, secret) {
				t.Errorf("context still holds %q: %s", secret, ie.Context)
			}
		}
		for _, kept := range []string{"ana@example.com", "text/html", "/login"} {
			if !strings.Contains(ie.Context, kept) {
				t.Errorf("context lost %q: %s", kept, ie.Context)
			}
		}
	})

	t.Run("names the kind of a key under a secret name", func(t *testing.T) {
		ie := &IngestedEvent{Message: "boot", Context: `{"stripe_api_key":"` + stripeKey + `"}`}

		kinds := RedactSecrets(ie)

		if !slices.Equal(kinds, []string{"Stripe secret key"}) {
			t.Errorf("kinds = %v, want [Stripe secret key]", kinds)
		}
		if strings.Contains(ie.Context, stripeKey) {
			t.Errorf("context still holds the key: %s", ie.Context)
		}
	})

	t.Run("leaves an event without secrets as it came", func(t *testing.T) {
		context := `{"method":"GET","path":"/api/usage","status":200,"token_count":1234,"password_set":true,"user":"ana@example.com"}`
		ie := &IngestedEvent{Message: "GET /api/usage → 200", Context: context}

		kinds := RedactSecrets(ie)

		if len(kinds) != 0 || ie.Context != context || ie.Message != "GET /api/usage → 200" {
			t.Errorf("got kinds %v, message %q, context %s; want the event unchanged", kinds, ie.Message, ie.Context)
		}
	})

	t.Run("keeps large numbers exact when it rewrites the context", func(t *testing.T) {
		ie := &IngestedEvent{Message: "signup", Context: `{"account_id":12345678901234567890,"password":"hunter2"}`}

		RedactSecrets(ie)

		if !strings.Contains(ie.Context, `"account_id":12345678901234567890`) {
			t.Errorf("context = %s, want the account id unchanged", ie.Context)
		}
	})

	t.Run("keeps a Stripe test key", func(t *testing.T) {
		testKey := "sk_" + "test_" + strings.Repeat("x", 24)
		ie := &IngestedEvent{Message: testKey, Context: `{}`}

		kinds := RedactSecrets(ie)

		if len(kinds) != 0 || ie.Message != testKey {
			t.Errorf("got kinds %v and message %q, want the test key kept", kinds, ie.Message)
		}
	})

	t.Run("recognizes each format", func(t *testing.T) {
		formats := []struct {
			kind   string
			secret string
		}{
			{"AWS access key", "AKIA" + strings.Repeat("X", 16)},
			{"GitHub token", "ghp_" + strings.Repeat("x", 36)},
			{"GitHub token", "github_" + "pat_" + strings.Repeat("x", 40)},
			{"Slack token", "xoxb-" + strings.Repeat("1", 12)},
			{"Anthropic API key", "sk-" + "ant-" + strings.Repeat("x", 40)},
			{"OpenAI API key", "sk-" + "proj-" + strings.Repeat("x", 40)},
			{"private key", privateKey},
			{"private key", strings.SplitN(privateKey, "-----END", 2)[0]}, // cut short by the SDK
		}
		for _, f := range formats {
			ie := &IngestedEvent{Message: "before " + f.secret, Context: `{}`}

			kinds := RedactSecrets(ie)

			if !slices.Equal(kinds, []string{f.kind}) || ie.Message != "before [redacted: "+f.kind+"]" {
				t.Errorf("%s: got kinds %v and message %q", f.kind, kinds, ie.Message)
			}
		}
	})
}
