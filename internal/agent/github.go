package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// githubAPIBase is a var so tests can point it at httptest.
var githubAPIBase = "https://api.github.com"

// TokenInfo is the result of validating a GitHub token. It carries enough to
// guarantee scope: the token type and the repos it can actually reach.
type TokenInfo struct {
	Login       string   // authenticated account
	FineGrained bool     // true for github_pat_ (the only repo-scopable kind)
	Repos       []string // full_names the token can reach; a fine-grained PAT only sees its selected repos
}

// ScopedTo reports whether the token is safe to use for exactly one repo:
// fine-grained AND limited to that single repo. This is the guarantee — the
// save handler refuses a token that isn't ScopedTo the configured repo.
func (i TokenInfo) ScopedTo(repo string) bool {
	return i.FineGrained && len(i.Repos) == 1 && strings.EqualFold(i.Repos[0], repo)
}

// IsFineGrained reports whether a token is a fine-grained PAT. Classic tokens
// (ghp_) are account-wide and cannot be limited to specific repos, so we reject
// them outright.
func IsFineGrained(token string) bool {
	return strings.HasPrefix(token, "github_pat_")
}

// ValidateToken confirms the token works and reports what it can reach, so the
// caller can refuse anything broader than the one configured repo.
func ValidateToken(ctx context.Context, token string) (TokenInfo, error) {
	info := TokenInfo{FineGrained: IsFineGrained(token)}

	login, err := getField(ctx, token, "/user", "login")
	if err != nil {
		return info, err
	}
	info.Login = login

	repos, err := accessibleRepos(ctx, token)
	if err != nil {
		return info, err
	}
	info.Repos = repos
	return info, nil
}

// CheckRepoAccess reports whether the token can reach owner/repo. Used to pin
// an app to a repo the global token is actually scoped to.
func CheckRepoAccess(ctx context.Context, token, repo string) (bool, error) {
	_, err := doGitHub(ctx, token, "/repos/"+repo)
	if err != nil {
		return false, err
	}
	return true, nil
}

func accessibleRepos(ctx context.Context, token string) ([]string, error) {
	body, err := doGitHub(ctx, token, "/user/repos?per_page=100")
	if err != nil {
		return nil, err
	}
	var repos []struct {
		FullName string `json:"full_name"`
	}
	if err := json.Unmarshal(body, &repos); err != nil {
		return nil, err
	}
	out := make([]string, 0, len(repos))
	for _, r := range repos {
		out = append(out, r.FullName)
	}
	return out, nil
}

func getField(ctx context.Context, token, path, field string) (string, error) {
	body, err := doGitHub(ctx, token, path)
	if err != nil {
		return "", err
	}
	m := map[string]any{}
	if err := json.Unmarshal(body, &m); err != nil {
		return "", err
	}
	s, _ := m[field].(string)
	return s, nil
}

func doGitHub(ctx context.Context, token, path string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, githubAPIBase+path, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")

	resp, err := (&http.Client{Timeout: 10 * time.Second}).Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("github %s: status %d", path, resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}
