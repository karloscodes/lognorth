package events

import (
	"time"

	"gorm.io/gorm"
)

// GetOrCreateSpikeState retrieves or creates a spike state for the endpoint.
func GetOrCreateSpikeState(db *gorm.DB, apiKeyID uint, path string) *SpikeState {
	var state SpikeState
	result := db.Where("api_key_id = ? AND path = ?", apiKeyID, path).First(&state)

	if result.Error != nil {
		state = SpikeState{
			APIKeyID: apiKeyID,
			Path:     path,
		}
		db.Create(&state)
	}

	return &state
}

// GetOrCreateCUSUMState retrieves or creates a CUSUM state for the endpoint.
func GetOrCreateCUSUMState(db *gorm.DB, apiKeyID uint, path string) *CUSUMState {
	var state CUSUMState
	result := db.Where("api_key_id = ? AND path = ?", apiKeyID, path).First(&state)

	if result.Error != nil {
		state = CUSUMState{
			APIKeyID: apiKeyID,
			Path:     path,
		}
		db.Create(&state)
	}

	return &state
}

// UpdateSPCSeverity updates the spike state with new severity and error count.
// Handles escalation from warning to critical based on time and error count.
func UpdateSPCSeverity(db *gorm.DB, state *SpikeState, newSeverity string, errorsDelta int) {
	state.ErrorCount += errorsDelta

	// Check for escalation from warning to critical
	if state.Severity == "warning" && newSeverity != "critical" {
		timeSinceDetection := time.Since(state.SpikeStartAt)

		if timeSinceDetection > EscalationTime || state.ErrorCount > EscalationErrorCount {
			newSeverity = "critical"
		}
	}

	// First detection
	if state.Severity == "" && newSeverity != "" {
		state.SpikeStartAt = time.Now()
		state.IsSpiking = true
	}

	// Upgrade to critical
	if newSeverity == "critical" && state.Severity != "critical" {
		state.Severity = "critical"
	} else if newSeverity == "warning" && state.Severity == "" {
		state.Severity = "warning"
	}

	db.Save(state)
}

// ClearSPCState resets the spike state after recovery.
func ClearSPCState(db *gorm.DB, state *SpikeState) {
	state.Severity = ""
	state.ErrorCount = 0
	state.IsSpiking = false
	state.SpikeStartAt = time.Time{}
	db.Save(state)
}

// ClearCUSUMState resets the CUSUM state after recovery.
func ClearCUSUMState(db *gorm.DB, state *CUSUMState) {
	state.Cumulative = 0
	state.StartedAt = time.Time{}
	db.Save(state)
}

// GetUnnotifiedCriticalSpikes returns critical spikes that haven't been notified.
func GetUnnotifiedCriticalSpikes(db *gorm.DB) []SpikeState {
	var spikes []SpikeState
	db.Where("severity = ? AND (last_alert_at IS NULL OR last_alert_at < spike_start_at)",
		"critical").Find(&spikes)
	return spikes
}

// MarkSpikeNotified marks the spike as notified.
func MarkSpikeNotified(db *gorm.DB, state *SpikeState) {
	state.LastAlertAt = time.Now()
	db.Save(state)
}

// QueueDigestItem adds a warning to the daily digest queue.
func QueueDigestItem(db *gorm.DB, apiKeyID uint, path, message, severity string) {
	item := DigestItem{
		APIKeyID:  apiKeyID,
		Path:      path,
		Message:   message,
		Severity:  severity,
		CreatedAt: time.Now(),
	}
	db.Create(&item)
}

// GetPendingDigestItems returns all unresolved digest items.
func GetPendingDigestItems(db *gorm.DB) []DigestItem {
	var items []DigestItem
	db.Where("resolved = ?", false).Order("created_at desc").Find(&items)
	return items
}

// MarkDigestItemsResolved marks digest items as resolved.
func MarkDigestItemsResolved(db *gorm.DB, items []DigestItem) {
	for _, item := range items {
		db.Model(&item).Update("resolved", true)
	}
}

// GetOrCreateSilenceState retrieves or creates a silence state for the endpoint.
func GetOrCreateSilenceState(db *gorm.DB, apiKeyID uint, path string) *SilenceState {
	var state SilenceState
	result := db.Where("api_key_id = ? AND path = ?", apiKeyID, path).First(&state)

	if result.Error != nil {
		state = SilenceState{
			APIKeyID: apiKeyID,
			Path:     path,
		}
		db.Create(&state)
	}

	return &state
}

// UpdateSilenceSeverity updates the silence state with new severity.
func UpdateSilenceSeverity(db *gorm.DB, state *SilenceState, severity string) {
	if state.Severity == "" && severity != "" {
		state.SilenceStartAt = time.Now()
		state.IsSilent = true
	}

	// Only escalate, never downgrade (until recovery)
	if severity == "critical" && state.Severity != "critical" {
		state.Severity = "critical"
	} else if severity == "warning" && state.Severity == "" {
		state.Severity = "warning"
	}

	db.Save(state)
}

// ClearSilenceState resets silence state after recovery.
func ClearSilenceState(db *gorm.DB, state *SilenceState) {
	state.Severity = ""
	state.IsSilent = false
	state.SilenceStartAt = time.Time{}
	db.Save(state)
}

// GetUnnotifiedCriticalSilences returns critical silences that need alerts.
func GetUnnotifiedCriticalSilences(db *gorm.DB) []SilenceState {
	var silences []SilenceState
	db.Where("severity = ? AND (last_alert_at IS NULL OR last_alert_at < silence_start_at)",
		"critical").Find(&silences)
	return silences
}

// MarkSilenceNotified marks the silence as notified.
func MarkSilenceNotified(db *gorm.DB, state *SilenceState) {
	state.LastAlertAt = time.Now()
	db.Save(state)
}
