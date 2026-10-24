package http

import (
	"github.com/karloscodes/cartridge"

	"lognorth/internal/alerts"
	"lognorth/internal/config"
	"lognorth/internal/settings"
	"lognorth/internal/users"
)

// renderSettingsPage loads the full settings + auxiliary props and
// renders one of the sub-page Inertia components. Each sub-page
// only edits its own subset of fields but receives the full settings
// object so saves don't clobber the others.
func renderSettingsPage(ctx *cartridge.Context, component string) error {
	db := ctx.DB()
	cfg := config.GetConfig()

	s, err := settings.Get(db)
	if err != nil {
		return ctx.Status(500).JSON(cartridge.Map{"error": "failed to load settings"})
	}

	if s.BaseURL == "" {
		s.BaseURL = cfg.GetBaseURL()
	}

	var userEmail string
	if userID, ok := ctx.Session.GetUserID(ctx); ok {
		if user, err := users.FindByID(db, userID); err == nil {
			userEmail = user.Email
		}
	}

	return RenderPage(ctx, component, Props{
		"settings":         s,
		"hasPassword":      s.HasPassword(),
		"hasTelegramToken": s.HasTelegramToken(),
		"hasWebhookToken":  s.HasWebhookToken(),
		"userEmail":        userEmail,
		"agentKeyPrefix":   s.AgentKeyPrefix,
	})
}

// SettingsAlertsPageAction handles GET /settings/alerts
func SettingsAlertsPageAction(ctx *cartridge.Context) error {
	return renderSettingsPage(ctx, "SettingsAlerts")
}

// SettingsAppearancePageAction handles GET /settings/appearance. The theme
// lives in the browser, so the page needs no settings.
func SettingsAppearancePageAction(ctx *cartridge.Context) error {
	return RenderPage(ctx, "SettingsAppearance", Props{})
}

// SettingsDataPageAction handles GET /settings/data
func SettingsDataPageAction(ctx *cartridge.Context) error {
	return renderSettingsPage(ctx, "SettingsData")
}

// SettingsDeveloperPageAction handles GET /settings/developer
func SettingsDeveloperPageAction(ctx *cartridge.Context) error {
	return renderSettingsPage(ctx, "SettingsDeveloper")
}

// UpdateSettingsInput represents the settings update request
type UpdateSettingsInput struct {
	// Channel: "ntfy", "email", "telegram", or "webhook"
	NotifyChannel string `json:"notify_channel"`

	// ntfy settings
	NtfyURL string `json:"ntfy_url"`

	// Email/SMTP settings
	SMTPHost     string `json:"smtp_host"`
	SMTPPort     int    `json:"smtp_port"`
	SMTPUsername string `json:"smtp_username"`
	SMTPPassword string `json:"smtp_password"`
	SMTPFrom     string `json:"smtp_from"`
	NotifyEmail  string `json:"notify_email"`

	// Telegram settings
	TelegramBotToken string `json:"telegram_bot_token"`
	TelegramChatID   string `json:"telegram_chat_id"`

	// Webhook settings
	WebhookURL   string `json:"webhook_url"`
	WebhookToken string `json:"webhook_token"`

	// Quiet hours
	QuietHoursEnabled bool `json:"quiet_hours_enabled"`
	QuietStart        int  `json:"quiet_start"`
	QuietEnd          int  `json:"quiet_end"`

	// General
	BaseURL string `json:"base_url"`

	// Data retention
	RetentionDays int `json:"retention_days"`
}

// UpdateSettingsAction handles POST /settings
func UpdateSettingsAction(ctx *cartridge.Context) error {
	db := ctx.DB()

	var input UpdateSettingsInput
	if err := ctx.BodyParser(&input); err != nil {
		return ctx.Status(400).JSON(cartridge.Map{"error": "invalid input"})
	}

	s, err := settings.Get(db)
	if err != nil {
		return ctx.Status(500).JSON(cartridge.Map{"error": "failed to load settings"})
	}

	// Update channel
	s.NotifyChannel = input.NotifyChannel

	// Update ntfy
	s.NtfyURL = input.NtfyURL

	// Update SMTP
	s.SMTPHost = input.SMTPHost
	s.SMTPPort = input.SMTPPort
	s.SMTPUsername = input.SMTPUsername
	s.SMTPFrom = input.SMTPFrom
	s.NotifyEmail = input.NotifyEmail
	s.BaseURL = input.BaseURL

	// Update Telegram
	s.TelegramChatID = input.TelegramChatID
	// Blank keeps the saved token: the browser never receives it.
	if input.TelegramBotToken != "" {
		s.TelegramBotToken = input.TelegramBotToken
	}

	// Update webhook. Blank keeps the saved token, like the Telegram one.
	s.WebhookURL = input.WebhookURL
	if input.WebhookToken != "" {
		s.WebhookToken = input.WebhookToken
	}

	// Update quiet hours
	s.QuietHoursEnabled = input.QuietHoursEnabled
	s.QuietStart = input.QuietStart
	s.QuietEnd = input.QuietEnd

	// Only update password if provided
	if input.SMTPPassword != "" {
		s.SMTPPassword = input.SMTPPassword
	}

	// Update retention days (minimum 7 days, max 365 days)
	if input.RetentionDays > 0 {
		if input.RetentionDays < 7 {
			input.RetentionDays = 7
		}
		if input.RetentionDays > 365 {
			input.RetentionDays = 365
		}
		s.RetentionDays = input.RetentionDays
	}

	if err := settings.Update(db, s); err != nil {
		return ctx.Status(500).JSON(cartridge.Map{"error": "failed to save settings"})
	}

	return ctx.JSON(cartridge.Map{"success": true})
}

// TestNotificationAction handles POST /settings/test
func TestNotificationAction(ctx *cartridge.Context) error {
	db := ctx.DB()

	s, err := settings.Get(db)
	if err != nil {
		return ctx.Status(500).JSON(cartridge.Map{"error": "failed to load settings"})
	}

	if !alerts.CanNotify(s) {
		return ctx.Status(400).JSON(cartridge.Map{"error": "notifications not configured"})
	}

	err = alerts.Send(s, "LogNorth Test", "This is a test notification. If you received this, notifications are working.")
	if err != nil {
		return ctx.Status(400).JSON(cartridge.Map{"error": err.Error()})
	}

	return ctx.JSON(cartridge.Map{"success": true, "message": "Test notification sent"})
}

// CreateAgentKeyAction handles POST /settings/agent-key
func CreateAgentKeyAction(ctx *cartridge.Context) error {
	db := ctx.DB()

	fullKey, err := settings.CreateAgentKey(db)
	if err != nil {
		return ctx.Status(400).JSON(cartridge.Map{"error": "Agent key already exists. Use regenerate to create a new one."})
	}

	s, _ := settings.Get(db)
	return ctx.JSON(cartridge.Map{"success": true, "key": fullKey, "prefix": s.AgentKeyPrefix})
}

// RegenerateAgentKeyAction handles POST /settings/agent-key/regenerate
func RegenerateAgentKeyAction(ctx *cartridge.Context) error {
	db := ctx.DB()

	fullKey, err := settings.RegenerateAgentKey(db)
	if err != nil {
		return ctx.Status(500).JSON(cartridge.Map{"error": "Failed to regenerate agent key"})
	}

	s, _ := settings.Get(db)
	return ctx.JSON(cartridge.Map{"success": true, "key": fullKey, "prefix": s.AgentKeyPrefix})
}

// RevokeAgentKeyAction handles POST /settings/agent-key/revoke
func RevokeAgentKeyAction(ctx *cartridge.Context) error {
	db := ctx.DB()

	if err := settings.RevokeAgentKey(db); err != nil {
		return ctx.Status(500).JSON(cartridge.Map{"error": "Failed to revoke agent key"})
	}

	return ctx.JSON(cartridge.Map{"success": true})
}
