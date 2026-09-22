package jobs

import (
	"time"

	"github.com/karloscodes/cartridge"

	"lognorth/internal/events"
	"lognorth/internal/settings"
)

// CleanupProcessor handles cleanup of old processed events
type CleanupProcessor struct{}

// NewCleanupProcessor creates a new cleanup processor
func NewCleanupProcessor() *CleanupProcessor {
	return &CleanupProcessor{}
}

// ProcessBatch cleans up old events
func (p *CleanupProcessor) ProcessBatch(ctx *cartridge.JobContext) error {
	// Delete events older than configured retention period
	s, err := settings.Get(ctx.DB)
	if err != nil {
		ctx.Logger.Error("Failed to load settings for retention", "error", err)
		return err
	}
	retention := s.GetRetentionDuration()

	// Skip if retention is 0 (keep forever)
	if retention > 0 {
		deletedEvents, err := events.DeleteOlderThan(ctx.DB, retention)
		if err != nil {
			ctx.Logger.Error("Failed to cleanup old events", "error", err)
			return err
		}
		if deletedEvents > 0 {
			ctx.Logger.Info("Cleaned up old events", "deleted", deletedEvents, "retention_days", s.RetentionDays)
		}
	}

	// Without a license, ingested events wait unprocessed. Keep that backlog
	// within the retention period (90 days when retention is "forever").
	backlog := retention
	if backlog == 0 {
		backlog = 90 * 24 * time.Hour
	}
	deletedBacklog, err := events.DeleteIngestedOlderThan(ctx.DB, backlog)
	if err != nil {
		ctx.Logger.Error("Failed to cleanup the ingest backlog", "error", err)
		return err
	}
	if deletedBacklog > 0 {
		ctx.Logger.Info("Cleaned up the ingest backlog", "deleted", deletedBacklog)
	}

	// Delete endpoint stats older than the SPC history needs
	deletedStats, err := events.CleanupOldStats(ctx.DB)
	if err != nil {
		ctx.Logger.Error("Failed to cleanup old stats", "error", err)
		return err
	}
	if deletedStats > 0 {
		ctx.Logger.Info("Cleaned up old endpoint stats", "deleted", deletedStats)
	}

	return nil
}
