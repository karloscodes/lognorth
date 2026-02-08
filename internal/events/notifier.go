package events

// Alerting logic has moved to alerting.go
// This file is kept for backwards compatibility with existing imports

import (
	"gorm.io/gorm"
)

// NotifyIfNeeded is deprecated, use AlertEngine.CheckNewErrorOrRegression
func NotifyIfNeeded(db *gorm.DB, ctx map[string]any, apiKeyID uint, appName string) {
	engine, err := NewAlertEngine(db)
	if err != nil {
		return
	}
	engine.CheckNewErrorOrRegression(ctx, apiKeyID, appName)
}

// SendSpikeAlert is deprecated, use AlertEngine.CheckSpikes
func SendSpikeAlert(_ any, _, _ string, _, _ float64) {
	// No-op: spike alerts are now handled by AlertEngine.CheckSpikes
}
