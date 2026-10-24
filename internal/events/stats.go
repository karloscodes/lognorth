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

// TruncateTo5Min truncates a time to 5-minute boundary
func TruncateTo5Min(t time.Time) time.Time {
	return t.Truncate(5 * time.Minute)
}

// StatBatch counts endpoint stats for many events and writes each bucket
// once. Each bucket row is read once, the events apply in order, and the row
// is written once, so the result matches one upsert per event.
type StatBatch struct {
	stats map[statKey]*EndpointStat
	order []statKey
}

type statKey struct {
	apiKeyID uint
	path     string
	bucket   time.Time
}

// NewStatBatch returns an empty batch.
func NewStatBatch() *StatBatch {
	return &StatBatch{stats: map[statKey]*EndpointStat{}}
}

// Add counts one event. It reads the bucket row the first time it sees the
// bucket. Use the same transaction for Add and Flush.
func (b *StatBatch) Add(tx *gorm.DB, apiKeyID uint, path string, isError bool, durationMS int, timestamp time.Time) error {
	if path == "" {
		return nil
	}
	// UTC: SQLite compares times as text, so all buckets need the same offset.
	key := statKey{apiKeyID, path, TruncateTo5Min(timestamp.UTC())}

	stat, seen := b.stats[key]
	if !seen {
		var row EndpointStat
		err := tx.Where("api_key_id = ? AND path = ? AND bucket = ?", key.apiKeyID, key.path, key.bucket).
			Limit(1).Find(&row).Error
		if err != nil {
			return err
		}
		if row.ID != 0 {
			stat = &row
		}
		b.order = append(b.order, key)
	}

	if stat == nil {
		stat = &EndpointStat{APIKeyID: apiKeyID, Path: path, Bucket: key.bucket, Total: 1, AvgDuration: durationMS}
		if isError {
			stat.Errors = 1
		}
		b.stats[key] = stat
		return nil
	}

	stat.Total++
	if isError {
		stat.Errors++
	}
	// The average over every event of the bucket, as GetEndpointStatsSince
	// weighs it. A 0 is a request that took under 1 ms: the SDKs send a
	// duration with every request.
	stat.AvgDuration = (stat.AvgDuration*(int(stat.Total)-1) + durationMS) / int(stat.Total)
	b.stats[key] = stat
	return nil
}

// Flush writes every bucket the batch touched.
func (b *StatBatch) Flush(tx *gorm.DB) error {
	for _, key := range b.order {
		if err := tx.Save(b.stats[key]).Error; err != nil {
			return err
		}
	}
	return nil
}

// LastCompleteBucket is the start of the 5-minute bucket that ended at least
// a minute before now. The SDKs flush every few seconds and the processor
// moves events every 5, so a bucket that ended a moment ago can still fill.
func LastCompleteBucket(now time.Time) time.Time {
	return TruncateTo5Min(now.UTC().Add(-time.Minute)).Add(-5 * time.Minute)
}

// LastCompleteBucketTime is LastCompleteBucket now.
func LastCompleteBucketTime() time.Time {
	return LastCompleteBucket(time.Now())
}

// GetActiveEndpoints returns the endpoints with traffic in the day before now.
// A day, not an hour, so the silence check keeps watching an endpoint that stopped.
func GetActiveEndpoints(db *gorm.DB, apiKeyID uint, now time.Time) []string {
	since := now.UTC().Add(-24 * time.Hour)

	var paths []string
	db.Model(&EndpointStat{}).
		Where("api_key_id = ? AND bucket > ?", apiKeyID, since).
		Distinct("path").
		Pluck("path", &paths)

	return paths
}

// GetCompleteBucket returns the stats of an endpoint in the bucket that
// starts at `at`, or an empty stat when the endpoint had no traffic then.
func GetCompleteBucket(db *gorm.DB, apiKeyID uint, path string, at time.Time) (EndpointStat, error) {
	var stat EndpointStat
	err := db.Where("api_key_id = ? AND path = ? AND bucket = ?", apiKeyID, path, at.UTC()).Limit(1).Find(&stat).Error
	return stat, err
}

// ErrorRate returns the error percentage, or 0 when there is no traffic.
func (s EndpointStat) ErrorRate() float64 {
	if s.Total == 0 {
		return 0
	}
	return float64(s.Errors) / float64(s.Total) * 100
}

// StatsRetention covers the longest history an alert reads (4 weeks of
// volume, in internal/alerts) plus a day. A test there checks it.
const StatsRetention = 29 * 24 * time.Hour

// CleanupOldStats removes stats older than StatsRetention.
func CleanupOldStats(db *gorm.DB) (int64, error) {
	cutoff := time.Now().UTC().Add(-StatsRetention)
	result := db.Where("bucket < ?", cutoff).Delete(&EndpointStat{})
	return result.RowsAffected, result.Error
}

// TimelinePoint is the traffic of one endpoint in one time step.
type TimelinePoint struct {
	At        time.Time `json:"at"`
	Requests  int64     `json:"requests"`
	Errors    int64     `json:"errors"`
	ErrorRate float64   `json:"error_rate"`
}

// GetTimeline returns the traffic of an endpoint from `from` to `to`, one
// point per `step`. Steps without traffic are included as zero, so a silence
// shows up as a gap you can see.
func GetTimeline(db *gorm.DB, apiKeyID uint, path string, from, to time.Time, step time.Duration) ([]TimelinePoint, error) {
	from, to = from.Truncate(step), to.Truncate(step)

	var stats []EndpointStat
	err := db.Where("api_key_id = ? AND path = ? AND bucket BETWEEN ? AND ?", apiKeyID, path, from, to.Add(step-time.Nanosecond)).
		Find(&stats).Error
	if err != nil {
		return nil, err
	}

	byStep := map[time.Time]EndpointStat{}
	for _, s := range stats {
		at := s.Bucket.UTC().Truncate(step)
		sum := byStep[at]
		sum.Total += s.Total
		sum.Errors += s.Errors
		byStep[at] = sum
	}

	var points []TimelinePoint
	for at := from; !at.After(to); at = at.Add(step) {
		s := byStep[at]
		points = append(points, TimelinePoint{At: at, Requests: s.Total, Errors: s.Errors, ErrorRate: s.ErrorRate()})
	}
	return points, nil
}
