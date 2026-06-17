package http

import (
	"context"
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/karloscodes/cartridge"

	"lognorth/internal/agent"
	"lognorth/internal/apikeys"
	"lognorth/internal/config"
	"lognorth/internal/settings"
)

// FixesPageAction handles GET /fixes — the global list of agent fix jobs
// across all apps: in progress, queued, done, and failed.
func FixesPageAction(ctx *cartridge.Context) error {
	db := ctx.DB()

	jobs, _ := agent.ListFixJobs(db, 0)

	connected := false
	if keys, err := apikeys.List(db); err == nil {
		for _, k := range keys {
			if k.Active && k.HasRepo() {
				connected = true
				break
			}
		}
	}

	return RenderPage(ctx.Ctx, "Fixes", Props{
		"jobs":      jobs,
		"connected": connected,
	})
}

// AppFixesPageAction handles GET /apps/:id/fixes — one app's repo and its fix
// queue. The token is global (Settings); here you just pick the repo.
func AppFixesPageAction(ctx *cartridge.Context) error {
	db := ctx.DB()

	app, apps, err := getAppFromURL(ctx)
	if err != nil {
		return ctx.Ctx.Redirect("/")
	}

	jobs, _ := agent.ListFixJobs(db, app.ID)

	tokenReady := false
	if s, err := settings.Get(db); err == nil {
		tokenReady = s.IsGitHubConfigured()
	}

	return RenderPage(ctx.Ctx, "AppFixes", Props{
		"appId":        app.ID,
		"appName":      app.Name,
		"applications": apps,
		"tokenReady":   tokenReady,
		"githubRepo":   app.GitHubRepo,
		"connected":    app.HasRepo(),
		"jobs":         jobs,
	})
}

// ConnectAppRepoInput is the body for POST /apps/api-keys/:id/github.
type ConnectAppRepoInput struct {
	Repo string `json:"github_repo"`
}

// ConnectAppRepoAction handles POST /apps/api-keys/:id/github. It pins an app
// to a repo, but only after confirming the global token can actually reach it.
func ConnectAppRepoAction(ctx *cartridge.Context) error {
	db := ctx.DB()

	id, err := strconv.ParseUint(ctx.Ctx.Params("id"), 10, 64)
	if err != nil {
		return ctx.Ctx.Status(400).JSON(fiber.Map{"error": "invalid app"})
	}

	var input ConnectAppRepoInput
	if err := ctx.Ctx.BodyParser(&input); err != nil {
		return ctx.Ctx.Status(400).JSON(fiber.Map{"error": "invalid input"})
	}
	repo := strings.TrimSpace(input.Repo)
	if parts := strings.Split(repo, "/"); len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return ctx.Ctx.Status(400).JSON(fiber.Map{"error": "Repository must be in owner/repo format."})
	}

	s, err := settings.Get(db)
	if err != nil {
		return ctx.Ctx.Status(500).JSON(fiber.Map{"error": "failed to load settings"})
	}
	if !s.IsGitHubConfigured() {
		return ctx.Ctx.Status(400).JSON(fiber.Map{"error": "Connect a GitHub token in Settings → Developer first."})
	}

	token, err := agent.Decrypt(s.GitHubToken, config.GetConfig().PrivateKey)
	if err != nil {
		return ctx.Ctx.Status(500).JSON(fiber.Map{"error": "failed to read token"})
	}
	if ok, _ := agent.CheckRepoAccess(context.Background(), token, repo); !ok {
		return ctx.Ctx.Status(400).JSON(fiber.Map{
			"error": "Your GitHub token can't reach " + repo + ". Add it to the token's selected repositories, or check the name.",
		})
	}

	if err := apikeys.SetRepo(db, uint(id), repo); err != nil {
		return ctx.Ctx.Status(500).JSON(fiber.Map{"error": "failed to save repository"})
	}

	return ctx.Ctx.JSON(fiber.Map{"success": true, "github_repo": repo})
}

// DisconnectAppRepoAction handles POST /apps/api-keys/:id/github/disconnect.
func DisconnectAppRepoAction(ctx *cartridge.Context) error {
	db := ctx.DB()

	id, err := strconv.ParseUint(ctx.Ctx.Params("id"), 10, 64)
	if err != nil {
		return ctx.Ctx.Status(400).JSON(fiber.Map{"error": "invalid app"})
	}
	if err := apikeys.ClearRepo(db, uint(id)); err != nil {
		return ctx.Ctx.Status(500).JSON(fiber.Map{"error": "failed to disconnect"})
	}

	return ctx.Ctx.JSON(fiber.Map{"success": true})
}
