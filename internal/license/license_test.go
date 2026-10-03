package license

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"lognorth/internal/polar"
)

// A self-hoster controls LOGNORTH_ENV. A free sandbox "purchase" must not
// unlock a release build, whatever the environment says.
func TestSandboxKeysDoNotUnlockARelease(t *testing.T) {
	sandbox := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"id":"lk_1","key":"SANDBOX-KEY","status":"granted","customer":{"email":"x@example.com"}}`))
	}))
	defer sandbox.Close()
	production := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer production.Close()
	prevSandbox, prevProduction := polar.SandboxBaseURL, polar.ProductionBaseURL
	polar.SandboxBaseURL, polar.ProductionBaseURL = sandbox.URL, production.URL
	defer func() { polar.SandboxBaseURL, polar.ProductionBaseURL = prevSandbox, prevProduction }()

	for _, env := range []string{"development", "", "production"} {
		t.Run("LOGNORTH_ENV="+env, func(t *testing.T) {
			t.Setenv("LOGNORTH_ENV", env)

			status, _, err := Validate(context.Background(), "SANDBOX-KEY")

			if err != nil {
				t.Fatalf("Validate: %v", err)
			}
			if status != "invalid" {
				t.Errorf("a sandbox-only key is %q, want invalid", status)
			}
		})
	}
}
