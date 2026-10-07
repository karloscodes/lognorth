// Package http provides HTTP handlers for LogNorth
package http

import (
	"github.com/karloscodes/cartridge"
	"github.com/karloscodes/cartridge/flash"
	"github.com/karloscodes/cartridge/inertia"

	"lognorth/internal/config"
)

// SetDevMode enables development mode
func SetDevMode(enabled bool) {
	inertia.SetDevMode(enabled)
}

// SetTitle sets the HTML page title
func SetTitle(title string) {
	inertia.SetTitle(title)
}

// Props is a convenience type for Inertia props
type Props = inertia.Props

// RenderPage renders an Inertia page with auto-injected flash
func RenderPage(c *cartridge.Context, component string, props inertia.Props) error {
	if props == nil {
		props = inertia.Props{}
	}
	if _, exists := props["flash"]; !exists {
		props["flash"] = flash.GetFlash(c.Response(), c.Request())
	}

	cfg := config.GetConfig()

	if cfg.IsDemo() {
		props["isDemo"] = true
	}

	return inertia.RenderPage(c.Response(), c.Request(), component, props)
}

// setFlash stores a one-time message for the next page. In production the
// cookie is Secure, like the session cookie: a flash can carry a new API key.
func setFlash(ctx *cartridge.Context, messageType, message string) {
	flash.SetFlash(ctx.Response(), messageType, message, config.GetConfig().IsProduction())
}
