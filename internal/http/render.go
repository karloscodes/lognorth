// Package http provides HTTP handlers for LogNorth
package http

import (
	"github.com/gofiber/fiber/v2"
	"github.com/karloscodes/cartridge/flash"
	"github.com/karloscodes/cartridge/inertia"

	"lognorth/internal/config"
	"lognorth/internal/license"
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

	// Surface license status to the layout so it can warn when events
	// are arriving but not being processed.
	if _, exists := props["licenseValid"]; !exists {
		props["licenseValid"] = license.IsConfigured()
	}

	return inertia.RenderPage(c, component, props)
}
