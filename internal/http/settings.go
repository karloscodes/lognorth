package http

import (
	"context"
	"fmt"
	"net/smtp"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/karloscodes/cartridge"

	"lognorth/internal/agent"
	"lognorth/internal/config"
	"lognorth/internal/license"
	"lognorth/internal/settings"
	"lognorth/internal/users"
)

// renderSettingsPage loads the full settings + auxiliary props and
// renders one of the four sub-page Inertia components. Each sub-page
// only edits its own subset of fields but receives the full settings
// object so saves don't clobber the others.
func renderSettingsPage(ctx *cartridge.Context, component string) error {
	db := ctx.DB()
	cfg := config.GetConfig()

	s, err := settings.Get(db)
	if err != nil {
		return ctx.Ctx.Status(500).JSON(fiber.Map{"error": "failed to load settings"})
	}

	if s.BaseURL == "" {
		s.BaseURL = cfg.GetBaseURL()
	}

	var userEmail string
	if userID, ok := ctx.Session.GetUserID(ctx.Ctx); ok {
		if user, err := users.FindByID(db, userID); err == nil {
			userEmail = user.Email
		}
	}

	return RenderPage(ctx.Ctx, component, Props{
		"settings":             s,
		"hasPassword":          s.HasPassword(),
		"userEmail":            userEmail,
		"agentKeyPrefix":       s.AgentKeyPrefix,
		"openrouterConfigured": s.IsOpenRouterConfigured(),
		"openrouterLabel":      s.OpenRouterLabel,
		"openrouterModel":      s.OpenRouterModel,
		"openrouterModels":     agent.OpenRouterModels,
		"openrouterDefault":    agent.DefaultOpenRouterModel,
		"githubConfigured":     s.IsGitHubConfigured(),
		"githubLogin":          s.GitHubLogin,
	})
}

// SettingsLicensePageAction handles GET /settings/license
func SettingsLicensePageAction(ctx *cartridge.Context) error {
	return renderSettingsPage(ctx, "SettingsLicense")
}

// SettingsAlertsPageAction handles GET /settings/alerts
func SettingsAlertsPageAction(ctx *cartridge.Context) error {
	return renderSettingsPage(ctx, "SettingsAlerts")
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
			status, email, err := license.Validate(context.Background(), input.LicenseKey)
			if err != nil {
				return ctx.Ctx.Status(503).JSON(fiber.Map{
					"error": "Couldn't reach the license server. Try again in a moment.",
				})
			}
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

// ConnectOpenRouterInput is the body for POST /settings/openrouter.
type ConnectOpenRouterInput struct {
	Key   string `json:"openrouter_key"`
	Model string `json:"openrouter_model"`
}

// ConnectOpenRouterAction handles POST /settings/openrouter. It validates the
// key against OpenRouter, then encrypts it at rest. Global: one key funds the
// agent's fixes across every app.
func ConnectOpenRouterAction(ctx *cartridge.Context) error {
	db := ctx.DB()

	var input ConnectOpenRouterInput
	if err := ctx.Ctx.BodyParser(&input); err != nil {
		return ctx.Ctx.Status(400).JSON(fiber.Map{"error": "invalid input"})
	}
	key := strings.TrimSpace(input.Key)
	if key == "" {
		return ctx.Ctx.Status(400).JSON(fiber.Map{"error": "Key is required."})
	}

	label, err := agent.ValidateOpenRouterKey(context.Background(), key)
	if err != nil {
		return ctx.Ctx.Status(400).JSON(fiber.Map{"error": "OpenRouter rejected the key. Check it and try again."})
	}

	encrypted, err := agent.Encrypt(key, config.GetConfig().PrivateKey)
	if err != nil {
		return ctx.Ctx.Status(500).JSON(fiber.Map{"error": "failed to encrypt key"})
	}

	s, err := settings.Get(db)
	if err != nil {
		return ctx.Ctx.Status(500).JSON(fiber.Map{"error": "failed to load settings"})
	}
	s.OpenRouterKey = encrypted
	s.OpenRouterLabel = label
	if model := strings.TrimSpace(input.Model); agent.IsKnownModel(model) {
		s.OpenRouterModel = model
	} else if s.OpenRouterModel == "" {
		s.OpenRouterModel = agent.DefaultOpenRouterModel
	}
	if err := settings.Update(db, s); err != nil {
		return ctx.Ctx.Status(500).JSON(fiber.Map{"error": "failed to save settings"})
	}

	return ctx.Ctx.JSON(fiber.Map{"success": true, "openrouter_label": label, "openrouter_model": s.OpenRouterModel})
}

// SetOpenRouterModelInput is the body for POST /settings/openrouter/model.
type SetOpenRouterModelInput struct {
	Model string `json:"openrouter_model"`
}

// SetOpenRouterModelAction handles POST /settings/openrouter/model — change the
// model without re-entering the key.
func SetOpenRouterModelAction(ctx *cartridge.Context) error {
	db := ctx.DB()

	var input SetOpenRouterModelInput
	if err := ctx.Ctx.BodyParser(&input); err != nil {
		return ctx.Ctx.Status(400).JSON(fiber.Map{"error": "invalid input"})
	}
	model := strings.TrimSpace(input.Model)
	if !agent.IsKnownModel(model) {
		return ctx.Ctx.Status(400).JSON(fiber.Map{"error": "Unknown model."})
	}

	s, err := settings.Get(db)
	if err != nil {
		return ctx.Ctx.Status(500).JSON(fiber.Map{"error": "failed to load settings"})
	}
	s.OpenRouterModel = model
	if err := settings.Update(db, s); err != nil {
		return ctx.Ctx.Status(500).JSON(fiber.Map{"error": "failed to save settings"})
	}

	return ctx.Ctx.JSON(fiber.Map{"success": true, "openrouter_model": model})
}

// ConnectGitHubInput is the body for POST /settings/github.
type ConnectGitHubInput struct {
	Token string `json:"github_token"`
}

// ConnectGitHubAction handles POST /settings/github. It stores the global
// fine-grained PAT the agent uses to open PRs. Classic tokens are rejected;
// apps then pin themselves to repos this token can reach. Encrypted at rest.
func ConnectGitHubAction(ctx *cartridge.Context) error {
	db := ctx.DB()

	var input ConnectGitHubInput
	if err := ctx.Ctx.BodyParser(&input); err != nil {
		return ctx.Ctx.Status(400).JSON(fiber.Map{"error": "invalid input"})
	}
	token := strings.TrimSpace(input.Token)
	if token == "" {
		return ctx.Ctx.Status(400).JSON(fiber.Map{"error": "Token is required."})
	}
	if !agent.IsFineGrained(token) {
		return ctx.Ctx.Status(400).JSON(fiber.Map{
			"error": "That looks like a classic token. Classic tokens grant access to your whole account. Create a fine-grained token (github_pat_...) scoped to only the repositories you'll connect.",
		})
	}

	info, err := agent.ValidateToken(context.Background(), token)
	if err != nil {
		return ctx.Ctx.Status(400).JSON(fiber.Map{"error": "GitHub rejected the token. Check it hasn't expired and try again."})
	}

	encrypted, err := agent.Encrypt(token, config.GetConfig().PrivateKey)
	if err != nil {
		return ctx.Ctx.Status(500).JSON(fiber.Map{"error": "failed to encrypt token"})
	}

	s, err := settings.Get(db)
	if err != nil {
		return ctx.Ctx.Status(500).JSON(fiber.Map{"error": "failed to load settings"})
	}
	s.GitHubToken = encrypted
	s.GitHubLogin = info.Login
	s.GitHubTokenStatus = "valid"
	if err := settings.Update(db, s); err != nil {
		return ctx.Ctx.Status(500).JSON(fiber.Map{"error": "failed to save settings"})
	}

	return ctx.Ctx.JSON(fiber.Map{"success": true, "github_login": info.Login, "repo_count": len(info.Repos)})
}

// DisconnectGitHubAction handles POST /settings/github/disconnect.
func DisconnectGitHubAction(ctx *cartridge.Context) error {
	db := ctx.DB()

	s, err := settings.Get(db)
	if err != nil {
		return ctx.Ctx.Status(500).JSON(fiber.Map{"error": "failed to load settings"})
	}
	s.GitHubToken = ""
	s.GitHubLogin = ""
	s.GitHubTokenStatus = ""
	if err := settings.Update(db, s); err != nil {
		return ctx.Ctx.Status(500).JSON(fiber.Map{"error": "failed to save settings"})
	}

	return ctx.Ctx.JSON(fiber.Map{"success": true})
}

// DisconnectOpenRouterAction handles POST /settings/openrouter/disconnect.
func DisconnectOpenRouterAction(ctx *cartridge.Context) error {
	db := ctx.DB()

	s, err := settings.Get(db)
	if err != nil {
		return ctx.Ctx.Status(500).JSON(fiber.Map{"error": "failed to load settings"})
	}
	s.OpenRouterKey = ""
	s.OpenRouterLabel = ""
	if err := settings.Update(db, s); err != nil {
		return ctx.Ctx.Status(500).JSON(fiber.Map{"error": "failed to save settings"})
	}

	return ctx.Ctx.JSON(fiber.Map{"success": true})
}

// RevokeAgentKeyAction handles POST /settings/agent-key/revoke
func RevokeAgentKeyAction(ctx *cartridge.Context) error {
	db := ctx.DB()

	if err := settings.RevokeAgentKey(db); err != nil {
		return ctx.Ctx.Status(500).JSON(fiber.Map{"error": "Failed to revoke agent key"})
	}

	return ctx.Ctx.JSON(fiber.Map{"success": true})
}
