package alerts

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"gorm.io/gorm"

	"lognorth/internal/apikeys"
	"lognorth/internal/settings"
)

// An Alert is one thing the owner hears about. A detector records it, and
// Deliver sends it: together with the other alerts of its app, and never in
// quiet hours. Detectors never send email themselves.
type Alert struct {
	ID        uint      `gorm:"primaryKey"`
	APIKeyID  uint      `gorm:"index;not null"`
	Kind      string    `gorm:"size:20;not null"`
	Title     string    `gorm:"size:500;not null"` // one line: the subject of an email that holds only this alert
	Body      string    `gorm:"type:text;not null"`
	CreatedAt time.Time `gorm:"not null"`
	// SentAt is set when the alert is done: sent, or dropped. Result says
	// which, or why the last try failed while it waits.
	SentAt *time.Time `gorm:"index"`
	Result string     `gorm:"size:500"`
}

func (Alert) TableName() string {
	return "alerts"
}

// The kinds of alert, in the order an email names them.
const (
	KindDown       = "down"
	KindSpike      = "spike"
	KindSilence    = "silence"
	KindNewError   = "new_error"
	KindRegression = "regression"
	KindLeak       = "leak"
	KindRecovered  = "recovered"
)

var kindOrder = []string{KindDown, KindSpike, KindSilence, KindNewError, KindRegression, KindLeak, KindRecovered}

// When an app's alerts go out. An incident (a deploy that breaks things, a
// database that goes away) sets off several detectors within minutes; the
// owner gets one email for it, then at most one every followUpGap.
const (
	// settle lets a burst gather: alerts go out once the oldest waited this long.
	settle = 30 * time.Second
	// followUpGap is the least time between two emails about one app.
	followUpGap = 10 * time.Minute
	// incidentGap: an app with no "down" email for this long tells the next
	// "down" at once, without waiting for followUpGap.
	incidentGap = 30 * time.Minute
	// maxWait drops an alert that could not go out for this long.
	maxWait = 24 * time.Hour
	// maxListed caps the alerts one email lists.
	maxListed = 10
)

// Record stores alerts for Deliver to send. Their CreatedAt is now.
func Record(db *gorm.DB, now time.Time, alerts ...*Alert) error {
	for _, a := range alerts {
		a.CreatedAt = now
		if err := db.Create(a).Error; err != nil {
			return err
		}
	}
	return nil
}

// Deliver sends the alerts that wait, one email per app:
//
//   - An app's alerts go out together once the oldest waited settle, so a
//     burst lands in one email.
//   - An app gets at most one email per followUpGap. The first "down" of an
//     incident does not wait for it.
//   - In quiet hours nothing goes out. When they end, the alerts of the night
//     go out in one email per app.
//   - Without a channel, alerts are dropped: setting one up later does not
//     send the past. An alert that cannot go out for maxWait is dropped too.
func Deliver(db *gorm.DB, now time.Time) error {
	s, err := settings.Get(db)
	if err != nil {
		return err
	}
	var waiting []Alert
	if err := db.Where("sent_at IS NULL").Order("id").Find(&waiting).Error; err != nil {
		return err
	}
	if len(waiting) == 0 {
		return nil
	}
	if !ChannelConfigured(s) {
		return finish(db, waiting, now, "dropped: no channel")
	}

	var stale, fresh []Alert
	for _, a := range waiting {
		if now.Sub(a.CreatedAt) > maxWait {
			stale = append(stale, a)
		} else {
			fresh = append(fresh, a)
		}
	}
	if err := finish(db, stale, now, "dropped: waited a day"); err != nil {
		return err
	}
	if IsQuietHours(s, now) {
		return nil
	}

	byApp := map[uint][]Alert{}
	var apps []uint
	for _, a := range fresh {
		if byApp[a.APIKeyID] == nil {
			apps = append(apps, a.APIKeyID)
		}
		byApp[a.APIKeyID] = append(byApp[a.APIKeyID], a)
	}
	for _, app := range apps {
		pending := byApp[app]
		lastEmail, lastDown, err := lastSent(db, app)
		if err != nil {
			return err
		}
		if !due(pending, lastEmail, lastDown, now) {
			continue
		}
		subject, body := compose(s, appName(db, app), app, pending, now)
		if err := Send(s, subject, body); err != nil {
			// It waits, and the next run tries again.
			ids := alertIDs(pending)
			if err := db.Model(&Alert{}).Where("id IN ?", ids).Update("result", "failed: "+err.Error()).Error; err != nil {
				return err
			}
			continue
		}
		if err := finish(db, pending, now, "sent"); err != nil {
			return err
		}
	}
	return nil
}

// due reports whether an app's waiting alerts go out now.
func due(pending []Alert, lastEmail, lastDown time.Time, now time.Time) bool {
	if now.Sub(pending[0].CreatedAt) < settle {
		return false
	}
	if now.Sub(lastEmail) >= followUpGap {
		return true
	}
	for _, a := range pending {
		if a.Kind == KindDown && now.Sub(lastDown) >= incidentGap {
			return true
		}
	}
	return false
}

// lastSent returns when the app's last email went out, and the last one that
// told a "down". Zero when there is none.
func lastSent(db *gorm.DB, app uint) (time.Time, time.Time, error) {
	var times struct {
		Last string
		Down string
	}
	err := db.Model(&Alert{}).
		Select("MAX(sent_at) AS last, MAX(CASE WHEN kind = ? THEN sent_at END) AS down", KindDown).
		Where("api_key_id = ? AND result = ?", app, "sent").
		Scan(&times).Error
	return parseTime(times.Last), parseTime(times.Down), err
}

// parseTime reads a time SQLite returns from MAX(), or zero.
func parseTime(s string) time.Time {
	for _, layout := range []string{"2006-01-02 15:04:05.999999999-07:00", time.RFC3339Nano} {
		if t, err := time.Parse(layout, s); err == nil {
			return t
		}
	}
	return time.Time{}
}

func finish(db *gorm.DB, alerts []Alert, now time.Time, result string) error {
	if len(alerts) == 0 {
		return nil
	}
	return db.Model(&Alert{}).Where("id IN ?", alertIDs(alerts)).
		Updates(map[string]any{"sent_at": now, "result": result}).Error
}

func alertIDs(alerts []Alert) []uint {
	ids := make([]uint, len(alerts))
	for i, a := range alerts {
		ids[i] = a.ID
	}
	return ids
}

func appName(db *gorm.DB, app uint) string {
	if key, err := apikeys.FindByID(db, app); err == nil {
		return key.Name
	}
	return "Unknown app"
}

// ChannelConfigured reports whether the owner set up a channel to tell them.
func ChannelConfigured(s *settings.Settings) bool {
	switch s.NotifyChannel {
	case "ntfy":
		return s.IsNtfyConfigured()
	case "email":
		return s.IsEmailConfigured()
	case "telegram":
		return s.IsTelegramConfigured()
	case "webhook":
		return s.IsWebhookConfigured()
	}
	return false
}

// compose builds the one email of an app's waiting alerts. One alert keeps
// the subject and body it has always had. Several are counted in the subject
// and listed in the body, oldest first.
func compose(s *settings.Settings, name string, app uint, pending []Alert, now time.Time) (string, string) {
	loc := Location(s)
	heldOvernight := IsQuietHours(s, pending[0].CreatedAt)
	if len(pending) == 1 {
		a := pending[0]
		body := a.Body
		if heldOvernight {
			body = fmt.Sprintf("This came in at %s, in your quiet hours.\n\n%s", a.CreatedAt.In(loc).Format("15:04"), body)
		}
		return name + " — " + a.Title, body
	}

	subject := name + " — " + countKinds(pending)
	if heldOvernight {
		subject = name + " — while you were off: " + countKinds(pending)
	}
	var body strings.Builder
	for i, a := range pending[:min(len(pending), maxListed)] {
		if i > 0 {
			body.WriteString("\n\n")
		}
		body.WriteString(a.CreatedAt.In(loc).Format("15:04") + "  " + a.Title + "\n")
		body.WriteString(indent(listedBody(a)))
	}
	if more := len(pending) - maxListed; more > 0 {
		body.WriteString(fmt.Sprintf("\n\nAnd %d more", more))
		if s.BaseURL != "" {
			body.WriteString(fmt.Sprintf(": %s/apps/%d/issues", s.BaseURL, app))
		}
		body.WriteString(".")
	}
	return subject, body.String()
}

// countKinds names what an email holds, most urgent first: "down, 15
// endpoints failing, 1 new error".
func countKinds(alerts []Alert) string {
	counts := map[string]int{}
	for _, a := range alerts {
		counts[a.Kind]++
	}
	var parts []string
	for _, kind := range kindOrder {
		n := counts[kind]
		if n == 0 {
			continue
		}
		parts = append(parts, kindPhrase(kind, n))
	}
	// "2 new errors, 1 back" reads better than "1 error back" after it.
	if counts[KindNewError] > 0 && counts[KindRegression] > 0 {
		i := slices.Index(parts, kindPhrase(KindRegression, counts[KindRegression]))
		parts[i] = fmt.Sprintf("%d back", counts[KindRegression])
	}
	// "down, recovered" reads better as one phrase.
	if i := slices.Index(parts, kindPhrase(KindDown, counts[KindDown])); i >= 0 && counts[KindRecovered] > 0 {
		parts[i] += " and recovered"
		parts = slices.DeleteFunc(parts, func(p string) bool { return p == kindPhrase(KindRecovered, counts[KindRecovered]) })
	}
	return strings.Join(parts, ", ")
}

func kindPhrase(kind string, n int) string {
	switch kind {
	case KindDown:
		if n == 1 {
			return "down"
		}
		return fmt.Sprintf("down %d times", n)
	case KindRecovered:
		return "recovered"
	case KindSpike:
		return fmt.Sprintf("%d %s failing", n, plural(n, "endpoint", "endpoints"))
	case KindSilence:
		return fmt.Sprintf("%d %s silent", n, plural(n, "endpoint", "endpoints"))
	case KindNewError:
		return fmt.Sprintf("%d new %s", n, plural(n, "error", "errors"))
	case KindRegression:
		return fmt.Sprintf("%d %s back", n, plural(n, "error", "errors"))
	case KindLeak:
		if n == 1 {
			return "logged a secret"
		}
		return fmt.Sprintf("logged %d secrets", n)
	}
	return fmt.Sprintf("%d %s", n, kind)
}

// listedBody is an alert's body under its title in a list. A body that
// opens with the title's own words (an error's headline) skips that line.
func listedBody(a Alert) string {
	first, rest, _ := strings.Cut(a.Body, "\n")
	if first = strings.TrimSpace(first); first != "" && strings.HasSuffix(a.Title, first) {
		return strings.TrimLeft(rest, "\n")
	}
	return a.Body
}

func indent(text string) string {
	lines := strings.Split(strings.TrimRight(text, "\n"), "\n")
	for i, l := range lines {
		if l != "" {
			lines[i] = "  " + l
		}
	}
	return strings.Join(lines, "\n")
}
