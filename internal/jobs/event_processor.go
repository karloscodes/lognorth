package jobs

import (
	"errors"

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

// ProcessBatch moves every waiting event, processBatchSize events per write
// transaction. Each event moves under its own savepoint: insert into events,
// record endpoint stats, delete the staging row. A crash never processes an
// event twice, and a row that fails rolls back alone and stays for the next
// run without blocking the rows after it. Alerts go out after the commit, so
// a slow email never holds the write lock.
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
		if len(batch) == 0 {
			break
		}
		afterID = batch[len(batch)-1].ID

		moved, err := moveEvents(ctx, batch)
		if err != nil {
			// Busy: the rows stay, and the next run moves them.
			return err
		}
		processed += len(moved)

		for _, event := range moved {
			if event.IsError {
				if err := alerts.CheckNewErrorOrRegression(event, appName(db, appNames, event.APIKeyID)); err != nil {
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

// moveEvents moves a batch in one write transaction and returns the events
// it stored. Endpoint stats count only the events that moved, and each stat
// bucket is written once per batch.
func moveEvents(ctx *cartridge.JobContext, batch []events.IngestedEvent) ([]*events.Event, error) {
	var moved []*events.Event
	err := ctx.WriteTx(func(tx *gorm.DB) error {
		stats := events.NewStatBatch()
		for _, ie := range batch {
			event, path, err := moveEvent(tx, ie)
			if errors.Is(err, errAlreadyMoved) {
				continue
			}
			if err != nil {
				ctx.Logger.Error("Failed to process event", "ingested_id", ie.ID, "error", err)
				continue
			}
			if err := stats.Add(tx, ie.APIKeyID, path, ie.IsError, ie.DurationMS, event.Timestamp); err != nil {
				return err
			}
			moved = append(moved, event)
		}
		return stats.Flush(tx)
	})
	if err != nil {
		return nil, err
	}
	return moved, nil
}

// errAlreadyMoved means another processor moved the staging row first. During
// a deploy the old and the new container both run a processor, and both can
// read the same row before either deletes it.
var errAlreadyMoved = errors.New("ingested event already moved")

// moveEvent turns one staging row into an event and returns the event and its
// endpoint path. Inside a transaction it runs under a savepoint, so a failure
// undoes only this event. It deletes the staging row first: when that deletes
// nothing, another processor has the event, and moveEvent stores no copy.
func moveEvent(db *gorm.DB, ie events.IngestedEvent) (*events.Event, string, error) {
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
		deleted := tx.Delete(&events.IngestedEvent{}, ie.ID)
		if deleted.Error != nil {
			return deleted.Error
		}
		if deleted.RowsAffected == 0 {
			return errAlreadyMoved
		}
		return events.Create(tx, event)
	})
	path := ""
	if eventCtx != nil {
		path = eventCtx.Path
	}
	return event, path, err
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
