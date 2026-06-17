package jobs

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/karloscodes/cartridge"
	"gorm.io/gorm"

	"lognorth/internal/agent"
	"lognorth/internal/apikeys"
	"lognorth/internal/config"
	"lognorth/internal/settings"
)

// maxRun caps a single fix. A job stuck running past this is assumed dead
// (its container died) and failed so it can't block the queue forever.
const maxRun = 20 * time.Minute

// fixCreds is everything the runner needs to fix one issue.
type fixCreds struct {
	Token, Key, Model, Repo, Issue, IssueHash string
}

// FixRunner drives the agent fix queue: one job at a time, oldest first.
type FixRunner struct {
	run func(fixCreds) (string, error)
}

// NewFixRunner runs fixes through the opencode container named by
// LOGNORTH_AGENT_IMAGE. With no image set (dev), it falls back to the stub so
// the queue is still exercisable.
func NewFixRunner() *FixRunner {
	if image := os.Getenv("LOGNORTH_AGENT_IMAGE"); image != "" {
		return &FixRunner{run: dockerRunner(image)}
	}
	return &FixRunner{run: stubRun}
}

// ProcessBatch runs at most one fix per tick. The single-flight guard, the
// failure handling, and "pick the next" are all just status queries.
func (r *FixRunner) ProcessBatch(ctx *cartridge.JobContext) error {
	db := ctx.DB

	// Don't touch running containers in demo mode.
	if os.Getenv("LOGNORTH_DEMO") == "true" {
		return nil
	}

	// Reap a job whose runner died mid-flight.
	db.Model(&agent.FixJob{}).
		Where("status = ? AND updated_at < ?", agent.StatusRunning, time.Now().Add(-maxRun)).
		Updates(map[string]any{"status": agent.StatusFailed, "detail": "timed out"})

	// One at a time.
	var running int64
	db.Model(&agent.FixJob{}).Where("status = ?", agent.StatusRunning).Count(&running)
	if running > 0 {
		return nil
	}

	// Claim the oldest queued job.
	var job agent.FixJob
	if err := db.Where("status = ?", agent.StatusQueued).Order("created_at asc").First(&job).Error; err != nil {
		return nil // nothing queued
	}
	db.Model(&agent.FixJob{}).Where("id = ?", job.ID).Update("status", agent.StatusRunning)

	creds, err := loadCreds(db, &job)
	if err != nil {
		failJob(db, job.ID, err.Error())
		return nil
	}

	prURL, err := r.run(creds)
	if err != nil {
		failJob(db, job.ID, err.Error())
		return nil
	}
	db.Model(&agent.FixJob{}).Where("id = ?", job.ID).Updates(map[string]any{
		"status": agent.StatusPROpen,
		"pr_url": prURL,
		"detail": "",
	})
	return nil
}

func loadCreds(db *gorm.DB, job *agent.FixJob) (fixCreds, error) {
	s, err := settings.Get(db)
	if err != nil {
		return fixCreds{}, fmt.Errorf("settings unavailable")
	}
	if !s.IsGitHubConfigured() {
		return fixCreds{}, fmt.Errorf("GitHub token not configured")
	}
	if !s.IsOpenRouterConfigured() {
		return fixCreds{}, fmt.Errorf("OpenRouter key not configured")
	}
	app, err := apikeys.FindByID(db, job.APIKeyID)
	if err != nil || !app.HasRepo() {
		return fixCreds{}, fmt.Errorf("app has no repo")
	}

	pk := config.GetConfig().PrivateKey
	token, err := agent.Decrypt(s.GitHubToken, pk)
	if err != nil {
		return fixCreds{}, fmt.Errorf("token unreadable")
	}
	key, err := agent.Decrypt(s.OpenRouterKey, pk)
	if err != nil {
		return fixCreds{}, fmt.Errorf("key unreadable")
	}

	return fixCreds{
		Token:     token,
		Key:       key,
		Model:     s.OpenRouterModel,
		Repo:      app.GitHubRepo,
		Issue:     job.Issue,
		IssueHash: job.IssueHash,
	}, nil
}

func failJob(db *gorm.DB, id uint, detail string) {
	db.Model(&agent.FixJob{}).Where("id = ?", id).
		Updates(map[string]any{"status": agent.StatusFailed, "detail": detail})
}

// dockerRunner runs opencode in a one-shot container. The container clones the
// repo, fixes the issue, pushes a branch, opens a PR, and prints the PR URL as
// its last stdout line. Secrets go in a 0600 env-file (never argv, which shows
// in `docker ps`); the issue text — which may be multi-line — goes via -e args
// (safe over argv, unlike env-file's line format).
func dockerRunner(image string) func(fixCreds) (string, error) {
	return func(c fixCreds) (string, error) {
		envFile, err := os.CreateTemp("", "lnfix-*.env")
		if err != nil {
			return "", fmt.Errorf("env file: %w", err)
		}
		defer os.Remove(envFile.Name())
		_ = envFile.Chmod(0600)
		fmt.Fprintf(envFile, "GH_TOKEN=%s\nOPENROUTER_API_KEY=%s\n", c.Token, c.Key)
		envFile.Close()

		ctx, cancel := context.WithTimeout(context.Background(), maxRun)
		defer cancel()

		cmd := exec.CommandContext(ctx, "docker", "run", "--rm",
			"--env-file", envFile.Name(),
			"-e", "OPENROUTER_MODEL="+c.Model,
			"-e", "REPO="+c.Repo,
			"-e", "ISSUE="+c.Issue,
			"-e", "ISSUE_HASH="+c.IssueHash,
			image,
		)
		out, err := cmd.CombinedOutput()
		if err != nil {
			if ctx.Err() == context.DeadlineExceeded {
				return "", fmt.Errorf("timed out")
			}
			return "", fmt.Errorf("agent failed: %s", tail(out))
		}

		pr := lastLine(out)
		if !strings.HasPrefix(pr, "http") {
			return "", fmt.Errorf("no PR url in output: %s", tail(out))
		}
		return pr, nil
	}
}

// stubRun stands in for the container in dev (no image configured). It pretends
// a PR was opened so the queue advances.
func stubRun(c fixCreds) (string, error) {
	return fmt.Sprintf("https://github.com/%s/pull/0", c.Repo), nil
}

func lastLine(b []byte) string {
	lines := strings.Split(strings.TrimSpace(string(b)), "\n")
	return strings.TrimSpace(lines[len(lines)-1])
}

func tail(b []byte) string {
	s := strings.TrimSpace(string(b))
	if len(s) > 500 {
		s = s[len(s)-500:]
	}
	return s
}
