package http

import (
	"fmt"
	"net/smtp"

	"github.com/gofiber/fiber/v2"
	"github.com/karloscodes/cartridge"

	"lognorth/internal/settings"
	"lognorth/internal/users"
)

// SettingsPageAction handles GET /settings
func SettingsPageAction(ctx *cartridge.Context) error {
	db := ctx.DB()

	s, err := settings.Get(db)
	if err != nil {
		return ctx.Ctx.Status(500).JSON(fiber.Map{"error": "failed to load settings"})
	}

	section := ctx.Ctx.Query("section", "notifications")

	// Get current user's email for default notification recipient
	var userEmail string
	if userID, ok := ctx.Session.GetUserID(ctx.Ctx); ok {
		if user, err := users.FindByID(db, userID); err == nil {
			userEmail = user.Email
		}
	}

	return RenderPage(ctx.Ctx, "Settings", Props{
		"settings":    s,
		"hasPassword": s.HasPassword(),
		"section":     section,
		"userEmail":   userEmail,
	})
}

// UpdateSettingsInput represents the settings update request
type UpdateSettingsInput struct {
	SMTPHost     string `json:"smtp_host"`
	SMTPPort     int    `json:"smtp_port"`
	SMTPUsername string `json:"smtp_username"`
	SMTPPassword string `json:"smtp_password"`
	SMTPFrom     string `json:"smtp_from"`

	NotifyNewErrors   bool   `json:"notify_new_errors"`
	NotifyRegressions bool   `json:"notify_regressions"`
	NotifySpikes      bool   `json:"notify_spikes"`
	SpikeThreshold    int    `json:"spike_threshold"`
	NotifyEmail       string `json:"notify_email"`
	BaseURL           string `json:"base_url"`
}

// UpdateSettingsAction handles POST /settings
func UpdateSettingsAction(ctx *cartridge.Context) error {
	db := ctx.DB()

	var input UpdateSettingsInput
	if err := ctx.Ctx.BodyParser(&input); err != nil {
		return ctx.Ctx.Status(400).JSON(fiber.Map{"error": "invalid input"})
	}

	// Get existing settings
	s, err := settings.Get(db)
	if err != nil {
		return ctx.Ctx.Status(500).JSON(fiber.Map{"error": "failed to load settings"})
	}

	// Update fields
	s.SMTPHost = input.SMTPHost
	s.SMTPPort = input.SMTPPort
	s.SMTPUsername = input.SMTPUsername
	s.SMTPFrom = input.SMTPFrom

	// Only update password if provided (non-empty)
	if input.SMTPPassword != "" {
		s.SMTPPassword = input.SMTPPassword
	}

	// Update notification settings
	s.NotifyNewErrors = input.NotifyNewErrors
	s.NotifyRegressions = input.NotifyRegressions
	s.NotifySpikes = input.NotifySpikes
	if input.SpikeThreshold > 0 {
		s.SpikeThreshold = input.SpikeThreshold
	} else {
		s.SpikeThreshold = 10 // Default 10%
	}
	s.NotifyEmail = input.NotifyEmail
	s.BaseURL = input.BaseURL

	if err := settings.Update(db, s); err != nil {
		return ctx.Ctx.Status(500).JSON(fiber.Map{"error": "failed to save settings"})
	}

	return ctx.Ctx.JSON(fiber.Map{"success": true})
}

// TestSMTPAction handles POST /settings/test-smtp
func TestSMTPAction(ctx *cartridge.Context) error {
	db := ctx.DB()

	s, err := settings.Get(db)
	if err != nil {
		return ctx.Ctx.Status(500).JSON(fiber.Map{"error": "failed to load settings"})
	}

	if !s.IsConfigured() {
		return ctx.Ctx.Status(400).JSON(fiber.Map{"error": "SMTP not configured"})
	}

	// Try to connect to SMTP server
	addr := fmt.Sprintf("%s:%d", s.SMTPHost, s.SMTPPort)

	var auth smtp.Auth
	if s.SMTPUsername != "" && s.SMTPPassword != "" {
		auth = smtp.PlainAuth("", s.SMTPUsername, s.SMTPPassword, s.SMTPHost)
	}

	// Send test email to the from address
	to := []string{s.SMTPFrom}
	msg := []byte("Subject: LogNorth Test Email\r\n\r\nThis is a test email from LogNorth. If you received this, SMTP is configured correctly.")

	err = smtp.SendMail(addr, auth, s.SMTPFrom, to, msg)
	if err != nil {
		return ctx.Ctx.Status(400).JSON(fiber.Map{"error": err.Error()})
	}

	return ctx.Ctx.JSON(fiber.Map{"success": true, "message": "Test email sent to " + s.SMTPFrom})
}
