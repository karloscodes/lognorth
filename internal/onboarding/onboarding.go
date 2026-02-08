// Package onboarding provides first-run setup for LogNorth
package onboarding

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"time"

	"gorm.io/gorm"

	"lognorth/internal/users"
)

// OnboardingStep represents the current step in the onboarding process
type OnboardingStep string

const (
	StepUserAccount OnboardingStep = "user_account"
	StepPassword    OnboardingStep = "password"
	StepCompleted   OnboardingStep = "completed"
)

// OnboardingData holds the collected onboarding information
type OnboardingData struct {
	Email    string `json:"email,omitempty"`
	Password string `json:"password,omitempty"`
}

// Scan implements sql.Scanner interface for OnboardingData
func (od *OnboardingData) Scan(value interface{}) error {
	if value == nil {
		*od = OnboardingData{}
		return nil
	}

	switch v := value.(type) {
	case []byte:
		return json.Unmarshal(v, od)
	case string:
		return json.Unmarshal([]byte(v), od)
	default:
		return fmt.Errorf("cannot scan %T into OnboardingData", value)
	}
}

// Value implements driver.Valuer interface for OnboardingData
func (od OnboardingData) Value() (driver.Value, error) {
	return json.Marshal(od)
}

// OnboardingSession tracks the multi-step onboarding process
type OnboardingSession struct {
	ID        string         `gorm:"primaryKey;type:text"`
	Step      OnboardingStep `gorm:"type:text;not null"`
	Data      OnboardingData `gorm:"type:text"`
	Completed bool           `gorm:"default:false"`
	ExpiresAt time.Time      `gorm:"not null"`
	CreatedAt time.Time      `gorm:"not null;autoCreateTime:milli"`
	UpdatedAt time.Time      `gorm:"not null;autoUpdateTime:milli"`
}

// AutoMigrate creates the onboarding_sessions table
func AutoMigrate(db *gorm.DB) error {
	return db.AutoMigrate(&OnboardingSession{})
}

// IsExpired checks if the onboarding session has expired
func (os *OnboardingSession) IsExpired() bool {
	return time.Now().After(os.ExpiresAt)
}

// CreateSession creates a new onboarding session with 1-hour expiration
func CreateSession(db *gorm.DB, sessionID string) (*OnboardingSession, error) {
	session := &OnboardingSession{
		ID:        sessionID,
		Step:      StepUserAccount,
		Data:      OnboardingData{},
		Completed: false,
		ExpiresAt: time.Now().Add(1 * time.Hour),
	}

	if err := db.Create(session).Error; err != nil {
		return nil, fmt.Errorf("failed to create onboarding session: %w", err)
	}

	return session, nil
}

// UpdateSession updates the session step and data
func UpdateSession(db *gorm.DB, sessionID string, step OnboardingStep, data OnboardingData) error {
	result := db.Model(&OnboardingSession{}).
		Where("id = ? AND completed = ? AND expires_at > ?", sessionID, false, time.Now()).
		Updates(map[string]interface{}{
			"step": step,
			"data": data,
		})

	if result.Error != nil {
		return fmt.Errorf("failed to update onboarding session: %w", result.Error)
	}

	if result.RowsAffected == 0 {
		return fmt.Errorf("onboarding session not found or expired")
	}

	return nil
}

// GetSession retrieves an active onboarding session
func GetSession(db *gorm.DB, sessionID string) (*OnboardingSession, error) {
	var session OnboardingSession
	err := db.Where("id = ? AND completed = ? AND expires_at > ?", sessionID, false, time.Now()).
		First(&session).Error

	if err != nil {
		return nil, err
	}

	return &session, nil
}

// CompleteSession marks the session as completed
func CompleteSession(db *gorm.DB, sessionID string) error {
	result := db.Model(&OnboardingSession{}).
		Where("id = ? AND completed = ?", sessionID, false).
		Update("completed", true)

	if result.Error != nil {
		return fmt.Errorf("failed to complete onboarding session: %w", result.Error)
	}

	if result.RowsAffected == 0 {
		return fmt.Errorf("onboarding session not found")
	}

	return nil
}

// CleanupExpiredSessions removes expired onboarding sessions
func CleanupExpiredSessions(db *gorm.DB) error {
	result := db.Where("expires_at < ?", time.Now()).Delete(&OnboardingSession{})
	return result.Error
}

// IsOnboardingRequired checks if onboarding is required (no users exist)
func IsOnboardingRequired(db *gorm.DB) (bool, error) {
	return users.IsOnboardingRequired(db)
}
