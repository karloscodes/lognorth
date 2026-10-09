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

// UpdateSPCSeverity applies one check result to the spike state.
// An alert only escalates while active. It clears after RecoveryChecks clean checks in a row.
func UpdateSPCSeverity(db *gorm.DB, state *SpikeState, severity string, errors int) {
	if severity == "" {
		if state.Severity != "" {
			state.CleanChecks++
			if state.CleanChecks >= RecoveryChecks {
				state.Severity = ""
				state.ErrorCount = 0
				state.CleanChecks = 0
				state.SpikeStartAt = time.Time{}
			}
			db.Save(state)
		}
		return
	}

	if state.Severity == "" {
		state.SpikeStartAt = time.Now().UTC()
	}
	state.Severity = MaxSeverity(state.Severity, severity)
	state.ErrorCount += errors
	state.CleanChecks = 0
	db.Save(state)
}

// GetUnnotifiedCriticalSpikes returns critical spikes that need an email.
// A path gets at most one email per AlertCooldown, so a flapping endpoint does not flood the inbox.
func GetUnnotifiedCriticalSpikes(db *gorm.DB) []SpikeState {
	var spikes []SpikeState
	db.Where("severity = ? AND last_alert_at < spike_start_at AND last_alert_at < ?",
		"critical", time.Now().UTC().Add(-AlertCooldown)).Find(&spikes)
	return spikes
}

// MarkSpikeNotified marks the spike as notified.
func MarkSpikeNotified(db *gorm.DB, state *SpikeState) {
	state.LastAlertAt = time.Now().UTC()
	db.Save(state)
}

// QueueDigestItem adds a warning to the daily digest queue.
func QueueDigestItem(db *gorm.DB, apiKeyID uint, path, message, severity string) {
	item := DigestItem{
		APIKeyID:  apiKeyID,
		Path:      path,
		Message:   message,
		Severity:  severity,
		CreatedAt: time.Now().UTC(),
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
	ids := make([]uint, len(items))
	for i, item := range items {
		ids[i] = item.ID
	}
	db.Model(&DigestItem{}).Where("id IN ?", ids).Update("resolved", true)
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

// UpdateSilenceSeverity applies one check result to the silence state.
// It works like UpdateSPCSeverity.
func UpdateSilenceSeverity(db *gorm.DB, state *SilenceState, severity string) {
	if severity == "" {
		if state.Severity != "" {
			state.CleanChecks++
			if state.CleanChecks >= RecoveryChecks {
				state.Severity = ""
				state.CleanChecks = 0
				state.SilenceStartAt = time.Time{}
			}
			db.Save(state)
		}
		return
	}

	if state.Severity == "" {
		state.SilenceStartAt = time.Now().UTC()
	}
	state.Severity = MaxSeverity(state.Severity, severity)
	state.CleanChecks = 0
	db.Save(state)
}

// GetUnnotifiedCriticalSilences returns critical silences that need an email.
func GetUnnotifiedCriticalSilences(db *gorm.DB) []SilenceState {
	var silences []SilenceState
	db.Where("severity = ? AND last_alert_at < silence_start_at AND last_alert_at < ?",
		"critical", time.Now().UTC().Add(-AlertCooldown)).Find(&silences)
	return silences
}

// MarkSilenceNotified marks the silence as notified.
func MarkSilenceNotified(db *gorm.DB, state *SilenceState) {
	state.LastAlertAt = time.Now().UTC()
	db.Save(state)
}
