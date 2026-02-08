package http

import (
	"bufio"
	"encoding/json"
	"fmt"
	"strconv"
	"sync"

	"github.com/gofiber/fiber/v2"
	"github.com/karloscodes/cartridge"
	"github.com/valyala/fasthttp"

	"lognorth/internal/events"
)

// EventSubscriber represents a subscriber to live events
type EventSubscriber struct {
	ID       string
	AppID    uint // 0 means all apps
	Events   chan *events.Event
}

var (
	subscribers   = make(map[string]*EventSubscriber)
	subscribersMu sync.RWMutex
	subscriberID  uint64
)

// NotifyEventSubscribers sends an event to all SSE subscribers
// Only sends to subscribers watching the specific app or all apps
func NotifyEventSubscribers(event *events.Event) {
	subscribersMu.RLock()
	defer subscribersMu.RUnlock()

	for _, sub := range subscribers {
		// Filter by app ID: send if subscriber wants all apps (0) or matches this app
		if sub.AppID != 0 && sub.AppID != event.APIKeyID {
			continue
		}

		select {
		case sub.Events <- event:
		default:
			// Channel full, skip this subscriber
		}
	}
}

// LiveEventsAction handles GET /api/v1/events/live (SSE streaming)
func LiveEventsAction(ctx *cartridge.Context) error {
	// Parse app_id filter
	var appID uint
	if appIDStr := ctx.Ctx.Query("app_id"); appIDStr != "" {
		if id, err := strconv.ParseUint(appIDStr, 10, 64); err == nil {
			appID = uint(id)
		}
	}

	// Set SSE headers
	ctx.Ctx.Set("Content-Type", "text/event-stream")
	ctx.Ctx.Set("Cache-Control", "no-cache")
	ctx.Ctx.Set("Connection", "keep-alive")
	ctx.Ctx.Set("Transfer-Encoding", "chunked")

	// Create subscriber
	subscribersMu.Lock()
	subscriberID++
	id := fmt.Sprintf("sub-%d", subscriberID)
	sub := &EventSubscriber{
		ID:     id,
		AppID:  appID,
		Events: make(chan *events.Event, 100),
	}
	subscribers[id] = sub
	subscribersMu.Unlock()

	// Clean up on disconnect
	defer func() {
		subscribersMu.Lock()
		delete(subscribers, id)
		close(sub.Events)
		subscribersMu.Unlock()
	}()

	// Stream events
	ctx.Ctx.Context().SetBodyStreamWriter(func(w *bufio.Writer) {
		// Send initial connection message
		w.WriteString("event: connected\n")
		w.WriteString("data: {\"status\":\"connected\"}\n\n")
		w.Flush()

		for event := range sub.Events {
			data, err := json.Marshal(event)
			if err != nil {
				continue
			}

			w.WriteString("event: event\n")
			w.WriteString("data: ")
			w.Write(data)
			w.WriteString("\n\n")

			if err := w.Flush(); err != nil {
				return
			}
		}
	})

	return nil
}

// LiveEventsRawAction handles GET /api/v1/events/live without session middleware
// This is a simpler implementation for API access
func LiveEventsRawAction(c *fiber.Ctx) error {
	// Parse app_id filter
	var appID uint
	if appIDStr := c.Query("app_id"); appIDStr != "" {
		if id, err := strconv.ParseUint(appIDStr, 10, 64); err == nil {
			appID = uint(id)
		}
	}

	// Set SSE headers
	c.Set("Content-Type", "text/event-stream")
	c.Set("Cache-Control", "no-cache")
	c.Set("Connection", "keep-alive")

	// Create subscriber
	subscribersMu.Lock()
	subscriberID++
	id := fmt.Sprintf("sub-%d", subscriberID)
	sub := &EventSubscriber{
		ID:     id,
		AppID:  appID,
		Events: make(chan *events.Event, 100),
	}
	subscribers[id] = sub
	subscribersMu.Unlock()

	// Clean up on disconnect
	defer func() {
		subscribersMu.Lock()
		delete(subscribers, id)
		close(sub.Events)
		subscribersMu.Unlock()
	}()

	c.Context().SetBodyStreamWriter(fasthttp.StreamWriter(func(w *bufio.Writer) {
		// Send initial connection message
		w.WriteString("event: connected\n")
		w.WriteString("data: {\"status\":\"connected\"}\n\n")
		w.Flush()

		for event := range sub.Events {
			data, err := json.Marshal(event)
			if err != nil {
				continue
			}

			w.WriteString("event: event\n")
			w.WriteString("data: ")
			w.Write(data)
			w.WriteString("\n\n")

			if err := w.Flush(); err != nil {
				return
			}
		}
	}))

	return nil
}
