// Package apikeys provides API key management for LogNorth
package apikeys

import (
	"crypto/rand"
	"encoding/hex"
	"time"

	"gorm.io/gorm"
)

// KeyType distinguishes between app keys (write) and agent keys (read-only)
type KeyType string

const (
	KeyTypeApp   KeyType = "app"   // lb_ prefix, write to one app
	KeyTypeAgent KeyType = "agent" // lna_ prefix, read all apps
)

// APIKey represents an API key for event ingestion or agent access
type APIKey struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	Name      string    `gorm:"size:100;not null" json:"name"`
	Key       string    `gorm:"uniqueIndex;size:64;not null" json:"-"`
	KeyPrefix string    `gorm:"size:12;not null" json:"key_prefix"` // "lb_" or "lna_" + 8 hex chars
	KeyType   KeyType   `gorm:"size:10;not null;default:'app'" json:"key_type"`
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

// CreateAgent generates and saves a new agent API key (read-only access)
func CreateAgent(db *gorm.DB) (*APIKey, string, error) {
	key, err := Generate("lgn-agent-")
	if err != nil {
		return nil, "", err
	}

	apiKey := &APIKey{
		Name:      "Agent API",
		Key:       key,
		KeyPrefix: key[:18], // "lgn-agent-" + first 8 hex chars
		KeyType:   KeyTypeAgent,
		Active:    true,
	}

	if err := db.Create(apiKey).Error; err != nil {
		return nil, "", err
	}

	return apiKey, key, nil
}

// GetAgentKey retrieves the active agent key if one exists
func GetAgentKey(db *gorm.DB) (*APIKey, error) {
	var key APIKey
	err := db.Where("key_type = ? AND active = ?", KeyTypeAgent, true).First(&key).Error
	if err != nil {
		return nil, err
	}
	return &key, nil
}

// RegenerateAgentKey revokes existing agent key and creates a new one
func RegenerateAgentKey(db *gorm.DB) (*APIKey, string, error) {
	// Revoke any existing agent keys
	db.Model(&APIKey{}).Where("key_type = ?", KeyTypeAgent).Update("active", false)

	// Create new one
	return CreateAgent(db)
}

// Validate checks if an API key is valid and active (any type)
func Validate(db *gorm.DB, key string) (*APIKey, error) {
	var apiKey APIKey
	if err := db.Where("key = ? AND active = ?", key, true).First(&apiKey).Error; err != nil {
		return nil, err
	}

	// Update last used timestamp
	db.Model(&apiKey).Update("last_used", time.Now())

	return &apiKey, nil
}

// ValidateAppKey checks if an API key is valid, active, and is an app key (for ingestion)
func ValidateAppKey(db *gorm.DB, key string) (*APIKey, error) {
	var apiKey APIKey
	if err := db.Where("key = ? AND active = ? AND key_type = ?", key, true, KeyTypeApp).First(&apiKey).Error; err != nil {
		return nil, err
	}

	db.Model(&apiKey).Update("last_used", time.Now())

	return &apiKey, nil
}

// ValidateAgentKey checks if an API key is valid, active, and is an agent key (read-only)
func ValidateAgentKey(db *gorm.DB, key string) (*APIKey, error) {
	var apiKey APIKey
	if err := db.Where("key = ? AND active = ? AND key_type = ?", key, true, KeyTypeAgent).First(&apiKey).Error; err != nil {
		return nil, err
	}

	db.Model(&apiKey).Update("last_used", time.Now())

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

// HasAny returns true if at least one API key exists
func HasAny(db *gorm.DB) bool {
	var count int64
	db.Model(&APIKey{}).Where("active = ?", true).Count(&count)
	return count > 0
}
