package alerts

import (
	"fmt"
	"lognorth/internal/events"
	"lognorth/internal/settings"
	"strings"
	"time"

	"gorm.io/gorm"
)

// SecretSeen records that an app logged a kind of secret, and when the owner
// was told.
type SecretSeen struct {
	ID        uint   `gorm:"primaryKey"`
	APIKeyID  uint   `gorm:"uniqueIndex:idx_secret_seen"`
	Kind      string `gorm:"size:50;uniqueIndex:idx_secret_seen"`
	AlertedAt *time.Time
	CreatedAt time.Time
}

func (SecretSeen) TableName() string {
	return "secrets_seen"
}

// A Leak is a secret the processor redacted from a stored event.
type Leak struct {
	Event *events.Event
	Kind  string
}

// CheckLeak returns the alert of the first time an app logs a kind of
// secret, and nil after that: the app and kind stay quiet once told. Unlike
// an error, a leak waits for a channel: with none set up, the next leak of
// the kind tries again, because the key needs rotating whenever it was seen.
func CheckLeak(db *gorm.DB, leak Leak, s *settings.Settings, now time.Time) (*Alert, error) {
	seen := SecretSeen{APIKeyID: leak.Event.APIKeyID, Kind: leak.Kind}
	if err := db.Where(&seen).FirstOrCreate(&seen).Error; err != nil {
		return nil, err
	}
	if seen.AlertedAt != nil || s == nil || !ChannelConfigured(s) {
		return nil, nil
	}
	if err := db.Model(&seen).Update("alerted_at", now).Error; err != nil {
		return nil, err
	}
	return &Alert{
		APIKeyID: leak.Event.APIKeyID,
		Kind:     KindLeak,
		Title:    "logged a secret: " + leak.Kind,
		Body:     leakBody(leak, s.BaseURL),
	}, nil
}

// leakBody says what the app logged and where, and what to do about it.
func leakBody(leak Leak, baseURL string) string {
	body := fmt.Sprintf("The app logged a secret: %s%s.\n", leak.Kind, placeOf(leak.Event))
	body += "LogNorth redacted it before it stored the event.\n\n"
	body += "Rotate the key. Any other place the app logs to can still have it.\n"
	body += "Then find the log line and stop logging the key."
	if baseURL != "" {
		body += fmt.Sprintf("\n\n%s/apps/%d/log/%d", baseURL, leak.Event.APIKeyID, leak.Event.ID)
	}
	return body
}

// placeOf names where an event came from: a request or a job.
func placeOf(event *events.Event) string {
	ctx, err := event.GetContext()
	switch {
	case err != nil:
		return ""
	case ctx.Path != "":
		return ", at " + strings.TrimSpace(ctx.Method+" "+ctx.Path)
	case ctx.Job != "":
		return ", in job " + ctx.Job
	}
	return ""
}
