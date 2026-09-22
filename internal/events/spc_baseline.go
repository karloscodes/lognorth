package events

import (
	"fmt"
	"math"
	"time"

	"gorm.io/gorm"
)

// GetOrCreateBaseline retrieves or creates a baseline for the given key.
func GetOrCreateBaseline(db *gorm.DB, apiKeyID uint, path string, hourOfWeek int) *BaselineStat {
	var baseline BaselineStat
	result := db.Where("api_key_id = ? AND path = ? AND hour_of_week = ?",
		apiKeyID, path, hourOfWeek).First(&baseline)

	if result.Error != nil {
		baseline = BaselineStat{
			APIKeyID:    apiKeyID,
			Path:        path,
			HourOfWeek:  hourOfWeek,
			Mean:        DefaultMean,
			StdDev:      DefaultStdDev,
			SampleCount: 0,
			UpdatedAt:   time.Now(),
		}
		db.Create(&baseline)
	}

	return &baseline
}

// UpdateBaseline updates the baseline with a new observation using EMA.
func UpdateBaseline(db *gorm.DB, baseline *BaselineStat, newValue float64) {
	// Exponential moving average for mean
	baseline.Mean = (1-BaselineSmoothingFactor)*baseline.Mean + BaselineSmoothingFactor*newValue

	// Welford's online algorithm for stddev (simplified EMA version)
	diff := newValue - baseline.Mean
	baseline.StdDev = math.Sqrt(
		(1-BaselineSmoothingFactor)*baseline.StdDev*baseline.StdDev +
			BaselineSmoothingFactor*diff*diff,
	)

	baseline.SampleCount++
	baseline.UpdatedAt = time.Now()

	db.Save(baseline)
}

// GetEffectiveBaseline returns the mean and stddev to use.
// Uses defaults during cold start (insufficient samples).
func GetEffectiveBaseline(baseline *BaselineStat) (mean, stddev float64) {
	if baseline.SampleCount < MinSamplesForBaseline {
		return DefaultMean, DefaultStdDev
	}
	return baseline.Mean, baseline.StdDev
}

// GetActiveEndpoints returns all endpoints with recent activity.
func GetActiveEndpoints(db *gorm.DB, apiKeyID uint) []string {
	since := time.Now().Add(-1 * time.Hour)

	var paths []string
	db.Model(&EndpointStat{}).
		Where("api_key_id = ? AND bucket > ?", apiKeyID, since).
		Distinct("path").
		Pluck("path", &paths)

	return paths
}

// EndpointAlertStatus represents the alerting status for an endpoint.
type EndpointAlertStatus struct {
	Path         string  `json:"path"`
	Status       string  `json:"status"`        // "learning", "healthy", "warning", "critical"
	SampleCount  int     `json:"sample_count"`  // Progress toward MinSamplesForBaseline
	SamplesNeeded int    `json:"samples_needed"` // MinSamplesForBaseline
	BaselineMean float64 `json:"baseline_mean,omitempty"`
	BaselineStdDev float64 `json:"baseline_stddev,omitempty"`
	CurrentRate  float64 `json:"current_rate,omitempty"`
	SinceSpiking string  `json:"since_spiking,omitempty"` // e.g., "2h 15m"
}

// GetEndpointAlertStatuses returns alerting status for all endpoints of an app.
func GetEndpointAlertStatuses(db *gorm.DB, apiKeyID uint) []EndpointAlertStatus {
	hourOfWeek := HourOfWeek(LastCompleteBucketTime())

	// Get all endpoints with any baseline data for this app
	var baselines []BaselineStat
	db.Where("api_key_id = ?", apiKeyID).
		Group("path").
		Find(&baselines)

	// Build a map for quick lookup
	baselineByPath := make(map[string]*BaselineStat)
	for i := range baselines {
		baselineByPath[baselines[i].Path] = &baselines[i]
	}

	// Get all spike states for this app
	var spikeStates []SpikeState
	db.Where("api_key_id = ?", apiKeyID).Find(&spikeStates)

	spikeByPath := make(map[string]*SpikeState)
	for i := range spikeStates {
		spikeByPath[spikeStates[i].Path] = &spikeStates[i]
	}

	// Get active endpoints (recent traffic)
	paths := GetActiveEndpoints(db, apiKeyID)

	var statuses []EndpointAlertStatus
	for _, path := range paths {
		status := EndpointAlertStatus{
			Path:          path,
			SamplesNeeded: MinSamplesForBaseline,
		}

		// Get baseline for current hour
		baseline := GetOrCreateBaseline(db, apiKeyID, path, hourOfWeek)
		status.SampleCount = baseline.SampleCount

		// Get current error rate
		stat, _ := GetLastCompleteBucket(db, apiKeyID, path)
		status.CurrentRate = stat.ErrorRate()

		// Check spike state
		if spike, ok := spikeByPath[path]; ok && spike.Severity != "" {
			status.Status = spike.Severity
			if !spike.SpikeStartAt.IsZero() {
				status.SinceSpiking = formatAlertDuration(time.Since(spike.SpikeStartAt))
			}
		} else if baseline.SampleCount < MinSamplesForBaseline {
			status.Status = "learning"
		} else {
			status.Status = "healthy"
			status.BaselineMean = math.Round(baseline.Mean*10) / 10
			status.BaselineStdDev = math.Round(baseline.StdDev*10) / 10
		}

		statuses = append(statuses, status)
	}

	return statuses
}

func formatAlertDuration(d time.Duration) string {
	if d < time.Minute {
		return "just now"
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	hours := int(d.Hours())
	mins := int(d.Minutes()) % 60
	if mins == 0 {
		return fmt.Sprintf("%dh", hours)
	}
	return fmt.Sprintf("%dh %dm", hours, mins)
}

// GetOrCreateVolumeBaseline retrieves or creates a volume baseline for the given key.
func GetOrCreateVolumeBaseline(db *gorm.DB, apiKeyID uint, path string, hourOfWeek int) *VolumeBaselineStat {
	var baseline VolumeBaselineStat
	result := db.Where("api_key_id = ? AND path = ? AND hour_of_week = ?",
		apiKeyID, path, hourOfWeek).First(&baseline)

	if result.Error != nil {
		baseline = VolumeBaselineStat{
			APIKeyID:    apiKeyID,
			Path:        path,
			HourOfWeek:  hourOfWeek,
			Mean:        0,
			StdDev:      0,
			SampleCount: 0,
			UpdatedAt:   time.Now(),
		}
		db.Create(&baseline)
	}

	return &baseline
}

// UpdateVolumeBaseline updates the volume baseline with a new observation using EMA.
func UpdateVolumeBaseline(db *gorm.DB, baseline *VolumeBaselineStat, newVolume float64) {
	baseline.Mean = (1-BaselineSmoothingFactor)*baseline.Mean + BaselineSmoothingFactor*newVolume

	diff := newVolume - baseline.Mean
	baseline.StdDev = math.Sqrt(
		(1-BaselineSmoothingFactor)*baseline.StdDev*baseline.StdDev +
			BaselineSmoothingFactor*diff*diff,
	)

	baseline.SampleCount++
	baseline.UpdatedAt = time.Now()

	db.Save(baseline)
}

// GetEffectiveVolumeBaseline returns the mean and stddev to use.
// Returns 0, 0 during cold start (insufficient samples) to skip alerting.
func GetEffectiveVolumeBaseline(baseline *VolumeBaselineStat) (mean, stddev float64) {
	if baseline.SampleCount < MinSamplesForBaseline {
		return 0, 0 // Not enough data, don't alert
	}
	return baseline.Mean, baseline.StdDev
}
