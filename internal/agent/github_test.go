package agent

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestIsFineGrained(t *testing.T) {
	if !IsFineGrained("github_pat_11ABC_xyz") {
		t.Error("fine-grained token not recognized")
	}
	if IsFineGrained("ghp_classicToken") {
		t.Error("classic token wrongly accepted as fine-grained")
	}
}

func TestValidateTokenAndScope(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/user":
			w.Write([]byte(`{"login":"karloscodes"}`))
		case "/user/repos":
			w.Write([]byte(`[{"full_name":"karloscodes/lognorth"}]`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()
	old := githubAPIBase
	githubAPIBase = srv.URL
	defer func() { githubAPIBase = old }()

	info, err := ValidateToken(context.Background(), "github_pat_test")

	if err != nil {
		t.Fatalf("validate: %v", err)
	}
	if info.Login != "karloscodes" {
		t.Errorf("login = %q, want karloscodes", info.Login)
	}
	if !info.ScopedTo("karloscodes/lognorth") {
		t.Errorf("expected ScopedTo the single repo; repos=%v fineGrained=%v", info.Repos, info.FineGrained)
	}
	if info.ScopedTo("karloscodes/other-repo") {
		t.Error("ScopedTo wrongly matched a different repo")
	}
}

func TestValidateTokenRejectsBadStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()
	old := githubAPIBase
	githubAPIBase = srv.URL
	defer func() { githubAPIBase = old }()

	if _, err := ValidateToken(context.Background(), "github_pat_bad"); err == nil {
		t.Fatal("expected error for 401, got nil")
	}
}

func TestPRState(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/karloscodes/lognorth/pulls/7":
			w.Write([]byte(`{"merged":true,"state":"closed"}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()
	old := githubAPIBase
	githubAPIBase = srv.URL
	defer func() { githubAPIBase = old }()

	merged, state, err := PRState(context.Background(), "github_pat_test", "karloscodes/lognorth", 7)

	if err != nil {
		t.Fatalf("PRState: %v", err)
	}
	if !merged || state != "closed" {
		t.Errorf("merged=%v state=%q, want merged=true state=closed", merged, state)
	}
}

// An over-scoped token (more than the one repo) must not pass ScopedTo.
func TestOverScopedTokenRejected(t *testing.T) {
	info := TokenInfo{
		Login:       "x",
		FineGrained: true,
		Repos:       []string{"karloscodes/lognorth", "karloscodes/secret-repo"},
	}
	if info.ScopedTo("karloscodes/lognorth") {
		t.Error("over-scoped token must fail ScopedTo")
	}
}
