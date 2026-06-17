package jobs

import (
	"context"
	"os"
	"strconv"
	"strings"

	"github.com/karloscodes/cartridge"

	"lognorth/internal/agent"
	"lognorth/internal/apikeys"
	"lognorth/internal/config"
	"lognorth/internal/settings"
)

// PRPoller closes the fix loop: it checks each open PR and moves its job to
// merged, or to failed if the PR was closed without merging.
type PRPoller struct{}

func NewPRPoller() *PRPoller { return &PRPoller{} }

func (p *PRPoller) ProcessBatch(ctx *cartridge.JobContext) error {
	db := ctx.DB
	if os.Getenv("LOGNORTH_DEMO") == "true" {
		return nil
	}

	var jobs []agent.FixJob
	if err := db.Where("status = ?", agent.StatusPROpen).Find(&jobs).Error; err != nil {
		return err
	}
	if len(jobs) == 0 {
		return nil
	}

	s, err := settings.Get(db)
	if err != nil || !s.IsGitHubConfigured() {
		return nil
	}
	token, err := agent.Decrypt(s.GitHubToken, config.GetConfig().PrivateKey)
	if err != nil {
		return nil
	}

	for _, job := range jobs {
		number := prNumber(job.PRURL)
		if number == 0 {
			continue
		}
		app, err := apikeys.FindByID(db, job.APIKeyID)
		if err != nil || app.GitHubRepo == "" {
			continue
		}
		merged, state, err := agent.PRState(context.Background(), token, app.GitHubRepo, number)
		if err != nil {
			continue // transient — try again next tick
		}
		switch {
		case merged:
			db.Model(&agent.FixJob{}).Where("id = ?", job.ID).Update("status", agent.StatusMerged)
		case state == "closed":
			db.Model(&agent.FixJob{}).Where("id = ?", job.ID).
				Updates(map[string]any{"status": agent.StatusFailed, "detail": "PR closed without merging"})
		}
	}
	return nil
}

// prNumber pulls the trailing number off a GitHub PR URL.
func prNumber(url string) int {
	parts := strings.Split(strings.TrimRight(url, "/"), "/")
	n, _ := strconv.Atoi(parts[len(parts)-1])
	return n
}
