package http

import (
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/karloscodes/cartridge"
	"github.com/karloscodes/cartridge/flash"

	"lognorth/internal/apikeys"
	"lognorth/internal/config"
	"lognorth/internal/events"
)

// AppForSetup represents an app with activity stats for the setup page
type AppForSetup struct {
	ID           uint   `json:"id"`
	Name         string `json:"name"`
	Key          string `json:"key"`
	KeyPrefix    string `json:"key_prefix"`
	Active       bool   `json:"active"`
	TodayTotal   int64  `json:"today_total"`
	LastSeen     *int64 `json:"last_seen"` // Unix timestamp, nil if never
	PingURL      string `json:"ping_url"`
	PingDisabled bool   `json:"ping_disabled"`
	IsDown       bool   `json:"is_down"`
}

// SetupPageAction handles GET /apps (Inertia page)
func SetupPageAction(ctx *cartridge.Context) error {
	db := ctx.DB()

	keys, err := apikeys.List(db)
	if err != nil {
		keys = []apikeys.APIKey{}
	}

	// The public demo shows the page but not the keys: anyone could copy one
	// and post events into the demo.
	isDemo := config.GetConfig().IsDemo()

	// Enrich with stats
	apps := make([]AppForSetup, 0, len(keys))
	for _, k := range keys {
		key := k.Key
		if isDemo {
			key = ""
		}
		app := AppForSetup{
			ID:           k.ID,
			Name:         k.Name,
			Key:          key,
			KeyPrefix:    k.KeyPrefix,
			Active:       k.Active,
			PingURL:      k.PingURL,
			PingDisabled: k.PingDisabled,
			IsDown:       k.IsDown,
		}
		if k.Active {
			stats := events.GetStats(db, k.ID)
			app.TodayTotal = stats.TodayTotal
			if stats.LastSeen != nil {
				ts := stats.LastSeen.Unix()
				app.LastSeen = &ts
			}
		}
		apps = append(apps, app)
	}

	return RenderPage(ctx.Ctx, "Setup", Props{
		"apiKeys": apps,
	})
}

// CreateAPIKeyAction handles POST /apps/api-keys (PRG pattern)
func CreateAPIKeyAction(ctx *cartridge.Context) error {
	db := ctx.DB()

	name := ctx.FormValue("name")
	if name == "" {
		name = "default"
	}

	apiKey, fullKey, err := apikeys.Create(db, name)
	if err != nil {
		flash.SetFlash(ctx.Ctx, "error", "Failed to create API key")
		return ctx.Redirect("/apps", fiber.StatusFound)
	}

	// Optional: set the ping URL provided at create time. Failure here
	// shouldn't roll back app creation — the user can fix it via [edit].
	if url := strings.TrimSpace(ctx.FormValue("url")); url != "" {
		_ = apikeys.SetPingURL(db, apiKey.ID, url)
	}

	// Store the full key in flash so user can copy it
	flash.SetFlash(ctx.Ctx, "newKey", fullKey)
	return ctx.Redirect("/apps", fiber.StatusFound)
}

// RevokeAPIKeyAction handles POST /apps/api-keys/:id/revoke
func RevokeAPIKeyAction(ctx *cartridge.Context) error {
	db := ctx.DB()

	id, err := ctx.Ctx.ParamsInt("id")
	if err != nil || id <= 0 {
		flash.SetFlash(ctx.Ctx, "error", "Invalid API key ID")
		return ctx.Redirect("/apps", fiber.StatusFound)
	}

	if err := apikeys.Revoke(db, uint(id)); err != nil {
		flash.SetFlash(ctx.Ctx, "error", "Failed to revoke API key")
		return ctx.Redirect("/apps", fiber.StatusFound)
	}

	flash.SetFlash(ctx.Ctx, "success", "API key revoked")
	return ctx.Redirect("/apps", fiber.StatusFound)
}

// RenameAPIKeyAction handles POST /apps/api-keys/:id/rename
func RenameAPIKeyAction(ctx *cartridge.Context) error {
	db := ctx.DB()

	id, err := ctx.Ctx.ParamsInt("id")
	if err != nil || id <= 0 {
		flash.SetFlash(ctx.Ctx, "error", "Invalid app")
		return ctx.Redirect("/apps", fiber.StatusFound)
	}

	if err := apikeys.Rename(db, uint(id), ctx.FormValue("name")); err != nil {
		flash.SetFlash(ctx.Ctx, "error", err.Error())
		return ctx.Redirect("/apps", fiber.StatusFound)
	}

	flash.SetFlash(ctx.Ctx, "success", "Renamed")
	return ctx.Redirect("/apps", fiber.StatusFound)
}

// SetPingURLAction handles POST /apps/api-keys/:id/ping-url
func SetPingURLAction(ctx *cartridge.Context) error {
	db := ctx.DB()

	id, err := ctx.Ctx.ParamsInt("id")
	if err != nil || id <= 0 {
		flash.SetFlash(ctx.Ctx, "error", "Invalid app")
		return ctx.Redirect("/apps", fiber.StatusFound)
	}

	if err := apikeys.SetPingURL(db, uint(id), ctx.FormValue("url")); err != nil {
		flash.SetFlash(ctx.Ctx, "error", "Couldn't update URL")
		return ctx.Redirect("/apps", fiber.StatusFound)
	}

	flash.SetFlash(ctx.Ctx, "success", "Ping URL updated")
	return ctx.Redirect("/apps", fiber.StatusFound)
}

// TogglePingAction handles POST /apps/api-keys/:id/toggle-ping
func TogglePingAction(ctx *cartridge.Context) error {
	db := ctx.DB()

	id, err := ctx.Ctx.ParamsInt("id")
	if err != nil || id <= 0 {
		flash.SetFlash(ctx.Ctx, "error", "Invalid app")
		return ctx.Redirect("/apps", fiber.StatusFound)
	}

	disabled := ctx.FormValue("disabled") == "true"
	if err := apikeys.SetPingDisabled(db, uint(id), disabled); err != nil {
		flash.SetFlash(ctx.Ctx, "error", "Couldn't update")
		return ctx.Redirect("/apps", fiber.StatusFound)
	}

	if disabled {
		flash.SetFlash(ctx.Ctx, "success", "Uptime check disabled")
	} else {
		flash.SetFlash(ctx.Ctx, "success", "Uptime check enabled")
	}
	return ctx.Redirect("/apps", fiber.StatusFound)
}

// DeleteAPIKeyAction handles DELETE /apps/api-keys/:id (or POST with _method=DELETE)
func DeleteAPIKeyAction(ctx *cartridge.Context) error {
	db := ctx.DB()

	id, err := ctx.Ctx.ParamsInt("id")
	if err != nil || id <= 0 {
		flash.SetFlash(ctx.Ctx, "error", "Invalid API key ID")
		return ctx.Redirect("/apps", fiber.StatusFound)
	}

	if err := apikeys.Delete(db, uint(id)); err != nil {
		flash.SetFlash(ctx.Ctx, "error", "Failed to delete API key")
		return ctx.Redirect("/apps", fiber.StatusFound)
	}

	flash.SetFlash(ctx.Ctx, "success", "App deleted")
	return ctx.Redirect("/apps", fiber.StatusFound)
}
