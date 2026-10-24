package alerts

import (
	"fmt"
	"lognorth/internal/events"
	"slices"
	"time"

	"gorm.io/gorm"
)

// Alert is one endpoint that is alerting now, as an agent or an email reads it.
type Alert struct {
	AppID    uint      `json:"app_id"`
	Path     string    `json:"path"`
	Kind     string    `json:"kind"`     // "error_rate" or "silence"
	Severity string    `json:"severity"` // "warning" or "critical"
	Since    time.Time `json:"since"`
	Current  float64   `json:"current"` // error_rate: % errors in the last 5 min. silence: requests in the last 5 min
	Normal   float64   `json:"normal"`  // The same measure at this time on earlier days (error rate) or weeks (volume)
	Summary  string    `json:"summary"`
}

// ListAlerts returns the endpoints alerting now, critical first.
// An apiKeyID of 0 means every app.
func ListAlerts(db *gorm.DB, apiKeyID uint) ([]Alert, error) {
	at := events.LastCompleteBucketTime()
	scope := func(q *gorm.DB) *gorm.DB {
		if apiKeyID > 0 {
			return q.Where("api_key_id = ?", apiKeyID)
		}
		return q
	}

	var spikes []SpikeState
	if err := scope(db.Where("severity <> ''")).Find(&spikes).Error; err != nil {
		return nil, err
	}
	var silences []SilenceState
	if err := scope(db.Where("severity <> ''")).Find(&silences).Error; err != nil {
		return nil, err
	}

	alerts := []Alert{}
	for _, s := range spikes {
		baseline, _ := GetErrorRateBaseline(db, s.APIKeyID, s.Path, at)
		bucket, _ := events.GetLastCompleteBucket(db, s.APIKeyID, s.Path)
		lastHour, _ := GetLastHour(db, s.APIKeyID, s.Path, at)
		alerts = append(alerts, Alert{
			AppID:    s.APIKeyID,
			Path:     s.Path,
			Kind:     "error_rate",
			Severity: s.Severity,
			Since:    s.SpikeStartAt,
			Current:  bucket.ErrorRate(),
			Normal:   baseline.Rate,
			Summary: fmt.Sprintf("%.1f%% errors in the last 5 min and %.1f%% in the last hour, normally %.1f%%. %d errors since it started.",
				bucket.ErrorRate(), lastHour.ErrorRate(), baseline.Rate, s.ErrorCount),
		})
	}
	for _, s := range silences {
		baseline, _ := GetVolumeBaseline(db, s.APIKeyID, s.Path, at)
		bucket, _ := events.GetLastCompleteBucket(db, s.APIKeyID, s.Path)
		alerts = append(alerts, Alert{
			AppID:    s.APIKeyID,
			Path:     s.Path,
			Kind:     "silence",
			Severity: s.Severity,
			Since:    s.SilenceStartAt,
			Current:  float64(bucket.Total),
			Normal:   baseline.Median,
			Summary: fmt.Sprintf("%d requests in the last 5 min, normally about %.0f.",
				bucket.Total, baseline.Median),
		})
	}

	slices.SortStableFunc(alerts, func(a, b Alert) int {
		if a.Severity != b.Severity {
			if a.Severity == "critical" {
				return -1
			}
			return 1
		}
		return a.Since.Compare(b.Since)
	})
	return alerts, nil
}
