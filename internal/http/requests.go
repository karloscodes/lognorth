package http

import (
	"net/http"
	"strconv"
	"time"

	"github.com/karloscodes/cartridge"

	"lognorth/internal/events"
)

// EventsAPIAction handles GET /api/v1/events
func EventsAPIAction(ctx *cartridge.Context) error {
	db := ctx.DB()
	filters := parseFilters(ctx)

	eventList, total, err := events.List(db, filters)
	if err != nil {
		return ctx.Status(http.StatusInternalServerError).JSON(cartridge.Map{"error": "Failed to fetch events"})
	}

	return ctx.JSON(cartridge.Map{
		"events": eventList,
		"total":  total,
	})
}

func parseFilters(c *cartridge.Context) events.ListFilters {
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

	if appID, err := strconv.Atoi(c.Query("app_id")); err == nil && appID > 0 {
		filters.APIKeyID = uint(appID)
	}
	if limit, err := strconv.Atoi(c.Query("limit")); err == nil && limit > 0 {
		filters.Limit = min(limit, 200)
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
