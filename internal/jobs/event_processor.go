package jobs

import (
	"github.com/karloscodes/cartridge"

	"lognorth/internal/apikeys"
	"lognorth/internal/events"
	lbhttp "lognorth/internal/http"
)

// EventProcessor handles processing of ingested events
type EventProcessor struct{}

// NewEventProcessor creates a new event processor
func NewEventProcessor() *EventProcessor {
	return &EventProcessor{}
}

// ProcessBatch processes unprocessed events from the ingested_events table
func (p *EventProcessor) ProcessBatch(ctx *cartridge.JobContext) error {
	db := ctx.DB

	// Count unprocessed events
	unprocessedCount, err := events.CountUnprocessed(db)
	if err != nil {
		ctx.Logger.Error("Failed to count unprocessed events", "error", err)
		return err
	}

	if unprocessedCount == 0 {
		return nil
	}

	ctx.Logger.Info("Processing events", "unprocessed", unprocessedCount)

	// Process in batches of 100
	batchSize := 100
	processed := 0

	for {
		count, err := p.processBatch(ctx, batchSize)
		if err != nil {
			ctx.Logger.Error("Failed to process batch", "error", err)
			return err
		}

		processed += count
		if count < batchSize {
			break // No more events to process
		}
	}

	ctx.Logger.Info("Finished processing events", "processed", processed)
	return nil
}

func (p *EventProcessor) processBatch(ctx *cartridge.JobContext, batchSize int) (int, error) {
	db := ctx.DB

	// Fetch unprocessed events
	ingested, err := events.GetUnprocessed(db, batchSize)
	if err != nil {
		return 0, err
	}

	if len(ingested) == 0 {
		return 0, nil
	}

	// Process each event
	var processedIDs []uint
	for _, ie := range ingested {
		event := &events.Event{
			APIKeyID:   ie.APIKeyID,
			Message:    ie.Message,
			TraceID:    ie.TraceID,
			DurationMS: ie.DurationMS,
			IsError:    ie.IsError,
			Context:    ie.Context,
			Timestamp:  ie.Timestamp,
		}

		if err := events.Create(db, event); err != nil {
			ctx.Logger.Error("Failed to create event",
				"ingested_id", ie.ID,
				"error", err)
			continue
		}

		// Record endpoint stats for spike detection
		eventCtx, _ := event.GetContext()
		if eventCtx != nil && eventCtx.Path != "" {
			events.RecordEndpointStat(db, ie.APIKeyID, eventCtx.Path, ie.IsError, ie.DurationMS, ie.Timestamp)
		}

		// Notify SSE subscribers
		lbhttp.NotifyEventSubscribers(event)

		// Check if we need to send notifications for this error
		if ie.IsError {
			eventCtx, _ := event.GetContext()
			if eventCtx != nil && eventCtx.Error != "" {
				appName := "Unknown"
				if apiKey, err := apikeys.FindByID(db, ie.APIKeyID); err == nil {
					appName = apiKey.Name
				}
				events.NotifyIfNeeded(db, eventCtx.ToMap(), ie.APIKeyID, appName)
			}
		}

		processedIDs = append(processedIDs, ie.ID)
	}

	// Mark as processed
	if len(processedIDs) > 0 {
		if err := events.MarkProcessed(db, processedIDs); err != nil {
			ctx.Logger.Error("Failed to mark events as processed", "error", err)
			return 0, err
		}
	}

	return len(processedIDs), nil
}
