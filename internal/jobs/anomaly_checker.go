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

// AnomalyChecker runs SPC detection and sends critical alerts.
// Every check compares the last complete bucket with the stored history in
// endpoint_stats, so there is no baseline to learn or keep.
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

	at := events.LastCompleteBucketTime()

	for _, key := range keys {
		for _, path := range events.GetActiveEndpoints(db, key.ID) {
			j.checkEndpoint(db, key.ID, key.Name, path, at)
		}
	}

	j.sendCriticalAlerts(db, at)

	return nil
}

func (j *AnomalyChecker) checkEndpoint(db *gorm.DB, apiKeyID uint, appName, path string, at time.Time) {
	bucket, err := events.GetLastCompleteBucket(db, apiKeyID, path)
	if err != nil {
		return
	}

	j.checkSilence(db, apiKeyID, appName, path, at, bucket.Total)
	j.checkErrorRate(db, apiKeyID, appName, path, at, bucket)
}

func (j *AnomalyChecker) checkErrorRate(db *gorm.DB, apiKeyID uint, appName, path string, at time.Time, bucket events.EndpointStat) {
	baseline, err := events.GetErrorRateBaseline(db, apiKeyID, path, at)
	if err != nil || !baseline.Ready() {
		return
	}
	lastHour, err := events.GetLastHour(db, apiKeyID, path, at)
	if err != nil {
		return
	}

	_, spike := events.SPCIsSpike(bucket.ErrorRate(), bucket.Total, baseline)
	_, drift := events.SPCIsDrift(lastHour.ErrorRate(), lastHour.Total, baseline)

	state := events.GetOrCreateSpikeState(db, apiKeyID, path)
	previousSeverity := state.Severity
	events.UpdateSPCSeverity(db, state, events.MaxSeverity(spike, drift), int(bucket.Errors))

	// Queue warning for digest if new warning
	if previousSeverity == "" && state.Severity == "warning" {
		msg := fmt.Sprintf("%s: %s error rate elevated to %.1f%% (normal: %.1f%%)",
			appName, path, lastHour.ErrorRate(), baseline.Rate)
		events.QueueDigestItem(db, apiKeyID, path, msg, "warning")
	}
}

func (j *AnomalyChecker) checkSilence(db *gorm.DB, apiKeyID uint, appName, path string, at time.Time, currentVolume int64) {
	baseline, err := events.GetVolumeBaseline(db, apiKeyID, path, at)
	if err != nil || !baseline.Ready() {
		return
	}

	_, severity := events.IsSilence(float64(currentVolume), baseline)

	state := events.GetOrCreateSilenceState(db, apiKeyID, path)
	previousSeverity := state.Severity
	events.UpdateSilenceSeverity(db, state, severity)

	// Queue warning for digest if new warning
	if previousSeverity == "" && state.Severity == "warning" {
		msg := fmt.Sprintf("%s: %s traffic dropped to %d requests (normally %.0f)",
			appName, path, currentVolume, baseline.Median)
		events.QueueDigestItem(db, apiKeyID, path, msg, "warning")
	}
}

func (j *AnomalyChecker) sendCriticalAlerts(db *gorm.DB, at time.Time) {
	s, err := settings.Get(db)
	if err != nil || !s.CanNotify() {
		return
	}

	for _, spike := range events.GetUnnotifiedCriticalSpikes(db) {
		var key apikeys.APIKey
		db.First(&key, spike.APIKeyID)

		baseline, _ := events.GetErrorRateBaseline(db, spike.APIKeyID, spike.Path, at)
		bucket, _ := events.GetLastCompleteBucket(db, spike.APIKeyID, spike.Path)
		currentRate := bucket.ErrorRate()

		subject := fmt.Sprintf("%s — spike: %s at %.1f%%", key.Name, spike.Path, currentRate)
		body := fmt.Sprintf(`%s is at %.1f%% errors — normally %.1f%%.
%d errors in the last %d min.

%s/apps/%d/issues

Ask your agent: /lognorth:investigate %s`,
			spike.Path,
			currentRate,
			baseline.Rate,
			spike.ErrorCount,
			int(time.Since(spike.SpikeStartAt).Minutes())+5, // The first bad bucket started 5 min before detection
			s.BaseURL,
			spike.APIKeyID,
			spike.Path,
		)

		if err := s.SendNotification(subject, body); err == nil {
			events.MarkSpikeNotified(db, &spike)
		}
	}

	for _, silence := range events.GetUnnotifiedCriticalSilences(db) {
		var key apikeys.APIKey
		db.First(&key, silence.APIKeyID)

		baseline, _ := events.GetVolumeBaseline(db, silence.APIKeyID, silence.Path, at)

		silentMin := int(time.Since(silence.SilenceStartAt).Minutes()) + 5
		subject := fmt.Sprintf("%s — silent: %s for %d min", key.Name, silence.Path, silentMin)
		body := fmt.Sprintf(`%s has been silent for %d min.
Normally ~%.0f requests.

%s/apps/%d/issues

Ask your agent: /lognorth:investigate %s`,
			silence.Path,
			silentMin,
			baseline.Median,
			s.BaseURL,
			silence.APIKeyID,
			silence.Path,
		)

		if err := s.SendNotification(subject, body); err == nil {
			events.MarkSilenceNotified(db, &silence)
		}
	}
}
