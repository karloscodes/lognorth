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

	hourOfWeek := events.HourOfWeek(time.Now())

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
	// Get current error rate
	currentRate, _, err := events.GetCurrentErrorRate(db, apiKeyID, path)
	if err != nil {
		return
	}

	// Get baseline
	baseline := events.GetOrCreateBaseline(db, apiKeyID, path, hourOfWeek)
	mean, stddev := events.GetEffectiveBaseline(baseline)

	// Control Chart check (spikes)
	_, spikeSeverity := events.SPCIsSpike(currentRate, mean, stddev)

	// Get CUSUM state for drift detection
	cusumState := events.GetOrCreateCUSUMState(db, apiKeyID, path)

	// CUSUM check (drift)
	_, driftSeverity := events.UpdateCUSUM(cusumState, currentRate, mean)
	db.Save(cusumState)

	// Get spike state for alert tracking
	state := events.GetOrCreateSpikeState(db, apiKeyID, path)

	// Take worst severity
	severity := events.MaxSeverity(spikeSeverity, driftSeverity)

	// Check for recovery
	if state.Severity != "" && severity == "" {
		if events.SPCIsRecovered(currentRate, mean, stddev) {
			j.handleRecovery(db, state, cusumState)
			return
		}
	}

	// Update alert state
	if severity != "" {
		// Estimate errors in this bucket
		errorsDelta := int(currentRate / 100 * 10) // Rough estimate

		previousSeverity := state.Severity
		events.UpdateSPCSeverity(db, state, severity, errorsDelta)

		// Queue warning for digest if new warning
		if previousSeverity == "" && state.Severity == "warning" {
			msg := fmt.Sprintf("%s: %s error rate elevated to %.1f%% (normal: %.1f%%)",
				appName, path, currentRate, mean)
			events.QueueDigestItem(db, apiKeyID, path, msg, "warning")
		}
	}
}

func (j *AnomalyChecker) handleRecovery(db *gorm.DB, state *events.SpikeState, cusumState *events.CUSUMState) {
	// Clear state, no notification needed
	// Recovery is the absence of a problem - you don't need to be told things are fine
	events.ClearSPCState(db, state)
	events.ClearCUSUMState(db, cusumState)
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

		// Get baseline for context
		hourOfWeek := events.HourOfWeek(time.Now())
		baseline := events.GetOrCreateBaseline(db, spike.APIKeyID, spike.Path, hourOfWeek)
		mean, stddev := events.GetEffectiveBaseline(baseline)

		currentRate, _, _ := events.GetCurrentErrorRate(db, spike.APIKeyID, spike.Path)

		var zScore float64
		if stddev > 0 {
			zScore = (currentRate - mean) / stddev
		}

		subject := fmt.Sprintf("[CRITICAL] %s: Error rate spike detected", key.Name)
		body := fmt.Sprintf(`%s is experiencing abnormal error rates.

What's happening:
- Endpoint: %s
- Current error rate: %.1f%% (normal: %.1f%%)
- Started: %s ago
- Errors so far: %d

This is %.1f standard deviations above normal for this time of day.

View details: %s/apps/%d/issues`,
			key.Name,
			spike.Path,
			currentRate,
			mean,
			time.Since(spike.SpikeStartAt).Round(time.Minute),
			spike.ErrorCount,
			zScore,
			s.BaseURL,
			spike.APIKeyID,
		)

		if err := s.SendEmail(subject, body); err == nil {
			events.MarkSpikeNotified(db, &spike)
		}
	}
}
