package jobs

import (
	"errors"
	"time"

	"github.com/karloscodes/cartridge"
	"gorm.io/gorm"

	"lognorth/internal/alerts"
	"lognorth/internal/events"
	"lognorth/internal/settings"
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
// run without blocking the rows after it. After the commit, it records the
// alerts of new and regressed issues and of leaked secrets; alerts.Deliver
// sends them.
func (p *EventProcessor) ProcessBatch(ctx *cartridge.JobContext) error {
	db := ctx.DB
	now := time.Now()
	s, err := settings.Get(db)
	if err != nil {
		return err
	}
	baseURL := s.BaseURL

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

		moved, leaks, err := moveEvents(ctx, batch)
		if err != nil {
			// Busy: the rows stay, and the next run moves them.
			return err
		}
		processed += len(moved)

		for _, event := range moved {
			if !event.IsError {
				continue
			}
			alert, err := alerts.CheckError(db, event, baseURL, now)
			if err != nil {
				ctx.Logger.Error("Failed to record an issue occurrence", "issue", event.IssueHash, "error", err)
			}
			if alert != nil {
				if err := alerts.Record(db, now, alert); err != nil {
					ctx.Logger.Error("Failed to record an error alert", "issue", event.IssueHash, "error", err)
				}
			}
		}
		for _, leak := range leaks {
			alert, err := alerts.CheckLeak(db, leak, s, now)
			if err == nil && alert != nil {
				err = alerts.Record(db, now, alert)
			}
			if err != nil {
				ctx.Logger.Error("Failed to record a secret alert", "kind", leak.Kind, "error", err)
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
// it stored, and the secrets it redacted from them. Endpoint stats count only
// the events that moved, and each stat bucket is written once per batch.
func moveEvents(ctx *cartridge.JobContext, batch []events.IngestedEvent) ([]*events.Event, []alerts.Leak, error) {
	var moved []*events.Event
	var leaks []alerts.Leak
	err := ctx.WriteTx(func(tx *gorm.DB) error {
		stats := events.NewStatBatch()
		hours := events.HourBatch{}
		durations := events.DurationBatch{}
		issues := events.IssueBatch{}
		for _, ie := range batch {
			// Before anything reads the event, so no copy keeps a secret.
			kinds := events.RedactSecrets(&ie)
			event, path, endpoint, err := moveEvent(tx, ie)
			if errors.Is(err, errAlreadyMoved) {
				continue
			}
			if err != nil {
				ctx.Logger.Error("Failed to process event", "ingested_id", ie.ID, "error", err)
				continue
			}
			if err := stats.Add(tx, ie.APIKeyID, endpoint, ie.IsError, ie.DurationMS, event.Timestamp); err != nil {
				return err
			}
			hours.Add(ie.APIKeyID, ie.IsError, event.Timestamp)
			durations.Add(ie.APIKeyID, endpoint, ie.DurationMS, event.Timestamp)
			if err := issues.Add(tx, event, path); err != nil {
				return err
			}
			moved = append(moved, event)
			for _, kind := range kinds {
				leaks = append(leaks, alerts.Leak{Event: event, Kind: kind})
			}
		}
		if err := stats.Flush(tx); err != nil {
			return err
		}
		if err := hours.Flush(tx); err != nil {
			return err
		}
		if err := durations.Flush(tx); err != nil {
			return err
		}
		return issues.Flush(tx)
	})
	if err != nil {
		return nil, nil, err
	}
	return moved, leaks, nil
}

// errAlreadyMoved means another processor moved the staging row first. During
// a deploy the old and the new container both run a processor, and both can
// read the same row before either deletes it.
var errAlreadyMoved = errors.New("ingested event already moved")

// moveEvent turns one staging row into an event and returns the event, its
// path, and its endpoint (events.EndpointKey). Inside a transaction it runs under a savepoint, so a failure
// undoes only this event. It deletes the staging row first: when that deletes
// nothing, another processor has the event, and moveEvent stores no copy.
func moveEvent(db *gorm.DB, ie events.IngestedEvent) (*events.Event, string, string, error) {
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
		if err := events.Create(tx, event); err != nil {
			return err
		}
		// A release start or an error names the version that ran: the first
		// one seen is the deploy the charts mark.
		if eventCtx != nil {
			return events.NoteRelease(tx, event.APIKeyID, eventCtx.Release, event.Timestamp)
		}
		return nil
	})
	path, endpoint := "", ""
	if eventCtx != nil {
		path, endpoint = eventCtx.Path, events.EndpointKey(eventCtx.Route, eventCtx.Path)
	}
	return event, path, endpoint, err
}
