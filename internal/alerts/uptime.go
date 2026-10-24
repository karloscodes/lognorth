package alerts

import (
	"fmt"
	"lognorth/internal/apikeys"
	"lognorth/internal/events"
	"lognorth/internal/uptime"
	"time"

	"gorm.io/gorm"
)

// An app that does not answer its uptime ping: down after FailuresToDown
// failed pings in a row, and recovered after RecoveryPings good ones.

// RecoveryPings is how many good pings in a row a down app needs to count as
// recovered. One answer between failures is a flap, not a recovery: a flapping
// app stays down and sends one alert, not one per flap.
const RecoveryPings = 5

// ApplyPing records an uptime probe of an app, applies the state transitions,
// and records an alert on each: down after FailuresToDown failed pings in a
// row, recovered after RecoveryPings good ones.
func ApplyPing(db *gorm.DB, key *apikeys.APIKey, check uptime.Check, baseURL string, now time.Time) error {
	check.APIKeyID = key.ID
	check.CheckedAt = now
	// Every probe is kept so the app can show response times and failures.
	if err := uptime.Record(db, &check); err != nil {
		return err
	}
	key.LastPingedAt = now

	var alert *Alert
	switch {
	case !check.Up:
		key.ConsecutiveUps = 0
		key.UpSince = time.Time{}
		key.ConsecutiveFails++
		if key.ConsecutiveFails >= uptime.FailuresToDown && !key.IsDown {
			key.IsDown = true
			key.DownSince = firstFailedPing(db, key.ID, now)
			key.DownNotified = true
			alert = &Alert{APIKeyID: key.ID, Kind: KindDown, Title: "down: " + key.PingURL, Body: downBody(key, baseURL, surgesNow(db, key.ID, now))}
		}
	case key.IsDown:
		key.ConsecutiveFails = 0
		if key.ConsecutiveUps == 0 {
			key.UpSince = now
		}
		key.ConsecutiveUps++
		if key.ConsecutiveUps >= RecoveryPings {
			alert = recoveredAlert(key, key.UpSince.Sub(key.DownSince), baseURL)
			key.IsDown = false
			key.DownNotified = false
			key.DownSince = time.Time{}
			key.ConsecutiveUps = 0
			key.UpSince = time.Time{}
		}
	default:
		// Healthy probe; reset the fail counter (handles flap).
		key.ConsecutiveFails = 0
	}

	// Only the ping columns: a full Save would overwrite an edit made
	// while the probe ran.
	err := db.Model(key).Updates(map[string]any{
		"last_pinged_at":    key.LastPingedAt,
		"consecutive_fails": key.ConsecutiveFails,
		"consecutive_ups":   key.ConsecutiveUps,
		"up_since":          key.UpSince,
		"is_down":           key.IsDown,
		"down_notified":     key.DownNotified,
		"down_since":        key.DownSince,
	}).Error
	if err != nil || alert == nil {
		return err
	}
	return Record(db, now, alert)
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

// firstFailedPing is when the run of failed pings that ends at now began: the
// app was down from then, not from the ping that confirmed it.
func firstFailedPing(db *gorm.DB, appID uint, now time.Time) time.Time {
	var lastUp struct{ At string }
	db.Model(&uptime.Check{}).Select("MAX(checked_at) AS at").
		Where("api_key_id = ? AND up = ? AND checked_at <= ?", appID, true, now).Scan(&lastUp)
	query := db.Model(&uptime.Check{}).Select("MIN(checked_at) AS at").
		Where("api_key_id = ? AND up = ? AND checked_at <= ?", appID, false, now)
	if since := parseTime(lastUp.At); !since.IsZero() {
		query = query.Where("checked_at > ?", since)
	}
	var first struct{ At string }
	query.Scan(&first)
	if at := parseTime(first.At); !at.IsZero() {
		return at
	}
	return now
}

// recoveredAlert says the app answers again, and how long it was down.
func recoveredAlert(key *apikeys.APIKey, downFor time.Duration, baseURL string) *Alert {
	body := fmt.Sprintf(`%s is responding again.
URL: %s
Was down for %s.

%s/apps/%d/issues`,
		key.Name,
		key.PingURL,
		FormatDuration(downFor),
		baseURL,
		key.ID,
	)
	return &Alert{APIKeyID: key.ID, Kind: KindRecovered, Title: fmt.Sprintf("recovered after %s down", FormatDuration(downFor)), Body: body}
}
