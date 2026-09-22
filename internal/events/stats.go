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

	bucket := TruncateTo5Min(timestamp)

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
	return TruncateTo5Min(time.Now()).Add(-5 * time.Minute)
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

// GetBaselineErrorRate returns the average error rate for an endpoint
// based on the same time-of-day over the past 7 days
func GetBaselineErrorRate(db *gorm.DB, apiKeyID uint, path string) (float64, error) {
	now := time.Now()
	currentBucket := TruncateTo5Min(now)

	// Get same 5-min window from past 7 days
	var totalEvents int64
	var totalErrors int64

	for i := 1; i <= 7; i++ {
		targetBucket := currentBucket.AddDate(0, 0, -i)

		var stat EndpointStat
		result := db.Where("api_key_id = ? AND path = ? AND bucket = ?", apiKeyID, path, targetBucket).First(&stat)
		if result.Error == nil {
			totalEvents += stat.Total
			totalErrors += stat.Errors
		}
	}

	if totalEvents == 0 {
		return 0, nil // No baseline data
	}

	return float64(totalErrors) / float64(totalEvents) * 100, nil
}

// EndpointSpike represents a detected spike
type EndpointSpike struct {
	APIKeyID    uint
	Path        string
	CurrentRate float64
	Baseline    float64
	Total       int64
}

// DetectSpikes finds endpoints with error rates significantly above baseline
func DetectSpikes(db *gorm.DB, threshold int) ([]EndpointSpike, error) {
	currentBucket := TruncateTo5Min(time.Now())

	// Get all stats for current bucket with minimum traffic
	var currentStats []EndpointStat
	if err := db.Where("bucket = ? AND total >= 10", currentBucket).Find(&currentStats).Error; err != nil {
		return nil, err
	}

	var spikes []EndpointSpike

	for _, stat := range currentStats {
		currentRate := float64(stat.Errors) / float64(stat.Total) * 100
		baseline, err := GetBaselineErrorRate(db, stat.APIKeyID, stat.Path)
		if err != nil {
			continue
		}

		if isSignificantSpike(currentRate, baseline, threshold) {
			spikes = append(spikes, EndpointSpike{
				APIKeyID:    stat.APIKeyID,
				Path:        stat.Path,
				CurrentRate: currentRate,
				Baseline:    baseline,
				Total:       stat.Total,
			})
		}
	}

	return spikes, nil
}

// isSignificantSpike determines if the current rate is a real spike
func isSignificantSpike(current, baseline float64, threshold int) bool {
	if baseline == 0 {
		// No baseline yet, use absolute threshold
		return current >= float64(threshold)
	}

	// Alert if: 3x baseline OR +15% absolute increase (whichever is lower)
	return current >= baseline*3 || current >= baseline+15
}

// CleanupOldStats removes stats older than 8 days (7 days for baseline + 1 day buffer)
func CleanupOldStats(db *gorm.DB) (int64, error) {
	cutoff := time.Now().AddDate(0, 0, -8)
	result := db.Where("bucket < ?", cutoff).Delete(&EndpointStat{})
	return result.RowsAffected, result.Error
}
