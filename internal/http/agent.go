package http

import (
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/karloscodes/cartridge"

	"lognorth/internal/apikeys"
	"lognorth/internal/events"
	"lognorth/internal/settings"
)

// AgentAppsAction handles GET /api/v1/agent/apps
// Returns list of apps (API keys) for filtering
func AgentAppsAction(ctx *cartridge.Context) error {
	db := ctx.DB()

	if err := validateAgentAuth(ctx); err != nil {
		return err
	}

	keys, err := apikeys.List(db)
	if err != nil {
		return ctx.Ctx.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to fetch apps",
		})
	}

	// Return simplified app info
	apps := make([]fiber.Map, len(keys))
	for i, k := range keys {
		apps[i] = fiber.Map{
			"id":   k.ID,
			"name": k.Name,
		}
	}

	return ctx.Ctx.JSON(fiber.Map{
		"apps": apps,
	})
}

// AgentEventsAction handles GET /api/v1/agent/events
// Returns events across all apps for authenticated agents
func AgentEventsAction(ctx *cartridge.Context) error {
	db := ctx.DB()

	if err := validateAgentAuth(ctx); err != nil {
		return err
	}

	filters := parseFilters(ctx.Ctx)

	// Add app_id filter support
	if appID, err := strconv.Atoi(ctx.Ctx.Query("app_id")); err == nil && appID > 0 {
		filters.APIKeyID = uint(appID)
	}

	eventList, total, err := events.List(db, filters)
	if err != nil {
		return ctx.Ctx.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to fetch events",
		})
	}

	return ctx.Ctx.JSON(fiber.Map{
		"events": eventList,
		"total":  total,
	})
}

// AgentEventDetailAction handles GET /api/v1/agent/events/:id
// Returns event detail with related trace events
func AgentEventDetailAction(ctx *cartridge.Context) error {
	db := ctx.DB()

	if err := validateAgentAuth(ctx); err != nil {
		return err
	}

	id, err := strconv.ParseUint(ctx.Ctx.Params("id"), 10, 64)
	if err != nil {
		return ctx.Ctx.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid event ID",
		})
	}

	event, err := events.FindByID(db, uint(id))
	if err != nil {
		return ctx.Ctx.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "Event not found",
		})
	}

	var relatedEvents []events.Event
	if event.TraceID != "" {
		relatedEvents, _ = events.FindByTraceID(db, event.TraceID)
	}

	eventContext, _ := event.GetContext()

	return ctx.Ctx.JSON(fiber.Map{
		"event":         event,
		"eventContext":  eventContext,
		"relatedEvents": relatedEvents,
	})
}

// AgentIssuesAction handles GET /api/v1/agent/issues
// Returns grouped errors with occurrence counts
func AgentIssuesAction(ctx *cartridge.Context) error {
	db := ctx.DB()

	if err := validateAgentAuth(ctx); err != nil {
		return err
	}

	// Support app_id filter
	appID := uint(0)
	if id, err := strconv.Atoi(ctx.Ctx.Query("app_id")); err == nil && id > 0 {
		appID = uint(id)
	}

	issues, err := events.ListIssuesWithState(db, appID, 100)
	if err != nil {
		return ctx.Ctx.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to fetch issues",
		})
	}

	return ctx.Ctx.JSON(fiber.Map{
		"issues": issues,
	})
}

// validateAgentAuth checks for valid agent API key in Authorization header
func validateAgentAuth(ctx *cartridge.Context) error {
	authHeader := ctx.Ctx.Get("Authorization")
	key := strings.TrimPrefix(authHeader, "Bearer ")

	if key == "" {
		return ctx.Ctx.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": "API key required",
		})
	}

	if err := settings.ValidateAgentKey(ctx.DB(), key); err != nil {
		return ctx.Ctx.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": "Invalid agent key",
		})
	}

	return nil
}
