// Package apikeys provides API key management for LogNorth
package apikeys

import (
	"crypto/rand"
	"encoding/hex"
	"time"

	"gorm.io/gorm"
)

// APIKey represents an API key for event ingestion
type APIKey struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	Name      string    `gorm:"size:100;not null" json:"name"`
	Key       string    `gorm:"uniqueIndex;size:64;not null" json:"-"`
	KeyPrefix string    `gorm:"size:11;not null" json:"key_prefix"` // "lb_" + 8 hex chars
	Active    bool      `gorm:"default:true" json:"active"`
	LastUsed  time.Time `json:"last_used,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// TableName specifies the table name
func (APIKey) TableName() string {
	return "api_keys"
}

// AutoMigrate creates the api_keys table if it doesn't exist
func AutoMigrate(db *gorm.DB) error {
	return db.AutoMigrate(&APIKey{})
}

// Generate creates a new random API key
func Generate() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return "lb_" + hex.EncodeToString(bytes), nil
}

// Create generates and saves a new API key
func Create(db *gorm.DB, name string) (*APIKey, string, error) {
	key, err := Generate()
	if err != nil {
		return nil, "", err
	}

	apiKey := &APIKey{
		Name:      name,
		Key:       key,
		KeyPrefix: key[:11], // "lb_" + first 8 hex chars
		Active:    true,
	}

	if err := db.Create(apiKey).Error; err != nil {
		return nil, "", err
	}

	return apiKey, key, nil
}

// Validate checks if an API key is valid and active
func Validate(db *gorm.DB, key string) (*APIKey, error) {
	var apiKey APIKey
	if err := db.Where("key = ? AND active = ?", key, true).First(&apiKey).Error; err != nil {
		return nil, err
	}

	// Update last used timestamp
	db.Model(&apiKey).Update("last_used", time.Now())

	return &apiKey, nil
}

// List retrieves all API keys
func List(db *gorm.DB) ([]APIKey, error) {
	var keys []APIKey
	if err := db.Order("created_at DESC").Find(&keys).Error; err != nil {
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

// HasAny returns true if at least one API key exists
func HasAny(db *gorm.DB) bool {
	var count int64
	db.Model(&APIKey{}).Where("active = ?", true).Count(&count)
	return count > 0
}
