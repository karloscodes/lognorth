package http

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/karloscodes/cartridge"

	"lognorth/internal/events"
	"lognorth/internal/uptime"
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

// IssueDetailPageAction handles GET /issues/:hash - redirects to app-scoped issue page
func IssueDetailPageAction(ctx *cartridge.Context) error {
	db := ctx.DB()
	hash := ctx.Params("hash")

	if hash == "" {
		return ctx.Redirect("/issues")
	}

	issue, err := events.GetIssueByHash(db, hash, 0)
	if err != nil || issue == nil {
		return ctx.Redirect("/issues")
	}

	return ctx.Redirect("/apps/" + strconv.FormatUint(uint64(issue.APIKeyID), 10) + "/issues/" + hash)
}

// AppIssueDetailPageAction handles GET /apps/:id/issues/:hash - renders issue detail page
func AppIssueDetailPageAction(ctx *cartridge.Context) error {
	hash := ctx.Params("hash")

	app, apps, err := getAppFromURL(ctx)
	if err != nil {
		return ctx.Redirect("/")
	}

	if hash == "" {
		return ctx.Redirect("/apps/" + strconv.FormatUint(uint64(app.ID), 10) + "/issues")
	}

	db := ctx.DB()

	issue, err := events.GetIssueByHash(db, hash, app.ID)
	if err != nil || issue == nil {
		return ctx.Redirect("/apps/" + strconv.FormatUint(uint64(app.ID), 10) + "/issues")
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

	// Get related locations (same error at other file+line combinations)
	var alsoHappensAt []events.RelatedLocation
	if len(issue.Entries) > 0 {
		first := issue.Entries[0]
		alsoHappensAt, _ = events.GetRelatedLocations(db, first.ErrorHash1, hash, first.ErrorFile, app.ID)
	}

	// Where it happened: the issue's errors per endpoint.
	paths, err := events.GetIssuePaths(db, hash, app.ID, 20)
	if err != nil {
		paths = []events.PathCount{}
	}

	reach, _ := events.GetIssueReach(db, hash, app.ID)

	// The outages this issue surged in, when the app has an uptime URL.
	outages := []uptime.IssueOutage{}
	if app.PingURL != "" {
		if recent, err := uptime.RecentOutages(db, app.ID, time.Now().Add(-uptime.Retention), 20); err == nil {
			outages = uptime.SurgedIn(recent, hash)
		}
	}

	events.MarkIssueSeen(db, hash)

	// Load trace siblings for the top 20 entries
	traces := make(map[string][]events.Event)
	seen := make(map[string]bool)
	limit := 20
	if len(issue.Entries) < limit {
		limit = len(issue.Entries)
	}
	for _, entry := range issue.Entries[:limit] {
		key := entry.TraceID
		if key == "" {
			key = fmt.Sprintf("nearby-%d", entry.ID)
		}
		if seen[key] {
			continue
		}
		seen[key] = true

		if entry.TraceID != "" {
			if siblings, err := events.FindByTraceID(db, entry.TraceID); err == nil && len(siblings) > 1 {
				traces[key] = siblings
				continue
			}
		}
		if nearby, err := events.FindNearby(db, app.ID, &entry); err == nil && len(nearby) > 1 {
			traces[key] = nearby
		}
	}

	return RenderPage(ctx, "IssueDetail", Props{
		"issue":         issue,
		"occurrences":   occurrences,
		"traces":        traces,
		"alsoHappensAt": alsoHappensAt,
		"paths":         paths,
		"reach":         reach,
		"outages":       outages,
		"appId":         app.ID,
		"appName":       app.Name,
		"applications":  apps,
	})
}

// MarkIssueSeenAction handles POST /issues/:hash/seen (kept for API use)
func MarkIssueSeenAction(ctx *cartridge.Context) error {
	db := ctx.DB()
	hash := ctx.Params("hash")

	if hash == "" {
		return ctx.Status(http.StatusBadRequest).JSON(cartridge.Map{"error": "Hash required"})
	}

	if err := events.MarkIssueSeen(db, hash); err != nil {
		return ctx.Status(http.StatusInternalServerError).JSON(cartridge.Map{"error": "Failed to mark seen"})
	}

	return ctx.RedirectBack("/issues")
}

// MarkAllIssuesSeenAction handles POST /issues/seen
func MarkAllIssuesSeenAction(ctx *cartridge.Context) error {
	db := ctx.DB()
	appID, _ := strconv.Atoi(ctx.Query("app_id"))
	apiKeyID := uint(appID)

	if err := events.MarkAllIssuesSeen(db, apiKeyID); err != nil {
		return ctx.Status(http.StatusInternalServerError).JSON(cartridge.Map{"error": "Failed to mark all seen"})
	}

	return ctx.RedirectBack("/issues")
}

// MuteIssueAction handles POST /issues/:hash/mute
func MuteIssueAction(ctx *cartridge.Context) error {
	db := ctx.DB()
	hash := ctx.Params("hash")

	if hash == "" {
		return ctx.Status(http.StatusBadRequest).JSON(cartridge.Map{"error": "Hash required"})
	}

	if err := events.MuteIssue(db, hash); err != nil {
		return ctx.Status(http.StatusInternalServerError).JSON(cartridge.Map{"error": "Failed to mute"})
	}

	return ctx.RedirectBack("/issues")
}

// UnmuteIssueAction handles POST /issues/:hash/unmute
func UnmuteIssueAction(ctx *cartridge.Context) error {
	db := ctx.DB()
	hash := ctx.Params("hash")

	if hash == "" {
		return ctx.Status(http.StatusBadRequest).JSON(cartridge.Map{"error": "Hash required"})
	}

	if err := events.UnmuteIssue(db, hash); err != nil {
		return ctx.Status(http.StatusInternalServerError).JSON(cartridge.Map{"error": "Failed to unmute"})
	}

	return ctx.RedirectBack("/issues")
}
