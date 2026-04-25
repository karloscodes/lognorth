package jobs

import (
	"fmt"
	"time"

	"github.com/karloscodes/cartridge"
	"gorm.io/gorm"

	"lognorth/internal/apikeys"
	"lognorth/internal/events"
	"lognorth/internal/settings"
)

// UptimeSweeper detects when an app stops sending events ("silence" at the
// app level — distinct from per-endpoint silence in AnomalyChecker) and
// sends a single notification per silent period plus one on recovery.
//
// Apps opt in by setting SilenceThresholdMinutes > 0. Default is 0 (off) so
// quiet apps don't get false-positive alerts.
type UptimeSweeper struct{}

// NewUptimeSweeper creates a new uptime sweeper.
func NewUptimeSweeper() *UptimeSweeper {
	return &UptimeSweeper{}
}

// bootstrapWindow is the grace period after an app is created during which
// silence detection is suppressed. Avoids false positives on a fresh app
// that hasn't started reporting yet.
const bootstrapWindow = 10 * time.Minute

// ProcessBatch checks every opt-in app for silence and sends alerts.
func (s *UptimeSweeper) ProcessBatch(ctx *cartridge.JobContext) error {
	db := ctx.DB

	var keys []apikeys.APIKey
	if err := db.
		Where("active = ? AND silence_threshold_minutes > 0", true).
		Find(&keys).Error; err != nil {
		return err
	}

	if len(keys) == 0 {
		return nil
	}

	settingsRow, err := settings.Get(db)
	if err != nil {
		return err
	}

	now := time.Now()
	for i := range keys {
		s.checkApp(db, &keys[i], settingsRow, now)
	}

	return nil
}

// checkApp evaluates a single app's silence state and notifies on transitions.
func (s *UptimeSweeper) checkApp(db *gorm.DB, key *apikeys.APIKey, settingsRow *settings.Settings, now time.Time) {
	// Bootstrap grace: don't alert on apps that just got created.
	if now.Sub(key.CreatedAt) < bootstrapWindow {
		return
	}

	threshold := time.Duration(key.SilenceThresholdMinutes) * time.Minute

	// Last event timestamp for this app.
	var lastEventStr string
	db.Raw(
		"SELECT MAX(timestamp) FROM events WHERE api_key_id = ?",
		key.ID,
	).Scan(&lastEventStr)

	if lastEventStr == "" {
		// Never received an event. Don't alert until the app reports at
		// least once — avoids paging for apps that exist in the DB but
		// haven't been deployed yet.
		return
	}

	lastEvent := events.ParseSQLiteTimestamp(lastEventStr)
	if lastEvent.IsZero() {
		return
	}

	silentFor := now.Sub(lastEvent)
	currentlySilent := silentFor >= threshold

	switch {
	case currentlySilent && !key.IsSilent:
		// Transition: live → silent. Send one alert.
		key.IsSilent = true
		key.SilenceStartAt = lastEvent
		s.notifySilent(settingsRow, key, lastEvent)
		key.LastSilenceAlertAt = now
		db.Save(key)

	case !currentlySilent && key.IsSilent:
		// Transition: silent → live. Send one recovery alert.
		s.notifyRecovered(settingsRow, key, key.SilenceStartAt)
		key.IsSilent = false
		key.SilenceStartAt = time.Time{}
		db.Save(key)
	}
}

func (s *UptimeSweeper) notifySilent(settingsRow *settings.Settings, key *apikeys.APIKey, lastEvent time.Time) {
	if !settingsRow.CanNotify() {
		return
	}
	silentMin := int(time.Since(lastEvent).Minutes())
	subject := fmt.Sprintf("%s — silent for %d min", key.Name, silentMin)
	body := fmt.Sprintf(`%s hasn't sent an event in %d min.
Last event: %s

%s/apps/%d/issues`,
		key.Name,
		silentMin,
		lastEvent.Format(time.RFC3339),
		settingsRow.BaseURL,
		key.ID,
	)
	_ = settingsRow.SendNotification(subject, body)
}

func (s *UptimeSweeper) notifyRecovered(settingsRow *settings.Settings, key *apikeys.APIKey, silenceStartAt time.Time) {
	if !settingsRow.CanNotify() {
		return
	}
	silentMin := int(time.Since(silenceStartAt).Minutes())
	subject := fmt.Sprintf("%s — recovered after %d min silent", key.Name, silentMin)
	body := fmt.Sprintf(`%s is sending events again.
Was silent for %d min.

%s/apps/%d/issues`,
		key.Name,
		silentMin,
		settingsRow.BaseURL,
		key.ID,
	)
	_ = settingsRow.SendNotification(subject, body)
}
