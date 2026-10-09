package events

import (
	"time"

	"gorm.io/gorm"
)

// What a user did before an error: their requests in the minutes before it.
// The SDKs stamp the same user ID on each request, so the steps belong to
// that user for certain. They read as the way to reproduce the error.
const (
	stepsWindow = 30 * time.Minute
	stepsLimit  = 10
)

// StepsBefore returns the requests (a method and a path) of the event's user in the 30 minutes up
// to the event, oldest first, at most 10. The last one is usually the request
// that failed. It returns nothing when the event carries no user.
func StepsBefore(db *gorm.DB, event *Event) []Event {
	steps := []Event{}
	ctx, err := event.GetContext()
	if err != nil || ctx.User == "" {
		return steps
	}
	at := event.Timestamp.UTC()
	db.Where("api_key_id = ? AND timestamp >= ? AND timestamp <= ? AND id != ?", event.APIKeyID, at.Add(-stepsWindow), at, event.ID).
		Where("CAST(json_extract(context, '$.user') AS TEXT) = ?", ctx.User).
		Where("json_extract(context, '$.method') IS NOT NULL AND json_extract(context, '$.path') IS NOT NULL").
		Order("timestamp DESC, id DESC").
		Limit(stepsLimit).
		Find(&steps)
	for i, j := 0, len(steps)-1; i < j; i, j = i+1, j-1 {
		steps[i], steps[j] = steps[j], steps[i]
	}
	return steps
}
