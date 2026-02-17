package events

import (
	"time"
)

// BaselineStat stores learned normal behavior per app/endpoint/hour-of-week.
// Hour-of-week ranges from 0-167 (Monday 00:00 = 0, Sunday 23:00 = 167).
type BaselineStat struct {
	ID          uint      `gorm:"primaryKey"`
	APIKeyID    uint      `gorm:"index:idx_baseline_lookup"`
	Path        string    `gorm:"size:500;index:idx_baseline_lookup"`
	HourOfWeek  int       `gorm:"index:idx_baseline_lookup"`
	Mean        float64   `gorm:"default:0"`
	StdDev      float64   `gorm:"default:0"`
	SampleCount int       `gorm:"default:0"`
	UpdatedAt   time.Time
}

// CUSUMState tracks cumulative deviation for drift detection.
type CUSUMState struct {
	ID         uint      `gorm:"primaryKey"`
	APIKeyID   uint      `gorm:"uniqueIndex:idx_cusum_lookup"`
	Path       string    `gorm:"size:500;uniqueIndex:idx_cusum_lookup"`
	Cumulative float64   `gorm:"default:0"`
	StartedAt  time.Time
}

// DigestItem stores warnings for the daily digest email.
type DigestItem struct {
	ID        uint      `gorm:"primaryKey"`
	APIKeyID  uint      `gorm:"index"`
	Path      string    `gorm:"size:500"`
	Message   string    `gorm:"type:text"`
	Severity  string    `gorm:"size:20"`
	Resolved  bool      `gorm:"default:false"`
	CreatedAt time.Time
}

// VolumeBaselineStat stores learned normal request volume per hour-of-week.
type VolumeBaselineStat struct {
	ID          uint      `gorm:"primaryKey"`
	APIKeyID    uint      `gorm:"index:idx_volume_baseline_lookup"`
	Path        string    `gorm:"size:500;index:idx_volume_baseline_lookup"`
	HourOfWeek  int       `gorm:"index:idx_volume_baseline_lookup"`
	Mean        float64   `gorm:"default:0"`
	StdDev      float64   `gorm:"default:0"`
	SampleCount int       `gorm:"default:0"`
	UpdatedAt   time.Time
}

// SilenceState tracks silence alerting state per endpoint.
type SilenceState struct {
	ID             uint      `gorm:"primaryKey"`
	APIKeyID       uint      `gorm:"uniqueIndex:idx_silence_state_lookup"`
	Path           string    `gorm:"size:500;uniqueIndex:idx_silence_state_lookup"`
	IsSilent       bool      `gorm:"default:false"`
	Severity       string    `gorm:"size:20"`
	SilenceStartAt time.Time
	LastAlertAt    time.Time
}

// AutoMigrateSPC runs migrations for SPC tables.
func AutoMigrateSPC(db interface{ AutoMigrate(dst ...any) error }) error {
	return db.AutoMigrate(
		&BaselineStat{},
		&CUSUMState{},
		&DigestItem{},
		&VolumeBaselineStat{},
		&SilenceState{},
	)
}
