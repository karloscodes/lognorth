// Package settings provides application settings management
package settings

import (
	"fmt"
	"net/smtp"

	"gorm.io/gorm"
)

// Settings stores application configuration
type Settings struct {
	ID           uint   `gorm:"primaryKey" json:"-"`
	SMTPHost     string `gorm:"size:255" json:"smtp_host"`
	SMTPPort     int    `json:"smtp_port"`
	SMTPUsername string `gorm:"size:255" json:"smtp_username"`
	SMTPPassword string `gorm:"size:255" json:"-"` // Never expose in JSON
	SMTPFrom     string `gorm:"size:255" json:"smtp_from"`

	// Notification preferences
	NotifyNewErrors   bool   `json:"notify_new_errors"`
	NotifyRegressions bool   `json:"notify_regressions"`  // Alert when error returns after 24h+ quiet
	NotifySpikes      bool   `json:"notify_spikes"`       // Alert when endpoint error rate jumps
	SpikeThreshold    int    `json:"spike_threshold"`     // Percentage threshold (default 10)
	NotifyEmail       string `gorm:"size:255" json:"notify_email"` // Where to send alerts

	// Base URL for links in emails (e.g., https://lognorth.example.com)
	BaseURL string `gorm:"size:255" json:"base_url"`

	// Digest settings
	DigestHour     int    `gorm:"default:9" json:"digest_hour"`        // Hour to send digest (0-23)
	DigestTimezone string `gorm:"size:50;default:'UTC'" json:"digest_timezone"` // IANA timezone
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
		// Create default settings
		settings = Settings{
			SMTPPort:       587,   // Default to TLS port
			SpikeThreshold: 10,    // Default 10% error rate threshold
			DigestHour:     9,     // Default 9am
			DigestTimezone: "UTC", // Default UTC
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
	// Always update ID 1 (singleton)
	s.ID = 1
	return db.Save(s).Error
}

// HasPassword returns true if a password is configured
func (s *Settings) HasPassword() bool {
	return s.SMTPPassword != ""
}

// IsConfigured returns true if SMTP is fully configured
func (s *Settings) IsConfigured() bool {
	return s.SMTPHost != "" && s.SMTPPort > 0 && s.SMTPFrom != ""
}

// CanNotify returns true if notifications are enabled and configured
func (s *Settings) CanNotify() bool {
	return s.IsConfigured() && s.NotifyEmail != "" && (s.NotifyNewErrors || s.NotifyRegressions || s.NotifySpikes)
}

// SendEmail sends an email using the configured SMTP settings
func (s *Settings) SendEmail(subject, body string) error {
	if !s.IsConfigured() || s.NotifyEmail == "" {
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
