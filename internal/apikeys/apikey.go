// Package apikeys provides API key management for LogNorth
package apikeys

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"gorm.io/gorm"
)

// Sentinel errors returned by Rename so the HTTP layer can surface a
// user-facing message without leaking gorm details.
var (
	ErrNameRequired  = errors.New("Name can't be empty")
	ErrNameTooLong   = errors.New("Too long — 60 characters max")
	ErrNameDuplicate = errors.New("That name's taken")
	ErrAppNotFound   = errors.New("Can't find that app")
)

// KeyType distinguishes between key types
type KeyType string

const (
	KeyTypeApp KeyType = "app" // lgn- prefix, write to one app
)

// APIKey represents an API key for event ingestion
type APIKey struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	Name      string    `gorm:"size:100;not null" json:"name"`
	Key       string    `gorm:"uniqueIndex;size:64;not null" json:"-"`
	KeyPrefix string    `gorm:"size:12;not null" json:"key_prefix"` // "lgn-" + 8 hex chars
	KeyType   KeyType   `gorm:"size:10;not null;default:'app'" json:"key_type"`
	Active    bool      `gorm:"default:true" json:"active"`
	LastUsed  time.Time `json:"last_used,omitempty"`

	// Uptime ping (active HTTP probe). The user provides the URL when
	// adding the app (or via the [edit] UI). Pinged every minute, alert
	// after 3 consecutive fails. Apps without a URL are skipped.
	PingURL          string    `gorm:"size:500" json:"ping_url"`
	PingDisabled     bool      `gorm:"default:false" json:"ping_disabled"`
	IsDown           bool      `gorm:"default:false" json:"is_down"`
	DownNotified     bool      `gorm:"default:false" json:"-"` // The down alert went out
	ConsecutiveFails int       `gorm:"default:0" json:"consecutive_fails"`
	DownSince        time.Time `json:"down_since,omitempty"`
	LastPingedAt     time.Time `json:"last_pinged_at,omitempty"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// TableName specifies the table name
func (APIKey) TableName() string {
	return "api_keys"
}

// Generate creates a new random API key with the given prefix
func Generate(prefix string) (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return prefix + hex.EncodeToString(bytes), nil
}

// Create generates and saves a new app API key (for event ingestion)
func Create(db *gorm.DB, name string) (*APIKey, string, error) {
	key, err := Generate("lgn-")
	if err != nil {
		return nil, "", err
	}

	apiKey := &APIKey{
		Name:      name,
		Key:       key,
		KeyPrefix: key[:12], // "lgn-" + first 8 hex chars
		KeyType:   KeyTypeApp,
		Active:    true,
	}

	if err := db.Create(apiKey).Error; err != nil {
		return nil, "", err
	}

	return apiKey, key, nil
}


// ValidateAppKey checks if an API key is valid, active, and is an app key (for ingestion)
func ValidateAppKey(db *gorm.DB, key string) (*APIKey, error) {
	var apiKey APIKey
	if err := db.Where("key = ? AND active = ? AND key_type = ?", key, true, KeyTypeApp).First(&apiKey).Error; err != nil {
		return nil, err
	}

	// At most one write per minute per key: ingest is the hot path.
	if time.Since(apiKey.LastUsed) > time.Minute {
		db.Model(&apiKey).Update("last_used", time.Now())
	}

	return &apiKey, nil
}


// List retrieves all app API keys (excludes agent keys)
func List(db *gorm.DB) ([]APIKey, error) {
	var keys []APIKey
	if err := db.Where("key_type = ? OR key_type IS NULL OR key_type = ''", KeyTypeApp).Order("created_at DESC").Find(&keys).Error; err != nil {
		return nil, err
	}
	return keys, nil
}

// FindByID retrieves an API key by ID
func FindByID(db *gorm.DB, id uint) (*APIKey, error) {
	var key APIKey
	if err := db.First(&key, id).Error; err != nil {
		return nil, err
	}
	return &key, nil
}

// Revoke deactivates an API key
func Revoke(db *gorm.DB, id uint) error {
	return db.Model(&APIKey{}).Where("id = ?", id).Update("active", false).Error
}

// Delete removes an API key
func Delete(db *gorm.DB, id uint) error {
	return db.Delete(&APIKey{}, id).Error
}

// SetPingURL sets the URL the uptime checker pings. Clears any current
// down state so the next probe starts fresh. Empty URL disables until
// auto-discovery picks one up again on the next event.
func SetPingURL(db *gorm.DB, id uint, url string) error {
	url = strings.TrimSpace(url)
	return db.Model(&APIKey{}).Where("id = ?", id).Updates(map[string]any{
		"ping_url":          url,
		"ping_disabled":     false,
		"is_down":           false,
		"consecutive_fails": 0,
		"down_since":        nil,
	}).Error
}

// SetPingDisabled flips the user's explicit opt-out. When disabled,
// auto-discovery is also suppressed.
func SetPingDisabled(db *gorm.DB, id uint, disabled bool) error {
	updates := map[string]any{"ping_disabled": disabled}
	if disabled {
		updates["is_down"] = false
		updates["consecutive_fails"] = 0
		updates["down_since"] = nil
	}
	return db.Model(&APIKey{}).Where("id = ?", id).Updates(updates).Error
}

// Rename updates the display name of an app API key. Trims whitespace,
// enforces a 60-char limit, and rejects names already in use by another
// active app. Returns one of ErrName* on validation failure.
func Rename(db *gorm.DB, id uint, newName string) error {
	trimmed := strings.TrimSpace(newName)
	if trimmed == "" {
		return ErrNameRequired
	}
	if utf8.RuneCountInString(trimmed) > 60 {
		return ErrNameTooLong
	}

	var dup int64
	db.Model(&APIKey{}).
		Where("id != ? AND name = ? AND active = ?", id, trimmed, true).
		Count(&dup)
	if dup > 0 {
		return ErrNameDuplicate
	}

	result := db.Model(&APIKey{}).Where("id = ?", id).Update("name", trimmed)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrAppNotFound
	}
	return nil
}

// HasAny returns true if at least one API key exists
func HasAny(db *gorm.DB) bool {
	var count int64
	db.Model(&APIKey{}).Where("active = ?", true).Count(&count)
	return count > 0
}
