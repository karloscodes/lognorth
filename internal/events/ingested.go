package events

import (
	"time"

	"gorm.io/gorm"
)

// IngestedEvent is a raw event waiting for the processor. Ingest writes it,
// the processor moves it into events and deletes it.
type IngestedEvent struct {
	ID         uint `gorm:"primaryKey"`
	APIKeyID   uint
	Message    string `gorm:"size:1000"`
	TraceID    string `gorm:"size:100"`
	DurationMS int
	IsError    bool
	Context    string `gorm:"type:text"` // JSON with all metadata
	Timestamp  time.Time
	CreatedAt  time.Time
}

// TableName specifies the table name
func (IngestedEvent) TableName() string {
	return "ingested_events"
}

// DropProcessedState removes what the old processed flag left behind. Rows
// used to be marked processed and deleted an hour later; they are now deleted
// when processed. The marked rows and the indexes the processor never used go.
func DropProcessedState(db *gorm.DB) error {
	if db.Migrator().HasColumn(&IngestedEvent{}, "processed") {
		if err := db.Exec("DELETE FROM ingested_events WHERE processed = 1").Error; err != nil {
			return err
		}
	}
	for _, index := range []string{"processed", "api_key_id", "trace_id", "is_error", "timestamp", "created_at"} {
		if err := db.Exec("DROP INDEX IF EXISTS idx_ingested_events_" + index).Error; err != nil {
			return err
		}
	}
	return nil
}

// CreateIngested stores events in one transaction: all of them or none.
func CreateIngested(db *gorm.DB, events []*IngestedEvent) error {
	if len(events) == 0 {
		return nil
	}
	now := time.Now().UTC()
	for _, e := range events {
		if e.Timestamp.IsZero() {
			e.Timestamp = now
		}
		e.Timestamp = e.Timestamp.UTC()
		e.CreatedAt = now
	}
	return db.Transaction(func(tx *gorm.DB) error {
		return tx.CreateInBatches(events, 200).Error
	})
}

// NextIngested returns up to limit waiting events with an id above afterID, oldest first.
func NextIngested(db *gorm.DB, afterID uint, limit int) ([]IngestedEvent, error) {
	var events []IngestedEvent
	err := db.Where("id > ?", afterID).Order("id").Limit(limit).Find(&events).Error
	return events, err
}

// DeleteIngestedOlderThan removes waiting events older than the given duration.
// A row that fails to process stays, so this keeps the table bounded.
func DeleteIngestedOlderThan(db *gorm.DB, olderThan time.Duration) (int64, error) {
	result := db.Where("created_at < ?", time.Now().UTC().Add(-olderThan)).Delete(&IngestedEvent{})
	return result.RowsAffected, result.Error
}
