package alerts

import (
	"fmt"
	"lognorth/internal/events"
	"strings"
	"time"
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

// CheckLeak alerts the first time an app logs a kind of secret. While the
// alert can't go out (no channel yet, quiet hours, a failed send), the next
// leak of that kind tries again. After it goes out, the app and kind stay
// quiet.
func (e *AlertEngine) CheckLeak(leak Leak, appName string) error {
	seen := SecretSeen{APIKeyID: leak.Event.APIKeyID, Kind: leak.Kind}
	if err := e.db.Where(&seen).FirstOrCreate(&seen).Error; err != nil {
		return err
	}
	if seen.AlertedAt != nil || !e.CanNotify() {
		return nil
	}

	subject := fmt.Sprintf("%s — logged a secret: %s", appName, leak.Kind)
	if err := Send(e.settings, subject, leakBody(leak, appName, e.settings.BaseURL)); err != nil {
		return err
	}
	return e.db.Model(&seen).Update("alerted_at", time.Now()).Error
}

// leakBody says what the app logged and where, and what to do about it.
func leakBody(leak Leak, appName, baseURL string) string {
	body := fmt.Sprintf("%s logged a secret: %s%s.\n", appName, leak.Kind, placeOf(leak.Event))
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
