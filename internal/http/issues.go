package http

import (
	"strconv"

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

// IssueDetailPageAction handles GET /issues/:hash - redirects to filtered logs
func IssueDetailPageAction(ctx *cartridge.Context) error {
	db := ctx.DB()
	hash := ctx.Ctx.Params("hash")

	if hash == "" {
		return ctx.Ctx.Redirect("/issues")
	}

	// Find which app this issue belongs to
	issue, err := events.GetIssueByHash(db, hash, 0)
	if err != nil || issue == nil {
		return ctx.Ctx.Redirect("/issues")
	}

	// Mark as seen
	events.MarkIssueSeen(db, hash)

	// Redirect to logs filtered by this error
	return ctx.Ctx.Redirect("/apps/" + strconv.FormatUint(uint64(issue.APIKeyID), 10) + "/log?error=" + hash + "&is_error=true")
}

// AppIssueDetailPageAction handles GET /apps/:id/issues/:hash - redirects to filtered logs
func AppIssueDetailPageAction(ctx *cartridge.Context) error {
	hash := ctx.Ctx.Params("hash")
	appID := ctx.Ctx.Params("id")

	if hash == "" {
		return ctx.Ctx.Redirect("/apps/" + appID + "/issues")
	}

	// Mark as seen
	events.MarkIssueSeen(ctx.DB(), hash)

	// Redirect to logs filtered by this error
	return ctx.Ctx.Redirect("/apps/" + appID + "/log?error=" + hash + "&is_error=true")
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
