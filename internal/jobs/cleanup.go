package jobs

import (
	"time"

	"github.com/karloscodes/cartridge"

	"lognorth/internal/events"
	"lognorth/internal/settings"
	"lognorth/internal/status"
	"lognorth/internal/uptime"
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

	// An ingested event that fails to process stays in the backlog. Keep that
	// backlog within the retention period (90 days when retention is "forever").
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

	// Each finished day of pings is counted before its pings go. The counts
	// stay for good: they hold each app's uptime since its first ping.
	now := time.Now()
	if err := uptime.RollUpAll(ctx.DB, now); err != nil {
		ctx.Logger.Error("Failed to roll up uptime days", "error", err)
		return err
	}

	// Uptime checks: one a minute per app, kept uptime.Retention.
	deletedChecks, err := uptime.DeleteOlderThan(ctx.DB, now.Add(-uptime.Retention))
	if err != nil {
		ctx.Logger.Error("Failed to cleanup old uptime checks", "error", err)
		return err
	}
	if deletedChecks > 0 {
		ctx.Logger.Info("Cleaned up old uptime checks", "deleted", deletedChecks)
	}

	// Status page notes go with the pings of their outage.
	deletedNotes, err := status.DeleteOlderThan(ctx.DB, now.Add(-uptime.Retention))
	if err != nil {
		ctx.Logger.Error("Failed to cleanup old status page notes", "error", err)
		return err
	}
	if deletedNotes > 0 {
		ctx.Logger.Info("Cleaned up old status page notes", "deleted", deletedNotes)
	}

	return nil
}
