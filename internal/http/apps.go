package http

import (
	"net/http"
	"strconv"
	"time"

	"github.com/karloscodes/cartridge"

	"lognorth/internal/apikeys"
	"lognorth/internal/events"
	"lognorth/internal/uptime"
)

// Application represents an app for the frontend
type Application struct {
	ID     uint          `json:"id"`
	Name   string        `json:"name"`
	Uptime *uptime.State `json:"uptime,omitempty"` // for the status bar and the app picker
}

// getAppFromURL extracts app ID from URL and validates it
func formatTimeOrEmpty(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format(time.RFC3339)
}

func getAppFromURL(ctx *cartridge.Context) (*apikeys.APIKey, []Application, error) {
	db := ctx.DB()

	id, err := strconv.ParseUint(ctx.Params("id"), 10, 64)
	if err != nil {
		return nil, nil, err
	}

	app, err := apikeys.FindByID(db, uint(id))
	if err != nil {
		return nil, nil, err
	}

	// Build applications list, each with its uptime at a glance.
	keys, _ := apikeys.List(db)
	states := uptime.States(db, keys, time.Now())
	apps := make([]Application, 0, len(keys))
	for _, k := range keys {
		if k.Active {
			state := states[k.ID]
			apps = append(apps, Application{ID: k.ID, Name: k.Name, Uptime: &state})
		}
	}

	return app, apps, nil
}

// AppLogPageAction handles GET /apps/:id/log
func AppLogPageAction(ctx *cartridge.Context) error {
	db := ctx.DB()

	app, apps, err := getAppFromURL(ctx)
	if err != nil {
		return ctx.Redirect("/")
	}

	filters := parseAppFilters(ctx, app.ID)
	entries, total, err := events.List(db, filters)
	if err != nil {
		return ctx.Status(http.StatusInternalServerError).JSON(cartridge.Map{"error": "Failed to fetch log"})
	}

	// Get histogram data - 48 buckets for the time range
	histogram := events.GetHistogram(db, app.ID, filters.StartTime, filters.EndTime)
	releases := []events.Release{}
	if len(histogram) > 0 {
		releases = events.ReleasesBetween(db, app.ID, histogram[0].Timestamp, time.Now())
	}

	return RenderPage(ctx, "Log", Props{
		"appId":        app.ID,
		"appName":      app.Name,
		"applications": apps,
		"entries":      entries,
		"histogram":    histogram,
		"releases":     releases,
		"filters": cartridge.Map{
			"is_error": filters.IsError,
			"search":   filters.Search,
			"error":    filters.ErrorHash,
			"trace":    filters.TraceID,
			"start_time": func() string {
				if ctx.Query("start_time") == "all" {
					return "all"
				}
				return formatTimeOrEmpty(filters.StartTime)
			}(),
			"end_time": formatTimeOrEmpty(filters.EndTime),
		},
		"pagination": cartridge.Map{
			"limit":  filters.Limit,
			"offset": filters.Offset,
			"total":  total,
		},
	})
}

// AppIssuesPageAction handles GET /apps/:id/issues
func AppIssuesPageAction(ctx *cartridge.Context) error {
	db := ctx.DB()

	app, apps, err := getAppFromURL(ctx)
	if err != nil {
		return ctx.Redirect("/")
	}

	issues, err := events.ListIssuesWithState(db, app.ID, 100)
	if err != nil {
		return ctx.Status(http.StatusInternalServerError).JSON(cartridge.Map{"error": "Failed to fetch issues"})
	}
	causes, _ := events.CausesByIssue(db, app.ID, time.Now())
	if causes == nil {
		causes = map[string][]events.IssueLink{}
	}

	return RenderPage(ctx, "Issues", Props{
		"appId":        app.ID,
		"appName":      app.Name,
		"applications": apps,
		"issues":       issues,
		"causes":       causes,
	})
}

// AppLogDetailPageAction handles GET /apps/:id/log/:eventId
func AppLogDetailPageAction(ctx *cartridge.Context) error {
	db := ctx.DB()

	app, apps, err := getAppFromURL(ctx)
	if err != nil {
		return ctx.Redirect("/")
	}

	entryId, err := strconv.ParseUint(ctx.Params("eventId"), 10, 64)
	if err != nil {
		return ctx.Status(http.StatusBadRequest).JSON(cartridge.Map{"error": "Invalid ID"})
	}

	entry, err := events.FindByID(db, uint(entryId))
	if err != nil {
		return ctx.Status(http.StatusNotFound).JSON(cartridge.Map{"error": "Not found"})
	}

	relatedEntries := []events.Event{}
	if entry.TraceID != "" {
		relatedEntries, _ = events.FindByTraceID(db, entry.TraceID)
	}
	if len(relatedEntries) <= 1 {
		relatedEntries, _ = events.FindNearby(db, app.ID, entry)
	}

	entryContext, _ := entry.GetContext()

	props := Props{
		"appId":          app.ID,
		"appName":        app.Name,
		"applications":   apps,
		"entry":          entry,
		"entryContext":   entryContext,
		"relatedEntries": relatedEntries,
		"stepsBefore":    events.StepsBefore(db, entry),
	}

	if entry.IsError && entry.IssueHash != "" {
		props["issueHash"] = entry.IssueHash
	}

	return RenderPage(ctx, "LogDetail", props)
}

// TracePageAction handles GET /trace/:traceId - redirects to logs filtered by trace
func TracePageAction(ctx *cartridge.Context) error {
	db := ctx.DB()
	traceID := ctx.Params("traceId")

	if traceID == "" {
		return ctx.Redirect("/")
	}

	entries, err := events.FindByTraceID(db, traceID)
	if err != nil || len(entries) == 0 {
		return ctx.Redirect("/")
	}

	// Redirect to logs filtered by this trace
	appID := entries[0].APIKeyID
	return ctx.Redirect("/apps/" + strconv.FormatUint(uint64(appID), 10) + "/log?trace=" + traceID)
}

// EndpointsPageAction handles GET /apps/:id/endpoints
func EndpointsPageAction(ctx *cartridge.Context) error {
	db := ctx.DB()

	app, apps, err := getAppFromURL(ctx)
	if err != nil {
		return ctx.Redirect("/")
	}

	endpoints, err := events.GetEndpointStats(db, app.ID, 50)
	if err != nil {
		return ctx.Status(http.StatusInternalServerError).JSON(cartridge.Map{"error": "Failed to fetch endpoints"})
	}

	// Endpoints alerting now, compared with their own history. The page marks
	// these instead of coloring rows by a fixed error rate.
	alerts, err := events.ListAlerts(db, app.ID)
	if err != nil {
		alerts = []events.Alert{}
	}

	return RenderPage(ctx, "Endpoints", Props{
		"appId":        app.ID,
		"appName":      app.Name,
		"applications": apps,
		"endpoints":    endpoints,
		"alerts":       alerts,
		"uptime":       uptime.ReportFor(db, *app, time.Now(), 24*time.Hour, 96),
		"releases":     events.ReleasesBetween(db, app.ID, time.Now().Add(-24*time.Hour), time.Now()),
	})
}

func parseAppFilters(c *cartridge.Context, appID uint) events.ListFilters {
	filters := events.ListFilters{
		APIKeyID:  appID,
		Search:    c.Query("search"),
		ErrorHash: c.Query("error"),
		TraceID:   c.Query("trace"),
		Limit:     50,
	}

	// Parse is_error filter: "true" = errors only, "false" = success only, empty = all
	if isErrorStr := c.Query("is_error"); isErrorStr != "" {
		if isErrorStr == "true" {
			isError := true
			filters.IsError = &isError
		} else if isErrorStr == "false" {
			isError := false
			filters.IsError = &isError
		}
	}

	if limit, err := strconv.Atoi(c.Query("limit")); err == nil && limit > 0 {
		filters.Limit = limit
	}
	if offset, err := strconv.Atoi(c.Query("offset")); err == nil && offset > 0 {
		filters.Offset = offset
	}
	if startTime := c.Query("start_time"); startTime == "all" {
		// Explicit "all time" — no time filter
	} else if startTime != "" {
		if t, err := time.Parse(time.RFC3339, startTime); err == nil {
			filters.StartTime = t
		}
	} else {
		// Default to last 7 days
		filters.StartTime = time.Now().AddDate(0, 0, -7)
	}
	if endTime := c.Query("end_time"); endTime != "" {
		if t, err := time.Parse(time.RFC3339, endTime); err == nil {
			filters.EndTime = t
		}
	}

	return filters
}
