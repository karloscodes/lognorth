package events

import (
	"time"

	"gorm.io/gorm"
)

// DigestItem stores warnings for the daily digest email.
type DigestItem struct {
	ID        uint   `gorm:"primaryKey"`
	APIKeyID  uint   `gorm:"index"`
	Path      string `gorm:"size:500"`
	Message   string `gorm:"type:text"`
	Severity  string `gorm:"size:20"`
	Resolved  bool   `gorm:"default:false"`
	CreatedAt time.Time
}

// SilenceState tracks silence alerting state per endpoint.
type SilenceState struct {
	ID             uint   `gorm:"primaryKey"`
	APIKeyID       uint   `gorm:"uniqueIndex:idx_silence_state_lookup"`
	Path           string `gorm:"size:500;uniqueIndex:idx_silence_state_lookup"`
	Severity       string `gorm:"size:20"`
	CleanChecks    int    `gorm:"default:0"`
	SilenceStartAt time.Time
	LastAlertAt    time.Time
}

// AutoMigrateSPC runs migrations for SPC tables.
func AutoMigrateSPC(db *gorm.DB) error {
	if err := db.AutoMigrate(&DigestItem{}, &SilenceState{}); err != nil {
		return err
	}

	// Baselines come from endpoint_stats now. Drop the old learned tables.
	for _, table := range []string{"baseline_stats", "volume_baseline_stats", "cusum_states"} {
		if err := db.Migrator().DropTable(table); err != nil {
			return err
		}
	}
	return nil
}
