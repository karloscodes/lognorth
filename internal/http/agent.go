package http

import (
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/karloscodes/cartridge"

	"lognorth/internal/apikeys"
	"lognorth/internal/events"
)

// AgentEventsAction handles GET /api/v1/agent/events
// Returns events across all apps for authenticated agents
func AgentEventsAction(ctx *cartridge.Context) error {
	db := ctx.DB()

	if err := validateAgentAuth(ctx); err != nil {
		return err
	}

	filters := parseFilters(ctx.Ctx)

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

	issues, err := events.ListIssuesWithState(db, 0, 100) // 0 = all apps
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
// Only agent keys (lna_ prefix) are accepted, not app keys
func validateAgentAuth(ctx *cartridge.Context) error {
	authHeader := ctx.Ctx.Get("Authorization")
	apiKey := strings.TrimPrefix(authHeader, "Bearer ")

	if apiKey == "" {
		return ctx.Ctx.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": "API key required",
		})
	}

	_, err := apikeys.ValidateAgentKey(ctx.DB(), apiKey)
	if err != nil {
		return ctx.Ctx.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": "Invalid agent key",
		})
	}

	return nil
}
