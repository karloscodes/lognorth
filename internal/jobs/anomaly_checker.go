package jobs

import (
	"fmt"
	"time"

	"github.com/karloscodes/cartridge"
	"gorm.io/gorm"

	"lognorth/internal/apikeys"
	"lognorth/internal/events"
	"lognorth/internal/settings"
)

// AnomalyChecker runs SPC detection, learns baselines, and sends critical alerts.
// Detection and learning run in one job so the order is fixed: check a bucket
// against the baseline first, then learn from it.
type AnomalyChecker struct{}

// NewAnomalyChecker creates a new anomaly checker.
func NewAnomalyChecker() *AnomalyChecker {
	return &AnomalyChecker{}
}

// ProcessBatch checks all endpoints for anomalies and sends alerts.
func (j *AnomalyChecker) ProcessBatch(ctx *cartridge.JobContext) error {
	db := ctx.DB

	// Get all active API keys
	var keys []apikeys.APIKey
	if err := db.Where("active = ?", true).Find(&keys).Error; err != nil {
		return err
	}

	hourOfWeek := events.HourOfWeek(events.LastCompleteBucketTime())

	for _, key := range keys {
		endpoints := events.GetActiveEndpoints(db, key.ID)

		for _, path := range endpoints {
			j.checkEndpoint(db, key.ID, key.Name, path, hourOfWeek)
		}
	}

	// Send critical alerts
	j.sendCriticalAlerts(db)

	return nil
}

func (j *AnomalyChecker) checkEndpoint(db *gorm.DB, apiKeyID uint, appName, path string, hourOfWeek int) {
	stat, err := events.GetLastCompleteBucket(db, apiKeyID, path)
	if err != nil {
		return
	}

	j.checkSilence(db, apiKeyID, appName, path, hourOfWeek, stat.Total)
	j.checkErrorRate(db, apiKeyID, appName, path, hourOfWeek, stat)
}

func (j *AnomalyChecker) checkErrorRate(db *gorm.DB, apiKeyID uint, appName, path string, hourOfWeek int, stat events.EndpointStat) {
	// Too few requests to trust the rate: do not check it, and do not learn from it.
	if stat.Total < events.MinRequestsForErrorRate {
		return
	}
	currentRate := stat.ErrorRate()

	// Get baseline
	baseline := events.GetOrCreateBaseline(db, apiKeyID, path, hourOfWeek)
	mean, stddev := events.GetEffectiveBaseline(baseline)

	// Control Chart check (spikes)
	_, spikeSeverity := events.SPCIsSpike(currentRate, mean, stddev, stat.Total)

	// Get CUSUM state for drift detection
	cusumState := events.GetOrCreateCUSUMState(db, apiKeyID, path)

	// CUSUM check (drift)
	_, driftSeverity := events.UpdateCUSUM(cusumState, currentRate, mean)
	db.Save(cusumState)

	// Get spike state for alert tracking
	state := events.GetOrCreateSpikeState(db, apiKeyID, path)

	// Take worst severity
	severity := events.MaxSeverity(spikeSeverity, driftSeverity)

	if state.Severity != "" && severity == "" {
		if events.SPCIsRecovered(currentRate, mean, stddev, stat.Total) {
			j.handleRecovery(db, state, cusumState)
		}
	} else if severity != "" {
		previousSeverity := state.Severity
		events.UpdateSPCSeverity(db, state, severity, int(stat.Errors))

		// Queue warning for digest if new warning
		if previousSeverity == "" && state.Severity == "warning" {
			msg := fmt.Sprintf("%s: %s error rate elevated to %.1f%% (normal: %.1f%%)",
				appName, path, currentRate, mean)
			events.QueueDigestItem(db, apiKeyID, path, msg, "warning")
		}
	}

	// Learn only from normal buckets, so a spike does not raise the baseline.
	// During cold start the defaults are not a real baseline, so always learn.
	if state.Severity == "" || baseline.SampleCount < events.MinSamplesForBaseline {
		events.UpdateBaseline(db, baseline, currentRate)
	}
}

func (j *AnomalyChecker) handleRecovery(db *gorm.DB, state *events.SpikeState, cusumState *events.CUSUMState) {
	// Clear state, no notification needed
	// Recovery is the absence of a problem - you don't need to be told things are fine
	events.ClearSPCState(db, state)
	events.ClearCUSUMState(db, cusumState)
}

func (j *AnomalyChecker) checkSilence(db *gorm.DB, apiKeyID uint, appName, path string, hourOfWeek int, currentVolume int64) {
	baseline := events.GetOrCreateVolumeBaseline(db, apiKeyID, path, hourOfWeek)
	state := events.GetOrCreateSilenceState(db, apiKeyID, path)

	// A mean of 0 means not enough data yet
	if mean, stddev := events.GetEffectiveVolumeBaseline(baseline); mean > 0 {
		_, severity := events.IsSilence(float64(currentVolume), mean, stddev)

		if state.Severity != "" && severity == "" {
			if events.IsSilenceRecovered(float64(currentVolume), mean, stddev) {
				events.ClearSilenceState(db, state)
			}
		} else if severity != "" {
			previousSeverity := state.Severity
			events.UpdateSilenceSeverity(db, state, severity)

			// Queue warning for digest if new warning
			if previousSeverity == "" && state.Severity == "warning" {
				msg := fmt.Sprintf("%s: %s traffic dropped to %d requests (normally %.0f)",
					appName, path, currentVolume, mean)
				events.QueueDigestItem(db, apiKeyID, path, msg, "warning")
			}
		}
	}

	// Learn only from normal traffic, so an outage does not lower the baseline.
	if state.Severity == "" {
		events.UpdateVolumeBaseline(db, baseline, float64(currentVolume))
	}
}

func (j *AnomalyChecker) sendCriticalAlerts(db *gorm.DB) {
	s, err := settings.Get(db)
	if err != nil || !s.CanNotify() {
		return
	}

	spikes := events.GetUnnotifiedCriticalSpikes(db)

	for _, spike := range spikes {
		// Get app name
		var key apikeys.APIKey
		db.First(&key, spike.APIKeyID)

		// Get baseline for context (mean only — the simplified email copy
		// dropped the z-score, so stddev isn't used here anymore).
		hourOfWeek := events.HourOfWeek(events.LastCompleteBucketTime())
		baseline := events.GetOrCreateBaseline(db, spike.APIKeyID, spike.Path, hourOfWeek)
		mean, _ := events.GetEffectiveBaseline(baseline)

		stat, _ := events.GetLastCompleteBucket(db, spike.APIKeyID, spike.Path)
		currentRate := stat.ErrorRate()

		subject := fmt.Sprintf("%s — spike: %s at %.1f%%", key.Name, spike.Path, currentRate)
		body := fmt.Sprintf(`%s is at %.1f%% errors — normally %.1f%%.
%d errors in the last %d min.

%s/apps/%d/issues`,
			spike.Path,
			currentRate,
			mean,
			spike.ErrorCount,
			int(time.Since(spike.SpikeStartAt).Minutes()),
			s.BaseURL,
			spike.APIKeyID,
		)

		if err := s.SendNotification(subject, body); err == nil {
			events.MarkSpikeNotified(db, &spike)
		}
	}

	// Send silence alerts
	j.sendSilenceAlerts(db, s)
}

func (j *AnomalyChecker) sendSilenceAlerts(db *gorm.DB, s *settings.Settings) {
	silences := events.GetUnnotifiedCriticalSilences(db)

	for _, silence := range silences {
		var key apikeys.APIKey
		db.First(&key, silence.APIKeyID)

		hourOfWeek := events.HourOfWeek(events.LastCompleteBucketTime())
		baseline := events.GetOrCreateVolumeBaseline(db, silence.APIKeyID, silence.Path, hourOfWeek)
		mean, _ := events.GetEffectiveVolumeBaseline(baseline)

		silentMin := int(time.Since(silence.SilenceStartAt).Minutes())
		subject := fmt.Sprintf("%s — silent: %s for %d min", key.Name, silence.Path, silentMin)
		body := fmt.Sprintf(`%s has been silent for %d min.
Normally ~%.0f requests.

%s/apps/%d/issues`,
			silence.Path,
			silentMin,
			mean,
			s.BaseURL,
			silence.APIKeyID,
		)

		if err := s.SendNotification(subject, body); err == nil {
			events.MarkSilenceNotified(db, &silence)
		}
	}
}
