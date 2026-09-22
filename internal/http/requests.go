package http

import (
	"strconv"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/karloscodes/cartridge"

	"lognorth/internal/apikeys"
	"lognorth/internal/events"
)

// EventsPageAction handles GET /events
func EventsPageAction(ctx *cartridge.Context) error {
	db := ctx.DB()
	filters := parseFilters(ctx.Ctx)

	// Get applications list
	keys, _ := apikeys.List(db)
	apps := make([]Application, 0, len(keys))
	for _, k := range keys {
		if k.Active {
			apps = append(apps, Application{ID: k.ID, Name: k.Name})
		}
	}

	eventList, total, err := events.List(db, filters)
	if err != nil {
		return ctx.Ctx.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to fetch events"})
	}

	return RenderPage(ctx.Ctx, "Events", Props{
		"applications": apps,
		"events":       eventList,
		"filters": fiber.Map{
			"is_error": filters.IsError,
			"search":   filters.Search,
		},
		"pagination": fiber.Map{
			"limit":  filters.Limit,
			"offset": filters.Offset,
			"total":  total,
		},
	})
}

// EventsAPIAction handles GET /api/v1/events
func EventsAPIAction(ctx *cartridge.Context) error {
	db := ctx.DB()
	filters := parseFilters(ctx.Ctx)

	eventList, total, err := events.List(db, filters)
	if err != nil {
		return ctx.Ctx.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to fetch events"})
	}

	return ctx.Ctx.JSON(fiber.Map{
		"events": eventList,
		"total":  total,
	})
}

// EventDetailPageAction handles GET /events/:id
func EventDetailPageAction(ctx *cartridge.Context) error {
	db := ctx.DB()

	id, err := strconv.ParseUint(ctx.Ctx.Params("id"), 10, 64)
	if err != nil {
		return ctx.Ctx.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid event ID"})
	}

	event, err := events.FindByID(db, uint(id))
	if err != nil {
		return ctx.Ctx.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Event not found"})
	}

	var relatedEvents []events.Event
	if event.TraceID != "" {
		relatedEvents, _ = events.FindByTraceID(db, event.TraceID)
	}

	eventContext, _ := event.GetContext()

	return RenderPage(ctx.Ctx, "EventDetail", Props{
		"event":         event,
		"eventContext":  eventContext,
		"relatedEvents": relatedEvents,
	})
}

func parseFilters(c *fiber.Ctx) events.ListFilters {
	filters := events.ListFilters{
		Search: c.Query("search"),
		Limit:  50,
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
