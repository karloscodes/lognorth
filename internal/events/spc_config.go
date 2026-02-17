package events

import "time"

// SPC Configuration Constants
const (
	// Baseline learning
	BaselineSmoothingFactor = 0.1  // EMA weight for new data (10%)
	MinSamplesForBaseline   = 20   // Samples needed before trusting baseline
	DefaultMean             = 2.0  // Default error rate % during cold start
	DefaultStdDev           = 2.0  // Conservative stddev during cold start

	// Control Chart thresholds (z-scores)
	WarningSigma  = 2.0 // 95% confidence
	CriticalSigma = 3.0 // 99.7% confidence

	// CUSUM thresholds
	CUSUMSlack             = 0.5 // Acceptable drift before accumulating
	CUSUMWarningThreshold  = 2.5 // Accumulated deviation for warning
	CUSUMCriticalThreshold = 5.0 // Accumulated deviation for critical

	// Escalation rules
	EscalationTime       = 10 * time.Minute // Warning → Critical if persists
	EscalationErrorCount = 20               // Warning → Critical if errors exceed

	// Recovery
	RecoverySigma    = 1.0              // Below mean + 1σ to recover
	RecoveryDuration = 10 * time.Minute // Must stay recovered this long

	// Silence detection
	MinVolumeForSilenceAlert = 50.0 // Don't alert on endpoints with <50 requests/5min baseline
	SilenceWarningSigma      = -2.0 // Alert at -2σ
	SilenceCriticalSigma     = -3.0 // Critical at -3σ

	// Jobs
	BaselineUpdateInterval = 5 * time.Minute
	AnomalyCheckInterval   = 5 * time.Minute
	DefaultDigestHour      = 9 // 9am local time
)

// HourOfWeek returns 0-167 for the current hour-of-week.
// Monday 00:00 = 0, Sunday 23:00 = 167.
func HourOfWeek(t time.Time) int {
	weekday := int(t.Weekday())
	// Convert Sunday=0 to Monday=0 based week
	if weekday == 0 {
		weekday = 6 // Sunday becomes 6
	} else {
		weekday-- // Mon=0, Tue=1, etc.
	}
	return weekday*24 + t.Hour()
}
