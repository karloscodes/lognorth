package http

import (
	"strconv"

	"github.com/gofiber/fiber/v2"
	"github.com/karloscodes/cartridge"
	"github.com/karloscodes/cartridge/flash"

	"lognorth/internal/apikeys"
	"lognorth/internal/events"
)

// AppForSetup represents an app with activity stats for the setup page
type AppForSetup struct {
	ID                      uint   `json:"id"`
	Name                    string `json:"name"`
	Key                     string `json:"key"`
	KeyPrefix               string `json:"key_prefix"`
	Active                  bool   `json:"active"`
	TodayTotal              int64  `json:"today_total"`
	LastSeen                *int64 `json:"last_seen"` // Unix timestamp, nil if never
	SilenceThresholdMinutes int    `json:"silence_threshold_minutes"`
	IsSilent                bool   `json:"is_silent"`
}

// SetupPageAction handles GET /apps (Inertia page)
func SetupPageAction(ctx *cartridge.Context) error {
	db := ctx.DB()

	keys, err := apikeys.List(db)
	if err != nil {
		keys = []apikeys.APIKey{}
	}

	// Enrich with stats
	apps := make([]AppForSetup, 0, len(keys))
	for _, k := range keys {
		app := AppForSetup{
			ID:                      k.ID,
			Name:                    k.Name,
			Key:                     k.Key,
			KeyPrefix:               k.KeyPrefix,
			Active:                  k.Active,
			SilenceThresholdMinutes: k.SilenceThresholdMinutes,
			IsSilent:                k.IsSilent,
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

	_, fullKey, err := apikeys.Create(db, name)
	if err != nil {
		flash.SetFlash(ctx.Ctx, "error", "Failed to create API key")
		return ctx.Redirect("/apps", fiber.StatusFound)
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

// SetSilenceThresholdAction handles POST /apps/api-keys/:id/silence-threshold
func SetSilenceThresholdAction(ctx *cartridge.Context) error {
	db := ctx.DB()

	id, err := ctx.Ctx.ParamsInt("id")
	if err != nil || id <= 0 {
		flash.SetFlash(ctx.Ctx, "error", "Invalid app")
		return ctx.Redirect("/apps", fiber.StatusFound)
	}

	minutes, _ := strconv.Atoi(ctx.FormValue("minutes"))
	if minutes < 0 {
		minutes = 0
	}
	if minutes > 1440 { // cap at 24 hours
		minutes = 1440
	}

	if err := apikeys.SetSilenceThreshold(db, uint(id), minutes); err != nil {
		flash.SetFlash(ctx.Ctx, "error", "Couldn't update")
		return ctx.Redirect("/apps", fiber.StatusFound)
	}

	if minutes == 0 {
		flash.SetFlash(ctx.Ctx, "success", "Uptime check disabled")
	} else {
		flash.SetFlash(ctx.Ctx, "success", "Uptime check updated")
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

// ListAPIKeysAction handles GET /api/v1/api-keys (JSON API)
func ListAPIKeysAction(ctx *cartridge.Context) error {
	db := ctx.DB()

	keys, err := apikeys.List(db)
	if err != nil {
		return ctx.Ctx.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to fetch API keys",
		})
	}

	return ctx.Ctx.JSON(fiber.Map{
		"apiKeys": keys,
	})
}
