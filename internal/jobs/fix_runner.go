package jobs

import (
	"fmt"
	"os"
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

// NewFixRunner returns a runner backed by the stub container command. Swap
// run for the real opencode-in-Docker command when the image exists.
func NewFixRunner() *FixRunner {
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

// stubRun stands in for the real opencode-in-Docker runner so the queue is
// exercisable before the image exists. The real version will:
//
//	docker run --rm --env-file <creds> <image> <repo> <issue>
//
// and read the PR URL from the container's last stdout line.
func stubRun(c fixCreds) (string, error) {
	return fmt.Sprintf("https://github.com/%s/pull/0", c.Repo), nil
}
