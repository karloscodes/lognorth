// Package http provides HTTP handlers for LogNorth
package http

import (
	"github.com/gofiber/fiber/v2"
	"github.com/karloscodes/cartridge/flash"
	"github.com/karloscodes/cartridge/inertia"

	"lognorth/internal/config"
)

var devMode bool

// SetDevMode enables development mode
func SetDevMode(enabled bool) {
	devMode = enabled
	inertia.SetDevMode(enabled)
}

// SetTitle sets the HTML page title
func SetTitle(title string) {
	inertia.SetTitle(title)
}

// Props is a convenience type for Inertia props
type Props = inertia.Props

// RenderPage renders an Inertia page with auto-injected flash
func RenderPage(c *fiber.Ctx, component string, props inertia.Props) error {
	if props == nil {
		props = inertia.Props{}
	}
	if _, exists := props["flash"]; !exists {
		props["flash"] = flash.GetFlash(c)
	}

	cfg := config.GetConfig()

	if cfg.IsDemo() {
		props["isDemo"] = true
	}

	if notifications := systemNotifications(cfg); len(notifications) > 0 {
		props["notifications"] = notifications
	}

	return inertia.RenderPage(c, component, props)
}

// Notification represents a system message shown in the feedback widget.
type Notification struct {
	ID      string `json:"id"`
	Title   string `json:"title"`
	Body    string `json:"body"`
	Expires string `json:"expires"` // ISO 8601 date, e.g. "2026-04-22"
}

// systemNotifications returns active notifications for the current state.
func systemNotifications(cfg *config.Config) []Notification {
	var notifications []Notification

	if cfg.CLINeedsUpdate() {
		notifications = append(notifications, Notification{
			ID:      "cli-selfupdate-fix-2026-03",
			Title:   "CLI update available",
			Body:    "If you are using the LogNorth manager, run this command on your server to patch a self-updating issue:\n\ncurl -fsSL https://github.com/karloscodes/lognorth-releases/releases/latest/download/lognorth-linux-$(uname -m | sed 's/x86_64/amd64/' | sed 's/aarch64/arm64/') -o /tmp/lognorth && chmod +x /tmp/lognorth && sudo mv /tmp/lognorth /usr/local/bin/lognorth",
			Expires: "2026-04-22",
		})
	}

	return notifications
}
