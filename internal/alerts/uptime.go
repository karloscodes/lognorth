package alerts

import (
	"fmt"
	"lognorth/internal/apikeys"
	"lognorth/internal/events"
	"lognorth/internal/settings"
	"lognorth/internal/uptime"
	"time"

	"gorm.io/gorm"
)

// An app that does not answer its uptime ping: down after FailuresToDown
// failed pings in a row, and recovered on the next answer.

// ApplyPing records an uptime probe of an app and applies the state
// transitions: down after FailuresToDown fails in a row, up again on the
// next success. It alerts on each.
func ApplyPing(db *gorm.DB, key *apikeys.APIKey, check uptime.Check, settingsRow *settings.Settings, now time.Time) {
	check.APIKeyID = key.ID
	check.CheckedAt = now
	// Every probe is kept so the app can show response times and failures.
	_ = uptime.Record(db, &check)
	up := check.Up
	key.LastPingedAt = now

	switch {
	case !up:
		key.ConsecutiveFails++
		if key.ConsecutiveFails >= uptime.FailuresToDown && !key.IsDown {
			key.IsDown = true
			key.DownSince = now
		}
		// Retried every sweep until it goes out: an outage that starts in
		// quiet hours alerts when they end.
		if key.IsDown && !key.DownNotified {
			key.DownNotified = notifyDown(settingsRow, key, surgesNow(db, key.ID, now))
		}
	case up && key.IsDown:
		// Recovered.
		downFor := now.Sub(key.DownSince)
		key.IsDown = false
		key.DownNotified = false
		key.DownSince = time.Time{}
		key.ConsecutiveFails = 0
		notifyRecovered(settingsRow, key, downFor)
	default:
		// Healthy probe; reset the fail counter (handles flap).
		key.ConsecutiveFails = 0
	}

	// Only the ping columns: a full Save would overwrite an edit made
	// while the probe ran.
	db.Model(key).Updates(map[string]any{
		"last_pinged_at":    key.LastPingedAt,
		"consecutive_fails": key.ConsecutiveFails,
		"is_down":           key.IsDown,
		"down_notified":     key.DownNotified,
		"down_since":        key.DownSince,
	})
}

// surgesNow returns the issues that surged as the app went down, from the
// outage that is running now.
func surgesNow(db *gorm.DB, appID uint, now time.Time) []events.Surge {
	outages, err := uptime.RecentOutages(db, appID, now.Add(-24*time.Hour), 1)
	if err != nil || len(outages) == 0 || !outages[0].Ongoing {
		return nil
	}
	return outages[0].Issues
}

// notifyDown reports whether the alert went out.
func notifyDown(settingsRow *settings.Settings, key *apikeys.APIKey, surges []events.Surge) bool {
	if settingsRow == nil || !CanNotify(settingsRow) {
		return false
	}
	subject := fmt.Sprintf("%s — down: %s", key.Name, key.PingURL)
	return Send(settingsRow, subject, downBody(key, settingsRow.BaseURL, surges)) == nil
}

// downBody says the app is down and, when some issues surged as it went
// down, names up to three of them. They are the likely cause, so the agent
// command points at the first. With no surge it says nothing about errors.
func downBody(key *apikeys.APIKey, baseURL string, surges []events.Surge) string {
	body := fmt.Sprintf("%s isn't responding.\nURL: %s\n%d consecutive failures.\n", key.Name, key.PingURL, uptime.FailuresToDown)
	if len(surges) > 0 {
		body += "\nErrors that surged as it went down:\n"
		for _, sg := range surges[:min(3, len(surges))] {
			body += fmt.Sprintf("  %dx %s\n  %s/issues/%s\n", sg.Count, FormatHeadline(sg.ErrorClass, sg.Error), baseURL, sg.Hash)
		}
		body += "\nAsk your agent: /lognorth:investigate " + surges[0].Hash + "\n"
	}
	return body + fmt.Sprintf("\n%s/apps/%d/endpoints", baseURL, key.ID)
}

func notifyRecovered(settingsRow *settings.Settings, key *apikeys.APIKey, downFor time.Duration) {
	if settingsRow == nil || !CanNotify(settingsRow) {
		return
	}
	subject := fmt.Sprintf("%s — recovered after %s down", key.Name, FormatDuration(downFor))
	body := fmt.Sprintf(`%s is responding again.
URL: %s
Was down for %s.

%s/apps/%d/issues`,
		key.Name,
		key.PingURL,
		FormatDuration(downFor),
		settingsRow.BaseURL,
		key.ID,
	)
	_ = Send(settingsRow, subject, body)
}
