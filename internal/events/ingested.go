package events

import (
	"time"

	"gorm.io/gorm"
)

// IngestedEvent represents a raw event stored before processing
// Uses the same schema as Event for simplicity
type IngestedEvent struct {
	ID         uint      `gorm:"primaryKey"`
	APIKeyID   uint      `gorm:"index"`
	Message    string    `gorm:"size:1000"`
	TraceID    string    `gorm:"index;size:100"`
	DurationMS int
	IsError    bool      `gorm:"index"`
	Context    string    `gorm:"type:text"` // JSON with all metadata
	Timestamp  time.Time `gorm:"index"`
	CreatedAt  time.Time `gorm:"index"`
	Processed  int       `gorm:"index;default:0"` // 0 = unprocessed, 1 = processed
}

// TableName specifies the table name
func (IngestedEvent) TableName() string {
	return "ingested_events"
}

// AutoMigrateIngested creates the ingested_events table
func AutoMigrateIngested(db *gorm.DB) error {
	return db.AutoMigrate(&IngestedEvent{})
}

// CreateIngested inserts a new ingested event (fast write, minimal processing)
func CreateIngested(db *gorm.DB, event *IngestedEvent) error {
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now()
	}
	event.CreatedAt = time.Now()
	event.Processed = 0
	return db.Create(event).Error
}

// GetUnprocessed fetches unprocessed events up to the given limit
func GetUnprocessed(db *gorm.DB, limit int) ([]IngestedEvent, error) {
	var events []IngestedEvent
	err := db.Where("processed = 0").
		Order("created_at ASC").
		Limit(limit).
		Find(&events).Error
	return events, err
}

// MarkProcessed marks events as processed by their IDs
func MarkProcessed(db *gorm.DB, ids []uint) error {
	if len(ids) == 0 {
		return nil
	}
	return db.Model(&IngestedEvent{}).
		Where("id IN ?", ids).
		Update("processed", 1).Error
}

// CountUnprocessed returns the count of unprocessed events
func CountUnprocessed(db *gorm.DB) (int64, error) {
	var count int64
	err := db.Model(&IngestedEvent{}).Where("processed = 0").Count(&count).Error
	return count, err
}

// DeleteProcessed removes processed events older than the given duration
func DeleteProcessed(db *gorm.DB, olderThan time.Duration) (int64, error) {
	cutoff := time.Now().Add(-olderThan)
	result := db.Where("processed = 1 AND created_at < ?", cutoff).Delete(&IngestedEvent{})
	return result.RowsAffected, result.Error
}
