package http

import (
	"strconv"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/karloscodes/cartridge"

	"lognorth/internal/apikeys"
	"lognorth/internal/events"
)

// parseTimestamp handles SQLite timestamp formats
func parseTimestamp(s string) time.Time {
	for _, layout := range []string{
		time.RFC3339,
		"2006-01-02 15:04:05-07:00",
		"2006-01-02 15:04:05+00:00",
		"2006-01-02T15:04:05Z",
		"2006-01-02 15:04:05",
	} {
		if t, err := time.Parse(layout, s); err == nil {
			return t
		}
	}
	return time.Time{}
}

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

// IssueDetailPageAction handles GET /issues/:hash - redirects to app-scoped issue page
func IssueDetailPageAction(ctx *cartridge.Context) error {
	db := ctx.DB()
	hash := ctx.Ctx.Params("hash")

	if hash == "" {
		return ctx.Ctx.Redirect("/issues")
	}

	issue, err := events.GetIssueByHash(db, hash, 0)
	if err != nil || issue == nil {
		return ctx.Ctx.Redirect("/issues")
	}

	return ctx.Ctx.Redirect("/apps/" + strconv.FormatUint(uint64(issue.APIKeyID), 10) + "/issues/" + hash)
}

// AppIssueDetailPageAction handles GET /apps/:id/issues/:hash - renders issue detail page
func AppIssueDetailPageAction(ctx *cartridge.Context) error {
	hash := ctx.Ctx.Params("hash")

	app, apps, err := getAppFromURL(ctx)
	if err != nil {
		return ctx.Ctx.Redirect("/")
	}

	if hash == "" {
		return ctx.Ctx.Redirect("/apps/" + strconv.FormatUint(uint64(app.ID), 10) + "/issues")
	}

	db := ctx.DB()

	issue, err := events.GetIssueByHash(db, hash, app.ID)
	if err != nil || issue == nil {
		return ctx.Ctx.Redirect("/apps/" + strconv.FormatUint(uint64(app.ID), 10) + "/issues")
	}

	// Compute days to cover the full issue history (min 7, max 30)
	days := 7
	if issue.FirstSeen != "" {
		firstSeen := parseTimestamp(issue.FirstSeen)
		if !firstSeen.IsZero() {
			d := int(time.Since(firstSeen).Hours()/24) + 1
			if d > days {
				days = d
			}
			if days > 30 {
				days = 30
			}
		}
	}

	occurrences, _ := events.GetIssueOccurrencesByHour(db, hash, app.ID, days)
	events.MarkIssueSeen(db, hash)

	// Load trace siblings for the top 20 entries
	traces := make(map[string][]events.Event)
	seen := make(map[string]bool)
	limit := 20
	if len(issue.Entries) < limit {
		limit = len(issue.Entries)
	}
	for _, entry := range issue.Entries[:limit] {
		if entry.TraceID != "" && !seen[entry.TraceID] {
			seen[entry.TraceID] = true
			if siblings, err := events.FindByTraceID(db, entry.TraceID); err == nil && len(siblings) > 0 {
				traces[entry.TraceID] = siblings
			}
		}
	}

	return RenderPage(ctx.Ctx, "IssueDetail", Props{
		"issue":        issue,
		"occurrences":  occurrences,
		"traces":       traces,
		"appId":        app.ID,
		"appName":      app.Name,
		"applications": apps,
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

// MuteIssueAction handles POST /issues/:hash/mute
func MuteIssueAction(ctx *cartridge.Context) error {
	db := ctx.DB()
	hash := ctx.Ctx.Params("hash")

	if hash == "" {
		return ctx.Ctx.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Hash required"})
	}

	if err := events.MuteIssue(db, hash); err != nil {
		return ctx.Ctx.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to mute"})
	}

	referer := ctx.Ctx.Get("Referer")
	if referer == "" {
		referer = "/issues"
	}
	return ctx.Ctx.Redirect(referer, fiber.StatusFound)
}

// UnmuteIssueAction handles POST /issues/:hash/unmute
func UnmuteIssueAction(ctx *cartridge.Context) error {
	db := ctx.DB()
	hash := ctx.Ctx.Params("hash")

	if hash == "" {
		return ctx.Ctx.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Hash required"})
	}

	if err := events.UnmuteIssue(db, hash); err != nil {
		return ctx.Ctx.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to unmute"})
	}

	referer := ctx.Ctx.Get("Referer")
	if referer == "" {
		referer = "/issues"
	}
	return ctx.Ctx.Redirect(referer, fiber.StatusFound)
}
