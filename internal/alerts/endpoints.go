package alerts

import (
	"fmt"
	"time"

	"gorm.io/gorm"

	"lognorth/internal/apikeys"
	"lognorth/internal/events"
	"lognorth/internal/settings"
)

// Spikes, drift, and silence of an endpoint: every 5 minutes, the last
// complete bucket of endpoint_stats against the endpoint's own history.

// CheckEndpoints checks every endpoint of every active app against its
// history at now, and records the critical alerts.
func CheckEndpoints(db *gorm.DB, now time.Time) error {
	var keys []apikeys.APIKey
	if err := db.Where("active = ?", true).Find(&keys).Error; err != nil {
		return err
	}

	at := events.LastCompleteBucket(now)
	for _, key := range keys {
		for _, path := range events.GetActiveEndpoints(db, key.ID, now) {
			checkEndpoint(db, key.ID, key.Name, path, at, now)
		}
	}
	return recordCriticalAlerts(db, now)
}

func checkEndpoint(db *gorm.DB, apiKeyID uint, appName, path string, at, now time.Time) {
	bucket, err := events.GetCompleteBucket(db, apiKeyID, path, at)
	if err != nil {
		return
	}

	checkSilence(db, apiKeyID, appName, path, at, bucket.Total, now)
	checkErrorRate(db, apiKeyID, appName, path, at, bucket, now)
}

func checkErrorRate(db *gorm.DB, apiKeyID uint, appName, path string, at time.Time, bucket events.EndpointStat, now time.Time) {
	baseline, err := GetErrorRateBaseline(db, apiKeyID, path, at)
	if err != nil {
		return
	}
	lastHour, err := lastHourWithoutSpikes(db, apiKeyID, path, at, baseline)
	if err != nil {
		return
	}
	last15, err := lastBuckets(db, apiKeyID, path, at, 3)
	if err != nil {
		return
	}

	_, spike := SPCIsSpike(bucket.ErrorRate(), bucket.Total, baseline)
	_, drift := SPCIsDrift(lastHour.ErrorRate(), lastHour.Total, baseline)
	drift = stillDrifting(drift, last15, baseline)
	severity := MaxSeverity(spike, drift)
	if smallEndpointFailing(bucket, last15, baseline) {
		severity = "critical"
	}

	state := GetOrCreateSpikeState(db, apiKeyID, path)
	previousSeverity := state.Severity
	UpdateSPCSeverity(db, state, severity, int(bucket.Errors), now)

	// Queue warning for digest if new warning
	if previousSeverity == "" && state.Severity == "warning" {
		msg := fmt.Sprintf("%s: %s error rate elevated to %.1f%% (normal: %.1f%%)",
			appName, path, lastHour.ErrorRate(), baseline.Rate)
		QueueDigestItem(db, apiKeyID, path, msg, "warning", now)
	}
}

func checkSilence(db *gorm.DB, apiKeyID uint, appName, path string, at time.Time, currentVolume int64, now time.Time) {
	baseline, err := GetVolumeBaseline(db, apiKeyID, path, at)
	if err != nil || !baseline.Ready() {
		return
	}

	_, severity := IsSilence(float64(currentVolume), baseline)

	state := GetOrCreateSilenceState(db, apiKeyID, path)
	previousSeverity := state.Severity
	UpdateSilenceSeverity(db, state, severity, now)

	// Queue warning for digest if new warning
	if previousSeverity == "" && state.Severity == "warning" {
		msg := fmt.Sprintf("%s: %s traffic dropped to %d requests (normally %.0f)",
			appName, path, currentVolume, baseline.Median)
		QueueDigestItem(db, apiKeyID, path, msg, "warning", now)
	}
}

// recordCriticalAlerts records an alert for each endpoint that turned
// critical, at most once per path per AlertCooldown. While its app is down,
// an endpoint's alert waits: the down alert already names the incident, and
// it names the errors that surged. If the endpoint still fails once the app
// is back, it alerts then.
func recordCriticalAlerts(db *gorm.DB, now time.Time) error {
	s, err := settings.Get(db)
	if err != nil {
		return err
	}
	at := events.LastCompleteBucket(now)

	for _, spike := range GetUnnotifiedCriticalSpikes(db, now) {
		key, err := apikeys.FindByID(db, spike.APIKeyID)
		if err != nil || key.IsDown {
			continue
		}
		baseline, _ := GetErrorRateBaseline(db, spike.APIKeyID, spike.Path, at)
		bucket, _ := events.GetCompleteBucket(db, spike.APIKeyID, spike.Path, at)
		currentRate := bucket.ErrorRate()
		if last15, err := lastBuckets(db, spike.APIKeyID, spike.Path, at, 3); err == nil && bucket.Total < MinRequestsForErrorRate {
			// A small endpoint: one bucket says little, so say the last 15 minutes.
			currentRate = last15.ErrorRate()
		}

		body := fmt.Sprintf(`%s is at %.1f%% errors — normally %.1f%%.
%d errors in the last %d min.

%s/apps/%d/issues

Ask your agent: /lognorth:investigate %s`,
			spike.Path,
			currentRate,
			baseline.Rate,
			spike.ErrorCount,
			int(now.Sub(spike.SpikeStartAt).Minutes())+5, // The first bad bucket started 5 min before detection
			s.BaseURL,
			spike.APIKeyID,
			spike.Path,
		)
		alert := &Alert{APIKeyID: spike.APIKeyID, Kind: KindSpike, Title: fmt.Sprintf("spike: %s at %.1f%%", spike.Path, currentRate), Body: body}
		if err := Record(db, now, alert); err != nil {
			return err
		}
		MarkSpikeNotified(db, &spike, now)
	}

	for _, silence := range GetUnnotifiedCriticalSilences(db, now) {
		key, err := apikeys.FindByID(db, silence.APIKeyID)
		if err != nil || key.IsDown {
			continue
		}
		baseline, _ := GetVolumeBaseline(db, silence.APIKeyID, silence.Path, at)

		silentMin := int(now.Sub(silence.SilenceStartAt).Minutes()) + 5
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
		alert := &Alert{APIKeyID: silence.APIKeyID, Kind: KindSilence, Title: fmt.Sprintf("silent: %s for %d min", silence.Path, silentMin), Body: body}
		if err := Record(db, now, alert); err != nil {
			return err
		}
		MarkSilenceNotified(db, &silence, now)
	}
	return nil
}

// stillDrifting keeps a drift only while the last 15 minutes are still above
// the normal rate, and a critical one only while they are past the floor a
// critical rate needs. The last hour holds the past: after a 30-minute outage
// it stays high for another half hour, though the endpoint is fine again.
func stillDrifting(drift string, last15 events.EndpointStat, baseline ErrorRateBaseline) string {
	now := last15.ErrorRate()
	switch {
	case drift == "" || now <= baseline.HourRate:
		return ""
	case drift == "critical" && now < max(CriticalMinRatio*baseline.HourRate, baseline.HourRate+CriticalMinPoints):
		return "warning"
	}
	return drift
}

// smallEndpointFailing is the plain rule for an endpoint with too little
// traffic for the spike check: in the last 15 minutes at least
// smallEndpointErrors errors, and half the requests failed; the bucket just
// checked fails as much, so the endpoint fails now; and its history fails
// less than smallEndpointNormal.
func smallEndpointFailing(bucket, last15 events.EndpointStat, baseline ErrorRateBaseline) bool {
	return bucket.Total > 0 && bucket.Total < MinRequestsForErrorRate &&
		bucket.ErrorRate() >= smallEndpointFailRate &&
		last15.Errors >= smallEndpointErrors &&
		last15.ErrorRate() >= smallEndpointFailRate &&
		baseline.Requests >= MinRequestsForErrorRate &&
		baseline.Rate < smallEndpointNormal
}
