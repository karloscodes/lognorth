package alerts

import (
	"fmt"
	"lognorth/internal/apikeys"
	"lognorth/internal/events"
	"lognorth/internal/settings"
	"time"

	"gorm.io/gorm"
)

// Spikes, drift, and silence of an endpoint: every 5 minutes, the last
// complete bucket of endpoint_stats against the endpoint's own history.

// CheckEndpoints checks every endpoint of every active app against its
// history, and sends the critical alerts.
func CheckEndpoints(db *gorm.DB) error {

	// Get all active API keys
	var keys []apikeys.APIKey
	if err := db.Where("active = ?", true).Find(&keys).Error; err != nil {
		return err
	}

	at := events.LastCompleteBucketTime()

	for _, key := range keys {
		for _, path := range events.GetActiveEndpoints(db, key.ID) {
			checkEndpoint(db, key.ID, key.Name, path, at)
		}
	}

	sendCriticalAlerts(db, at)

	return nil
}

func checkEndpoint(db *gorm.DB, apiKeyID uint, appName, path string, at time.Time) {
	bucket, err := events.GetLastCompleteBucket(db, apiKeyID, path)
	if err != nil {
		return
	}

	checkSilence(db, apiKeyID, appName, path, at, bucket.Total)
	checkErrorRate(db, apiKeyID, appName, path, at, bucket)
}

func checkErrorRate(db *gorm.DB, apiKeyID uint, appName, path string, at time.Time, bucket events.EndpointStat) {
	baseline, err := GetErrorRateBaseline(db, apiKeyID, path, at)
	if err != nil || !baseline.Ready() {
		return
	}
	lastHour, err := GetLastHour(db, apiKeyID, path, at)
	if err != nil {
		return
	}

	_, spike := SPCIsSpike(bucket.ErrorRate(), bucket.Total, baseline)
	_, drift := SPCIsDrift(lastHour.ErrorRate(), lastHour.Total, baseline)

	state := GetOrCreateSpikeState(db, apiKeyID, path)
	previousSeverity := state.Severity
	UpdateSPCSeverity(db, state, MaxSeverity(spike, drift), int(bucket.Errors))

	// Queue warning for digest if new warning
	if previousSeverity == "" && state.Severity == "warning" {
		msg := fmt.Sprintf("%s: %s error rate elevated to %.1f%% (normal: %.1f%%)",
			appName, path, lastHour.ErrorRate(), baseline.Rate)
		QueueDigestItem(db, apiKeyID, path, msg, "warning")
	}
}

func checkSilence(db *gorm.DB, apiKeyID uint, appName, path string, at time.Time, currentVolume int64) {
	baseline, err := GetVolumeBaseline(db, apiKeyID, path, at)
	if err != nil || !baseline.Ready() {
		return
	}

	_, severity := IsSilence(float64(currentVolume), baseline)

	state := GetOrCreateSilenceState(db, apiKeyID, path)
	previousSeverity := state.Severity
	UpdateSilenceSeverity(db, state, severity)

	// Queue warning for digest if new warning
	if previousSeverity == "" && state.Severity == "warning" {
		msg := fmt.Sprintf("%s: %s traffic dropped to %d requests (normally %.0f)",
			appName, path, currentVolume, baseline.Median)
		QueueDigestItem(db, apiKeyID, path, msg, "warning")
	}
}

func sendCriticalAlerts(db *gorm.DB, at time.Time) {
	s, err := settings.Get(db)
	if err != nil || !CanNotify(s) {
		return
	}

	for _, spike := range GetUnnotifiedCriticalSpikes(db) {
		var key apikeys.APIKey
		db.First(&key, spike.APIKeyID)

		baseline, _ := GetErrorRateBaseline(db, spike.APIKeyID, spike.Path, at)
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

		if err := Send(s, subject, body); err == nil {
			MarkSpikeNotified(db, &spike)
		}
	}

	for _, silence := range GetUnnotifiedCriticalSilences(db) {
		var key apikeys.APIKey
		db.First(&key, silence.APIKeyID)

		baseline, _ := GetVolumeBaseline(db, silence.APIKeyID, silence.Path, at)

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

		if err := Send(s, subject, body); err == nil {
			MarkSilenceNotified(db, &silence)
		}
	}
}
