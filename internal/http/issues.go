package http

import (
	"github.com/gofiber/fiber/v2"
	"github.com/karloscodes/cartridge"

	"lognorth/internal/apikeys"
	"lognorth/internal/events"
)

// IssuesPageAction handles GET /issues
func IssuesPageAction(ctx *cartridge.Context) error {
	db := ctx.DB()

	// Get applications list
	keys, _ := apikeys.List(db)
	apps := make([]Application, 0, len(keys))
	for _, k := range keys {
		if k.Active {
			apps = append(apps, Application{ID: k.ID, Name: k.Name})
		}
	}

	issues, err := events.ListIssuesWithState(db, 0, 100) // 0 = all apps
	if err != nil {
		return ctx.Ctx.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to fetch issues"})
	}

	return RenderPage(ctx.Ctx, "Issues", Props{
		"applications": apps,
		"issues":       issues,
	})
}

// IssuesAPIAction handles GET /api/v1/issues
func IssuesAPIAction(ctx *cartridge.Context) error {
	db := ctx.DB()

	issues, err := events.ListIssuesWithState(db, 0, 100) // 0 = all apps
	if err != nil {
		return ctx.Ctx.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to fetch issues"})
	}

	return ctx.Ctx.JSON(fiber.Map{
		"issues": issues,
	})
}

// IssueDetailPageAction handles GET /issues/:hash
func IssueDetailPageAction(ctx *cartridge.Context) error {
	db := ctx.DB()
	hash := ctx.Ctx.Params("hash")

	if hash == "" {
		return ctx.Ctx.Redirect("/")
	}

	// Get applications list
	keys, _ := apikeys.List(db)
	apps := make([]Application, 0, len(keys))
	appNameByID := make(map[uint]string)
	for _, k := range keys {
		if k.Active {
			apps = append(apps, Application{ID: k.ID, Name: k.Name})
			appNameByID[k.ID] = k.Name
		}
	}

	issue, err := events.GetIssueByHash(db, hash, 0)
	if err != nil || issue == nil {
		return ctx.Ctx.Redirect("/")
	}

	// Mark as seen when viewing
	events.MarkIssueSeen(db, hash)

	// Get app name for the issue
	appName := appNameByID[issue.APIKeyID]

	return RenderPage(ctx.Ctx, "IssueDetail", Props{
		"applications": apps,
		"issue":        issue,
		"appId":        issue.APIKeyID,
		"appName":      appName,
	})
}

// AppIssueDetailPageAction handles GET /apps/:id/issues/:hash
func AppIssueDetailPageAction(ctx *cartridge.Context) error {
	db := ctx.DB()
	hash := ctx.Ctx.Params("hash")

	app, apps, err := getAppFromURL(ctx)
	if err != nil {
		return ctx.Ctx.Redirect("/")
	}

	if hash == "" {
		return ctx.Ctx.Redirect("/apps/" + ctx.Ctx.Params("id") + "/issues")
	}

	issue, err := events.GetIssueByHash(db, hash, app.ID)
	if err != nil || issue == nil {
		return ctx.Ctx.Redirect("/apps/" + ctx.Ctx.Params("id") + "/issues")
	}

	// Mark as seen when viewing
	events.MarkIssueSeen(db, hash)

	return RenderPage(ctx.Ctx, "IssueDetail", Props{
		"appId":        app.ID,
		"appName":      app.Name,
		"applications": apps,
		"issue":        issue,
	})
}

// MarkIssueSeenAction handles POST /issues/:hash/seen (kept for API use)
func MarkIssueSeenAction(ctx *cartridge.Context) error {
	db := ctx.DB()
	hash := ctx.Ctx.Params("hash")

	if hash == "" {
		return ctx.Ctx.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Hash required"})
	}

	if err := events.MarkIssueSeen(db, hash); err != nil {
		return ctx.Ctx.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to mark seen"})
	}

	// Redirect back
	referer := ctx.Ctx.Get("Referer")
	if referer == "" {
		referer = "/issues"
	}
	return ctx.Ctx.Redirect(referer, fiber.StatusFound)
}

// MarkAllIssuesSeenAction handles POST /issues/seen
func MarkAllIssuesSeenAction(ctx *cartridge.Context) error {
	db := ctx.DB()
	apiKeyID := uint(ctx.Ctx.QueryInt("app_id", 0))

	if err := events.MarkAllIssuesSeen(db, apiKeyID); err != nil {
		return ctx.Ctx.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to mark all seen"})
	}

	// Redirect back to issues page
	referer := ctx.Ctx.Get("Referer")
	if referer == "" {
		referer = "/issues"
	}
	return ctx.Ctx.Redirect(referer, fiber.StatusFound)
}
