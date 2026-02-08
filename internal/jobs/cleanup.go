package jobs

import (
	"time"

	"github.com/karloscodes/cartridge"

	"lognorth/internal/events"
)

// CleanupProcessor handles cleanup of old processed events
type CleanupProcessor struct{}

// NewCleanupProcessor creates a new cleanup processor
func NewCleanupProcessor() *CleanupProcessor {
	return &CleanupProcessor{}
}

// ProcessBatch cleans up old events
func (p *CleanupProcessor) ProcessBatch(ctx *cartridge.JobContext) error {
	// Delete processed ingested events older than 1 hour
	// (they've already been copied to the events table)
	deleted, err := events.DeleteProcessed(ctx.DB, 1*time.Hour)
	if err != nil {
		ctx.Logger.Error("Failed to cleanup processed events", "error", err)
		return err
	}
	if deleted > 0 {
		ctx.Logger.Info("Cleaned up processed ingested events", "deleted", deleted)
	}

	// Delete events older than 90 days
	deletedEvents, err := events.DeleteOlderThan(ctx.DB, 90*24*time.Hour)
	if err != nil {
		ctx.Logger.Error("Failed to cleanup old events", "error", err)
		return err
	}
	if deletedEvents > 0 {
		ctx.Logger.Info("Cleaned up old events", "deleted", deletedEvents)
	}

	// Delete endpoint stats older than 8 days (7 for baseline + 1 buffer)
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
