package http

import (
	"strconv"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/karloscodes/cartridge"

	"lognorth/internal/apikeys"
	"lognorth/internal/events"
)

// Application represents an app for the frontend
type Application struct {
	ID   uint   `json:"id"`
	Name string `json:"name"`
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

	id, err := strconv.ParseUint(ctx.Ctx.Params("id"), 10, 64)
	if err != nil {
		return nil, nil, err
	}

	app, err := apikeys.FindByID(db, uint(id))
	if err != nil {
		return nil, nil, err
	}

	// Build applications list
	keys, _ := apikeys.List(db)
	apps := make([]Application, 0, len(keys))
	for _, k := range keys {
		if k.Active {
			apps = append(apps, Application{ID: k.ID, Name: k.Name})
		}
	}

	return app, apps, nil
}

// AppLogPageAction handles GET /apps/:id/log
func AppLogPageAction(ctx *cartridge.Context) error {
	db := ctx.DB()

	app, apps, err := getAppFromURL(ctx)
	if err != nil {
		return ctx.Ctx.Redirect("/")
	}

	filters := parseAppFilters(ctx.Ctx, app.ID)
	entries, total, err := events.List(db, filters)
	if err != nil {
		return ctx.Ctx.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to fetch log"})
	}

	return RenderPage(ctx.Ctx, "Log", Props{
		"appId":        app.ID,
		"appName":      app.Name,
		"applications": apps,
		"entries":      entries,
		"filters": fiber.Map{
			"is_error":   filters.IsError,
			"search":     filters.Search,
			"error":      filters.ErrorHash,
			"start_time": formatTimeOrEmpty(filters.StartTime),
			"end_time":   formatTimeOrEmpty(filters.EndTime),
		},
		"pagination": fiber.Map{
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
		return ctx.Ctx.Redirect("/")
	}

	issues, err := events.ListIssuesWithState(db, app.ID, 100)
	if err != nil {
		return ctx.Ctx.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to fetch issues"})
	}

	return RenderPage(ctx.Ctx, "Issues", Props{
		"appId":        app.ID,
		"appName":      app.Name,
		"applications": apps,
		"issues":       issues,
	})
}

// AppLogDetailPageAction handles GET /apps/:id/log/:eventId
func AppLogDetailPageAction(ctx *cartridge.Context) error {
	db := ctx.DB()

	app, apps, err := getAppFromURL(ctx)
	if err != nil {
		return ctx.Ctx.Redirect("/")
	}

	entryId, err := strconv.ParseUint(ctx.Ctx.Params("eventId"), 10, 64)
	if err != nil {
		return ctx.Ctx.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid ID"})
	}

	entry, err := events.FindByID(db, uint(entryId))
	if err != nil {
		return ctx.Ctx.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Not found"})
	}

	var relatedEntries []events.Event
	if entry.TraceID != "" {
		relatedEntries, _ = events.FindByTraceID(db, entry.TraceID)
	}

	entryContext, _ := entry.GetContext()

	return RenderPage(ctx.Ctx, "LogDetail", Props{
		"appId":          app.ID,
		"appName":        app.Name,
		"applications":   apps,
		"entry":          entry,
		"entryContext":   entryContext,
		"relatedEntries": relatedEntries,
	})
}

// TracePageAction handles GET /trace/:traceId - redirects to first event's log detail
func TracePageAction(ctx *cartridge.Context) error {
	db := ctx.DB()
	traceID := ctx.Ctx.Params("traceId")

	if traceID == "" {
		return ctx.Ctx.Redirect("/")
	}

	entries, err := events.FindByTraceID(db, traceID)
	if err != nil || len(entries) == 0 {
		return ctx.Ctx.Redirect("/")
	}

	// Redirect to the first event's detail page (trace is shown inline there)
	appID := entries[0].APIKeyID
	eventID := entries[0].ID
	return ctx.Ctx.Redirect("/apps/" + strconv.FormatUint(uint64(appID), 10) + "/log/" + strconv.FormatUint(uint64(eventID), 10))
}

// EndpointsPageAction handles GET /apps/:id/endpoints
func EndpointsPageAction(ctx *cartridge.Context) error {
	db := ctx.DB()

	app, apps, err := getAppFromURL(ctx)
	if err != nil {
		return ctx.Ctx.Redirect("/")
	}

	endpoints, err := events.GetEndpointStats(db, app.ID, 50)
	if err != nil {
		return ctx.Ctx.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to fetch endpoints"})
	}

	// Get alerting status for each endpoint
	alertStatuses := events.GetEndpointAlertStatuses(db, app.ID)

	return RenderPage(ctx.Ctx, "Endpoints", Props{
		"appId":        app.ID,
		"appName":      app.Name,
		"applications": apps,
		"endpoints":    endpoints,
		"alertStatuses": alertStatuses,
	})
}

func parseAppFilters(c *fiber.Ctx, appID uint) events.ListFilters {
	filters := events.ListFilters{
		APIKeyID:  appID,
		Search:    c.Query("search"),
		ErrorHash: c.Query("error"),
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
	if startTime := c.Query("start_time"); startTime != "" {
		if t, err := time.Parse(time.RFC3339, startTime); err == nil {
			filters.StartTime = t
		}
	}
	if endTime := c.Query("end_time"); endTime != "" {
		if t, err := time.Parse(time.RFC3339, endTime); err == nil {
			filters.EndTime = t
		}
	}

	return filters
}
