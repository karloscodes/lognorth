package events

import (
	"time"

	"gorm.io/gorm"
)

// EndpointStat tracks per-endpoint metrics in 5-minute buckets
type EndpointStat struct {
	ID          uint      `gorm:"primaryKey"`
	APIKeyID    uint      `gorm:"index:idx_endpoint_stat_lookup"`
	Path        string    `gorm:"size:500;index:idx_endpoint_stat_lookup"`
	Bucket      time.Time `gorm:"index:idx_endpoint_stat_lookup"` // Truncated to 5-min
	Total       int64     `gorm:"default:0"`
	Errors      int64     `gorm:"default:0"`
	AvgDuration int       `gorm:"default:0"`
}

func (EndpointStat) TableName() string {
	return "endpoint_stats"
}

// AutoMigrateStats creates the endpoint_stats table
func AutoMigrateStats(db *gorm.DB) error {
	return db.AutoMigrate(&EndpointStat{})
}

// TruncateTo5Min truncates a time to 5-minute boundary
func TruncateTo5Min(t time.Time) time.Time {
	return t.Truncate(5 * time.Minute)
}

// RecordEndpointStat increments stats for an endpoint
func RecordEndpointStat(db *gorm.DB, apiKeyID uint, path string, isError bool, durationMS int, timestamp time.Time) error {
	if path == "" {
		return nil
	}

	// UTC: SQLite compares times as text, so all buckets need the same offset.
	bucket := TruncateTo5Min(timestamp.UTC())

	// Upsert: increment counters
	var stat EndpointStat
	result := db.Where("api_key_id = ? AND path = ? AND bucket = ?", apiKeyID, path, bucket).First(&stat)

	if result.Error == gorm.ErrRecordNotFound {
		// Create new
		stat = EndpointStat{
			APIKeyID:    apiKeyID,
			Path:        path,
			Bucket:      bucket,
			Total:       1,
			Errors:      0,
			AvgDuration: durationMS,
		}
		if isError {
			stat.Errors = 1
		}
		return db.Create(&stat).Error
	}

	// Update existing
	stat.Total++
	if isError {
		stat.Errors++
	}
	// Running average for duration
	if durationMS > 0 {
		stat.AvgDuration = (stat.AvgDuration*(int(stat.Total)-1) + durationMS) / int(stat.Total)
	}

	return db.Save(&stat).Error
}

// LastCompleteBucketTime returns the start of the last complete 5-minute bucket.
// The current bucket is still filling, so its counts are partial.
func LastCompleteBucketTime() time.Time {
	return TruncateTo5Min(time.Now().UTC()).Add(-5 * time.Minute)
}

// GetActiveEndpoints returns the endpoints with traffic in the last day.
// A day, not an hour, so the silence check keeps watching an endpoint that stopped.
func GetActiveEndpoints(db *gorm.DB, apiKeyID uint) []string {
	since := time.Now().UTC().Add(-24 * time.Hour)

	var paths []string
	db.Model(&EndpointStat{}).
		Where("api_key_id = ? AND bucket > ?", apiKeyID, since).
		Distinct("path").
		Pluck("path", &paths)

	return paths
}

// GetLastCompleteBucket returns the stats for an endpoint in the last complete bucket.
// It returns an empty stat when the endpoint had no traffic in that bucket.
func GetLastCompleteBucket(db *gorm.DB, apiKeyID uint, path string) (EndpointStat, error) {
	var stat EndpointStat
	result := db.Where("api_key_id = ? AND path = ? AND bucket = ?", apiKeyID, path, LastCompleteBucketTime()).First(&stat)
	if result.Error == gorm.ErrRecordNotFound {
		return EndpointStat{}, nil
	}
	return stat, result.Error
}

// ErrorRate returns the error percentage, or 0 when there is no traffic.
func (s EndpointStat) ErrorRate() float64 {
	if s.Total == 0 {
		return 0
	}
	return float64(s.Errors) / float64(s.Total) * 100
}

// StatsRetention covers the longest SPC history (4 weeks of volume) plus a day.
const StatsRetention = (VolumeHistoryWeeks*7 + 1) * 24 * time.Hour

// CleanupOldStats removes stats older than StatsRetention.
func CleanupOldStats(db *gorm.DB) (int64, error) {
	cutoff := time.Now().UTC().Add(-StatsRetention)
	result := db.Where("bucket < ?", cutoff).Delete(&EndpointStat{})
	return result.RowsAffected, result.Error
}
