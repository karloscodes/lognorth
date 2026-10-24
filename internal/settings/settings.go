// Package settings provides application settings management
package settings

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"time"

	"gorm.io/gorm"
)

// Settings stores application configuration
type Settings struct {
	ID uint `gorm:"primaryKey" json:"-"`

	// Notification channel: "email", "ntfy", "telegram", or "webhook"
	NotifyChannel string `gorm:"size:20" json:"notify_channel"`

	// ntfy.sh URL (e.g., https://ntfy.sh/my-secret-topic)
	NtfyURL string `gorm:"size:500" json:"ntfy_url"`

	// Email/SMTP settings
	SMTPHost     string `gorm:"size:255" json:"smtp_host"`
	SMTPPort     int    `json:"smtp_port"`
	SMTPUsername string `gorm:"size:255" json:"smtp_username"`
	SMTPPassword string `gorm:"size:255" json:"-"` // Never expose in JSON
	SMTPFrom     string `gorm:"size:255" json:"smtp_from"`
	NotifyEmail  string `gorm:"size:255" json:"notify_email"`

	// Telegram settings
	TelegramBotToken string `gorm:"size:100" json:"-"` // Never expose in JSON
	TelegramChatID   string `gorm:"size:50" json:"telegram_chat_id"`

	// Webhook settings: a JSON POST per alert, for Grok Bot routines, Slack,
	// or your own endpoint. The token goes in the Authorization header.
	WebhookURL   string `gorm:"size:500" json:"webhook_url"`
	WebhookToken string `gorm:"size:500" json:"-"` // Never expose in JSON

	// Quiet hours - no alerts during this window
	QuietHoursEnabled bool `gorm:"default:false" json:"quiet_hours_enabled"`
	QuietStart        int  `gorm:"default:23" json:"quiet_start"` // Hour (0-23), default 11pm
	QuietEnd          int  `gorm:"default:7" json:"quiet_end"`    // Hour (0-23), default 7am

	// Base URL for links in notifications (e.g., https://lognorth.example.com)
	BaseURL string `gorm:"size:255" json:"base_url"`

	// Agent API key (read-only access for AI coding assistants)
	AgentKey       string `gorm:"size:100" json:"-"`               // Full key, never exposed
	AgentKeyPrefix string `gorm:"size:20" json:"agent_key_prefix"` // For display (lgn-agent-xxxxxxxx)

	// Data retention - how many days to keep events (default 90 = 3 months)
	RetentionDays int `gorm:"default:90" json:"retention_days"`
}

// TableName specifies the table name
func (Settings) TableName() string {
	return "settings"
}

// Get retrieves the settings (creates default if none exist)
func Get(db *gorm.DB) (*Settings, error) {
	var settings Settings
	result := db.First(&settings)
	if result.Error == gorm.ErrRecordNotFound {
		settings = Settings{
			SMTPPort:      587,
			RetentionDays: 90, // 3 months default
		}
		if err := db.Create(&settings).Error; err != nil {
			return nil, err
		}
		return &settings, nil
	}
	if result.Error != nil {
		return nil, result.Error
	}
	return &settings, nil
}

// Update saves the settings
func Update(db *gorm.DB, s *Settings) error {
	s.ID = 1
	return db.Save(s).Error
}

// HasPassword returns true if SMTP password is configured
func (s *Settings) HasPassword() bool {
	return s.SMTPPassword != ""
}

// IsEmailConfigured returns true if SMTP is fully configured
func (s *Settings) IsEmailConfigured() bool {
	return s.SMTPHost != "" && s.SMTPPort > 0 && s.SMTPFrom != "" && s.NotifyEmail != ""
}

// IsNtfyConfigured returns true if ntfy URL is set
func (s *Settings) IsNtfyConfigured() bool {
	return s.NtfyURL != ""
}

// HasTelegramToken reports whether a bot token is saved, without exposing it.
func (s *Settings) HasTelegramToken() bool {
	return s.TelegramBotToken != ""
}

// IsTelegramConfigured returns true if Telegram bot is configured
func (s *Settings) IsTelegramConfigured() bool {
	return s.TelegramBotToken != "" && s.TelegramChatID != ""
}

// HasWebhookToken reports whether a webhook token is saved, without exposing it.
func (s *Settings) HasWebhookToken() bool {
	return s.WebhookToken != ""
}

// IsWebhookConfigured returns true if a webhook URL is set
func (s *Settings) IsWebhookConfigured() bool {
	return s.WebhookURL != ""
}

// GenerateAgentKey creates a new agent API key
func GenerateAgentKey() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return "lgn-agent-" + hex.EncodeToString(bytes), nil
}

// CreateAgentKey generates and saves a new agent key
func CreateAgentKey(db *gorm.DB) (string, error) {
	s, err := Get(db)
	if err != nil {
		return "", err
	}

	if s.AgentKey != "" {
		return "", fmt.Errorf("agent key already exists")
	}

	key, err := GenerateAgentKey()
	if err != nil {
		return "", err
	}

	s.AgentKey = key
	s.AgentKeyPrefix = key[:18] // "lgn-agent-" + 8 hex chars

	if err := Update(db, s); err != nil {
		return "", err
	}

	return key, nil
}

// RegenerateAgentKey creates a new agent key, replacing any existing one
func RegenerateAgentKey(db *gorm.DB) (string, error) {
	s, err := Get(db)
	if err != nil {
		return "", err
	}

	key, err := GenerateAgentKey()
	if err != nil {
		return "", err
	}

	s.AgentKey = key
	s.AgentKeyPrefix = key[:18]

	if err := Update(db, s); err != nil {
		return "", err
	}

	return key, nil
}

// RevokeAgentKey removes the agent key
func RevokeAgentKey(db *gorm.DB) error {
	s, err := Get(db)
	if err != nil {
		return err
	}

	s.AgentKey = ""
	s.AgentKeyPrefix = ""

	return Update(db, s)
}

// ValidateAgentKey checks if a key matches the stored agent key
func ValidateAgentKey(db *gorm.DB, key string) error {
	s, err := Get(db)
	if err != nil {
		return err
	}

	if s.AgentKey == "" {
		return fmt.Errorf("no agent key configured")
	}

	if subtle.ConstantTimeCompare([]byte(s.AgentKey), []byte(key)) != 1 {
		return fmt.Errorf("invalid agent key")
	}

	return nil
}

// GetRetentionDuration returns the data retention period as time.Duration
// Returns 0 if retention is disabled (keep forever)
func (s *Settings) GetRetentionDuration() time.Duration {
	if s.RetentionDays == 0 {
		return 0 // forever - no deletion
	}
	days := s.RetentionDays
	if days < 0 {
		days = 90 // default to 3 months
	}
	return time.Duration(days) * 24 * time.Hour
}
