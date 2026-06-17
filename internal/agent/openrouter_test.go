package agent

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestValidateOpenRouterKey(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer sk-or-test" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Write([]byte(`{"data":{"label":"lognorth agent"}}`))
	}))
	defer srv.Close()
	old := openRouterAPIBase
	openRouterAPIBase = srv.URL
	defer func() { openRouterAPIBase = old }()

	label, err := ValidateOpenRouterKey(context.Background(), "sk-or-test")

	if err != nil {
		t.Fatalf("validate: %v", err)
	}
	if label != "lognorth agent" {
		t.Errorf("label = %q, want %q", label, "lognorth agent")
	}
}

func TestValidateOpenRouterKeyRejectsBad(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()
	old := openRouterAPIBase
	openRouterAPIBase = srv.URL
	defer func() { openRouterAPIBase = old }()

	if _, err := ValidateOpenRouterKey(context.Background(), "bad"); err == nil {
		t.Fatal("expected error for 401, got nil")
	}
}
