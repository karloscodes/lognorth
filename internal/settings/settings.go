// Package settings provides application settings management
package settings

import (
	"bytes"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/smtp"
	"strings"
	"time"

	"gorm.io/gorm"
)

// Settings stores application configuration
type Settings struct {
	ID uint `gorm:"primaryKey" json:"-"`

	// Notification channel: "email", "ntfy", or "telegram"
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

	// License key from purchase
	LicenseKey    string `gorm:"size:255" json:"license_key"`
	LicenseStatus string `gorm:"size:20" json:"license_status"` // valid, invalid, expired, or empty
	LicenseEmail  string `gorm:"size:255" json:"license_email"`

	// Legacy fields - kept for migration compatibility
	NotifyNewErrors   bool   `json:"-"`
	NotifyRegressions bool   `json:"-"`
	NotifySpikes      bool   `json:"-"`
	SpikeThreshold    int    `json:"-"`
	DigestHour        int    `json:"-"`
	DigestTimezone    string `gorm:"size:50" json:"-"`
}

// TableName specifies the table name
func (Settings) TableName() string {
	return "settings"
}

// AutoMigrate creates the settings table
func AutoMigrate(db *gorm.DB) error {
	return db.AutoMigrate(&Settings{})
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

// IsQuietHours returns true if current time is within quiet hours
func (s *Settings) IsQuietHours() bool {
	if !s.QuietHoursEnabled {
		return false
	}
	hour := time.Now().Hour()
	if s.QuietStart > s.QuietEnd {
		// Crosses midnight (e.g., 23-7)
		return hour >= s.QuietStart || hour < s.QuietEnd
	}
	return hour >= s.QuietStart && hour < s.QuietEnd
}

// CanNotify returns true if any notification channel is configured and not in quiet hours
func (s *Settings) CanNotify() bool {
	if s.IsQuietHours() {
		return false
	}
	switch s.NotifyChannel {
	case "ntfy":
		return s.IsNtfyConfigured()
	case "email":
		return s.IsEmailConfigured()
	case "telegram":
		return s.IsTelegramConfigured()
	default:
		return false
	}
}

// SendNotification sends a notification via the configured channel
func (s *Settings) SendNotification(title, message string) error {
	switch s.NotifyChannel {
	case "ntfy":
		return s.sendViaNtfy(title, message)
	case "email":
		return s.sendViaSMTP(title, message)
	case "telegram":
		return s.sendViaTelegram(title, message)
	default:
		return fmt.Errorf("no notification channel configured")
	}
}

// sendViaNtfy sends notification via ntfy.sh
func (s *Settings) sendViaNtfy(title, message string) error {
	if s.NtfyURL == "" {
		return fmt.Errorf("ntfy URL not configured")
	}

	req, err := http.NewRequest("POST", s.NtfyURL, strings.NewReader(message))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Title", title)
	req.Header.Set("Priority", "high")
	req.Header.Set("Tags", "warning")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("ntfy request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("ntfy failed with status %d", resp.StatusCode)
	}

	return nil
}

// sendViaSMTP sends notification via email
func (s *Settings) sendViaSMTP(subject, body string) error {
	if !s.IsEmailConfigured() {
		return fmt.Errorf("SMTP not configured")
	}

	addr := fmt.Sprintf("%s:%d", s.SMTPHost, s.SMTPPort)

	var auth smtp.Auth
	if s.SMTPUsername != "" && s.SMTPPassword != "" {
		auth = smtp.PlainAuth("", s.SMTPUsername, s.SMTPPassword, s.SMTPHost)
	}

	to := []string{s.NotifyEmail}
	msg := fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: %s\r\n\r\n%s",
		s.SMTPFrom, s.NotifyEmail, subject, body)

	return smtp.SendMail(addr, auth, s.SMTPFrom, to, []byte(msg))
}

// sendViaTelegram sends notification via Telegram bot
func (s *Settings) sendViaTelegram(title, message string) error {
	if !s.IsTelegramConfigured() {
		return fmt.Errorf("Telegram not configured")
	}

	url := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", s.TelegramBotToken)

	// Plain text: Markdown mode rejects messages with an unbalanced _ or `,
	// which paths and error messages often contain.
	body, err := json.Marshal(map[string]string{
		"chat_id": s.TelegramChatID,
		"text":    title + "\n\n" + message,
	})
	if err != nil {
		return fmt.Errorf("failed to encode message: %w", err)
	}

	req, err := http.NewRequest("POST", url, bytes.NewReader(body))
	if err != nil {
		return errors.New("failed to create telegram request")
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		// Unwrap drops the request URL, which holds the bot token.
		return fmt.Errorf("telegram request failed: %w", errors.Unwrap(err))
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("telegram failed with status %d", resp.StatusCode)
	}

	return nil
}

// SendEmail is a compatibility wrapper for SendNotification
func (s *Settings) SendEmail(subject, body string) error {
	return s.SendNotification(subject, body)
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

// HasAgentKey returns true if an agent key is configured
func (s *Settings) HasAgentKey() bool {
	return s.AgentKey != ""
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
