// Package settings provides application settings management
package settings

import (
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
	TelegramBotToken string `gorm:"size:100" json:"telegram_bot_token"`
	TelegramChatID   string `gorm:"size:50" json:"telegram_chat_id"`

	// Quiet hours - no alerts during this window
	QuietHoursEnabled bool `gorm:"default:false" json:"quiet_hours_enabled"`
	QuietStart        int  `gorm:"default:23" json:"quiet_start"` // Hour (0-23), default 11pm
	QuietEnd          int  `gorm:"default:7" json:"quiet_end"`    // Hour (0-23), default 7am

	// Base URL for links in notifications (e.g., https://lognorth.example.com)
	BaseURL string `gorm:"size:255" json:"base_url"`

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
			SMTPPort: 587,
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

	// Format message with title
	text := fmt.Sprintf("*%s*\n\n%s", title, message)

	body := fmt.Sprintf(`{"chat_id":"%s","text":%q,"parse_mode":"Markdown"}`,
		s.TelegramChatID, text)

	req, err := http.NewRequest("POST", url, strings.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("telegram request failed: %w", err)
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
