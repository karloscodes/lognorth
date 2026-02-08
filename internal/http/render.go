// Package http provides HTTP handlers for LogNorth
package http

import (
	"github.com/gofiber/fiber/v2"
	"github.com/karloscodes/cartridge/flash"
	"github.com/karloscodes/cartridge/inertia"
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
	// Auto-inject flash
	if props == nil {
		props = inertia.Props{}
	}
	if _, exists := props["flash"]; !exists {
		props["flash"] = flash.GetFlash(c)
	}

	return inertia.RenderPage(c, component, props)
}
