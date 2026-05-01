package http

import (
	"context"
	"fmt"
	"net/smtp"

	"github.com/gofiber/fiber/v2"
	"github.com/karloscodes/cartridge"

	"lognorth/internal/config"
	"lognorth/internal/license"
	"lognorth/internal/settings"
	"lognorth/internal/users"
)

// SettingsPageAction handles GET /settings
func SettingsPageAction(ctx *cartridge.Context) error {
	db := ctx.DB()
	cfg := config.GetConfig()

	s, err := settings.Get(db)
	if err != nil {
		return ctx.Ctx.Status(500).JSON(fiber.Map{"error": "failed to load settings"})
	}

	// Auto-populate base URL from config if not set
	if s.BaseURL == "" {
		s.BaseURL = cfg.GetBaseURL()
	}

	// Get current user's email for default notification recipient
	var userEmail string
	if userID, ok := ctx.Session.GetUserID(ctx.Ctx); ok {
		if user, err := users.FindByID(db, userID); err == nil {
			userEmail = user.Email
		}
	}

	return RenderPage(ctx.Ctx, "Settings", Props{
		"settings":       s,
		"hasPassword":    s.HasPassword(),
		"userEmail":      userEmail,
		"agentKeyPrefix": s.AgentKeyPrefix,
		"hasLicense":     s.LicenseKey != "",
	})
}

// UpdateSettingsInput represents the settings update request
type UpdateSettingsInput struct {
	// Channel: "ntfy", "email", or "telegram"
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

	// Quiet hours
	QuietHoursEnabled bool `json:"quiet_hours_enabled"`
	QuietStart        int  `json:"quiet_start"`
	QuietEnd          int  `json:"quiet_end"`

	// General
	BaseURL string `json:"base_url"`

	// Data retention
	RetentionDays int `json:"retention_days"`

	// License key
	LicenseKey string `json:"license_key"`
}

// UpdateSettingsAction handles POST /settings
func UpdateSettingsAction(ctx *cartridge.Context) error {
	db := ctx.DB()

	var input UpdateSettingsInput
	if err := ctx.Ctx.BodyParser(&input); err != nil {
		return ctx.Ctx.Status(400).JSON(fiber.Map{"error": "invalid input"})
	}

	s, err := settings.Get(db)
	if err != nil {
		return ctx.Ctx.Status(500).JSON(fiber.Map{"error": "failed to load settings"})
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
	s.TelegramBotToken = input.TelegramBotToken
	s.TelegramChatID = input.TelegramChatID

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

	// Validate and update license key
	if input.LicenseKey != s.LicenseKey {
		s.LicenseKey = input.LicenseKey
		if input.LicenseKey == "" {
			s.LicenseStatus = ""
			s.LicenseEmail = ""
		} else {
			status, email := license.Validate(context.Background(), input.LicenseKey)
			s.LicenseStatus = status
			s.LicenseEmail = email
		}
	}

	if err := settings.Update(db, s); err != nil {
		return ctx.Ctx.Status(500).JSON(fiber.Map{"error": "failed to save settings"})
	}

	// Update cached license status
	license.SetConfigured(s.LicenseStatus == "valid")

	return ctx.Ctx.JSON(fiber.Map{
		"success":        true,
		"license_status": s.LicenseStatus,
		"license_email":  s.LicenseEmail,
	})
}

// TestNotificationAction handles POST /settings/test
func TestNotificationAction(ctx *cartridge.Context) error {
	db := ctx.DB()

	s, err := settings.Get(db)
	if err != nil {
		return ctx.Ctx.Status(500).JSON(fiber.Map{"error": "failed to load settings"})
	}

	if !s.CanNotify() {
		return ctx.Ctx.Status(400).JSON(fiber.Map{"error": "notifications not configured"})
	}

	err = s.SendNotification("LogNorth Test", "This is a test notification. If you received this, notifications are working.")
	if err != nil {
		return ctx.Ctx.Status(400).JSON(fiber.Map{"error": err.Error()})
	}

	return ctx.Ctx.JSON(fiber.Map{"success": true, "message": "Test notification sent"})
}

// TestSMTPAction handles POST /settings/test-smtp (legacy, redirects to TestNotificationAction)
func TestSMTPAction(ctx *cartridge.Context) error {
	db := ctx.DB()

	s, err := settings.Get(db)
	if err != nil {
		return ctx.Ctx.Status(500).JSON(fiber.Map{"error": "failed to load settings"})
	}

	if !s.IsEmailConfigured() {
		return ctx.Ctx.Status(400).JSON(fiber.Map{"error": "SMTP not configured"})
	}

	addr := fmt.Sprintf("%s:%d", s.SMTPHost, s.SMTPPort)

	var auth smtp.Auth
	if s.SMTPUsername != "" && s.SMTPPassword != "" {
		auth = smtp.PlainAuth("", s.SMTPUsername, s.SMTPPassword, s.SMTPHost)
	}

	to := []string{s.SMTPFrom}
	msg := []byte("Subject: LogNorth Test Email\r\n\r\nThis is a test email from LogNorth. If you received this, SMTP is configured correctly.")

	err = smtp.SendMail(addr, auth, s.SMTPFrom, to, msg)
	if err != nil {
		return ctx.Ctx.Status(400).JSON(fiber.Map{"error": err.Error()})
	}

	return ctx.Ctx.JSON(fiber.Map{"success": true, "message": "Test email sent to " + s.SMTPFrom})
}

// CreateAgentKeyAction handles POST /settings/agent-key
func CreateAgentKeyAction(ctx *cartridge.Context) error {
	db := ctx.DB()

	fullKey, err := settings.CreateAgentKey(db)
	if err != nil {
		return ctx.Ctx.Status(400).JSON(fiber.Map{"error": "Agent key already exists. Use regenerate to create a new one."})
	}

	s, _ := settings.Get(db)
	return ctx.Ctx.JSON(fiber.Map{"success": true, "key": fullKey, "prefix": s.AgentKeyPrefix})
}

// RegenerateAgentKeyAction handles POST /settings/agent-key/regenerate
func RegenerateAgentKeyAction(ctx *cartridge.Context) error {
	db := ctx.DB()

	fullKey, err := settings.RegenerateAgentKey(db)
	if err != nil {
		return ctx.Ctx.Status(500).JSON(fiber.Map{"error": "Failed to regenerate agent key"})
	}

	s, _ := settings.Get(db)
	return ctx.Ctx.JSON(fiber.Map{"success": true, "key": fullKey, "prefix": s.AgentKeyPrefix})
}

// RevokeAgentKeyAction handles POST /settings/agent-key/revoke
func RevokeAgentKeyAction(ctx *cartridge.Context) error {
	db := ctx.DB()

	if err := settings.RevokeAgentKey(db); err != nil {
		return ctx.Ctx.Status(500).JSON(fiber.Map{"error": "Failed to revoke agent key"})
	}

	return ctx.Ctx.JSON(fiber.Map{"success": true})
}

