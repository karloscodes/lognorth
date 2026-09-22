package events

import "time"

// SPC Configuration Constants
const (
	// Error rate checks need this many requests in a bucket.
	// With fewer, one error moves the rate by 5% or more.
	MinRequestsForErrorRate = 20

	// History: the same time of day on earlier days (error rate) or the same
	// time of week on earlier weeks (volume), ± HistoryWindow.
	HistoryWindow        = 30 * time.Minute
	ErrorRateHistoryDays = 7
	VolumeHistoryWeeks   = 4
	MinHistoryBuckets    = 12 // History buckets needed before a check runs

	// Thresholds (z-scores). The job checks every endpoint every 5 minutes,
	// 288 times a day. At 3σ, 0.13% of normal checks would be critical: about
	// one false email per endpoint every 3 days. At 5σ it is about one a century.
	WarningSigma  = 3.0 // Queued for the daily digest
	CriticalSigma = 5.0 // Emailed now

	// Recovery
	RecoveryChecks = 2         // Clean checks in a row before an alert clears
	AlertCooldown  = time.Hour // Minimum time between two emails for the same path

	// Silence detection
	MinVolumeForSilenceAlert = 50.0 // Don't alert on endpoints with <50 requests/5min baseline
)
