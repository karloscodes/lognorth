// Package settings provides application settings management
package settings

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/smtp"
	"os"
	"time"

	"gorm.io/gorm"
)

const (
	RelayURL     = "https://lognorth.com/.netlify/functions/alert-relay"
	RelayTimeout = 10 * time.Second
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
// Works with either SMTP or relay (license key from env)
func (s *Settings) CanNotify() bool {
	hasEmail := s.NotifyEmail != ""
	hasAlerts := s.NotifyNewErrors || s.NotifyRegressions || s.NotifySpikes
	canSend := s.IsConfigured() || s.canUseRelay()
	return hasEmail && hasAlerts && canSend
}

// canUseRelay returns true if relay can be used (license key set)
func (s *Settings) canUseRelay() bool {
	return os.Getenv("LOGNORTH_PRIVATE_KEY") != ""
}

// SendEmail sends an email using SMTP if configured, otherwise uses relay
func (s *Settings) SendEmail(subject, body string) error {
	if s.NotifyEmail == "" {
		return fmt.Errorf("no notification email configured")
	}

	// Prefer SMTP if configured
	if s.IsConfigured() {
		return s.sendViaSMTP(subject, body)
	}

	// Fall back to relay
	return s.sendViaRelay(subject, body)
}

// sendViaSMTP sends email via configured SMTP server
func (s *Settings) sendViaSMTP(subject, body string) error {
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

// sendViaRelay sends email via lognorth.com relay
func (s *Settings) sendViaRelay(subject, body string) error {
	licenseKey := os.Getenv("LOGNORTH_PRIVATE_KEY")
	if licenseKey == "" {
		return fmt.Errorf("no license key configured for relay")
	}

	payload := map[string]string{
		"license_key": licenseKey,
		"to_email":    s.NotifyEmail,
		"subject":     subject,
		"body":        body,
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal relay payload: %w", err)
	}

	client := &http.Client{Timeout: RelayTimeout}
	resp, err := client.Post(RelayURL, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("relay request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == 429 {
		return fmt.Errorf("daily relay limit reached - configure SMTP in Settings")
	}

	if resp.StatusCode != 200 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("relay failed (%d): %s", resp.StatusCode, string(respBody))
	}

	return nil
}
