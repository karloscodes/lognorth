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

	// Notification channel: "email" or "ntfy"
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

	// Base URL for links in notifications (e.g., https://lognorth.example.com)
	BaseURL string `gorm:"size:255" json:"base_url"`

	// Legacy fields - kept for migration compatibility
	NotifyNewErrors   bool `json:"-"`
	NotifyRegressions bool `json:"-"`
	NotifySpikes      bool `json:"-"`
	SpikeThreshold    int  `json:"-"`
	DigestHour        int  `json:"-"`
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

// CanNotify returns true if any notification channel is configured
func (s *Settings) CanNotify() bool {
	switch s.NotifyChannel {
	case "ntfy":
		return s.IsNtfyConfigured()
	case "email":
		return s.IsEmailConfigured()
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

// SendEmail is a compatibility wrapper for SendNotification
func (s *Settings) SendEmail(subject, body string) error {
	return s.SendNotification(subject, body)
}
