package jobs

import (
	"github.com/karloscodes/cartridge"
	"gorm.io/gorm"

	"lognorth/internal/apikeys"
	"lognorth/internal/events"
)

// EventProcessor moves ingested events into the events table
type EventProcessor struct{}

// NewEventProcessor creates a new event processor
func NewEventProcessor() *EventProcessor {
	return &EventProcessor{}
}

const processBatchSize = 100

// ProcessBatch moves every waiting event. Each event moves in its own
// transaction: insert into events, record endpoint stats, delete the staging
// row. A crash never processes an event twice, and a row that fails stays for
// the next run without blocking the rows after it.
func (p *EventProcessor) ProcessBatch(ctx *cartridge.JobContext) error {
	db := ctx.DB

	// Loaded once per run: settings for alerts, names for alert subjects.
	alerts, err := events.NewAlertEngine(db)
	if err != nil {
		return err
	}
	appNames := map[uint]string{}

	var afterID uint
	processed := 0
	for {
		batch, err := events.NextIngested(db, afterID, processBatchSize)
		if err != nil {
			return err
		}

		for _, ie := range batch {
			afterID = ie.ID
			event, err := moveEvent(db, ie)
			if err != nil {
				ctx.Logger.Error("Failed to process event", "ingested_id", ie.ID, "error", err)
				continue
			}
			processed++

			if event.IsError {
				if err := alerts.CheckNewErrorOrRegression(event, appName(db, appNames, ie.APIKeyID)); err != nil {
					ctx.Logger.Error("Failed to send error alert", "issue", event.IssueHash, "error", err)
				}
			}
		}

		if len(batch) < processBatchSize {
			break
		}
	}

	if processed > 0 {
		ctx.Logger.Info("Processed events", "count", processed)
	}
	return nil
}

// moveEvent turns one staging row into an event in a single transaction.
func moveEvent(db *gorm.DB, ie events.IngestedEvent) (*events.Event, error) {
	event := &events.Event{
		APIKeyID:   ie.APIKeyID,
		Message:    ie.Message,
		TraceID:    ie.TraceID,
		DurationMS: ie.DurationMS,
		IsError:    ie.IsError,
		Context:    ie.Context,
		Timestamp:  ie.Timestamp,
	}
	eventCtx, _ := event.GetContext()
	if ie.IsError && eventCtx != nil {
		events.PopulateErrorFields(event, eventCtx)
	}

	err := db.Transaction(func(tx *gorm.DB) error {
		if err := events.Create(tx, event); err != nil {
			return err
		}
		if eventCtx != nil && eventCtx.Path != "" {
			if err := events.RecordEndpointStat(tx, ie.APIKeyID, eventCtx.Path, ie.IsError, ie.DurationMS, event.Timestamp); err != nil {
				return err
			}
		}
		return tx.Delete(&events.IngestedEvent{}, ie.ID).Error
	})
	return event, err
}

func appName(db *gorm.DB, cache map[uint]string, apiKeyID uint) string {
	if name, ok := cache[apiKeyID]; ok {
		return name
	}
	name := "Unknown"
	if key, err := apikeys.FindByID(db, apiKeyID); err == nil {
		name = key.Name
	}
	cache[apiKeyID] = name
	return name
}
