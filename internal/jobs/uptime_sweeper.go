package jobs

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/karloscodes/cartridge"
	"gorm.io/gorm"

	"lognorth/internal/apikeys"
	"lognorth/internal/events"
	"lognorth/internal/settings"
	"lognorth/internal/uptime"
)

// UptimeSweeper actively probes each app's URL every minute and alerts on
// transitions (up -> down, down -> up). The URL is set when the app is
// created or via the [edit] UI. Apps without a URL are skipped.
type UptimeSweeper struct {
	client *http.Client
}

// NewUptimeSweeper creates a new uptime sweeper with a sensible HTTP client.
func NewUptimeSweeper() *UptimeSweeper {
	return &UptimeSweeper{
		client: &http.Client{
			Timeout: probeTimeout,
			// Don't follow redirects past one hop. If your homepage 301s
			// to a CDN, that's still "up" — but more than one hop is a
			// red flag worth surfacing.
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				if len(via) >= 2 {
					return http.ErrUseLastResponse
				}
				return nil
			},
		},
	}
}

const (
	probeTimeout     = 5 * time.Second
	failureThreshold = 3
	bootstrapWindow  = 10 * time.Minute
	pingUserAgent    = "LogNorth Uptime/1.0 (+https://lognorth.com)"
)

// ProcessBatch probes the configured URL for all eligible apps.
func (s *UptimeSweeper) ProcessBatch(ctx *cartridge.JobContext) error {
	db := ctx.DB

	var keys []apikeys.APIKey
	if err := db.
		Where("active = ? AND ping_disabled = ?", true, false).
		Find(&keys).Error; err != nil {
		return err
	}

	if len(keys) == 0 {
		return nil
	}

	settingsRow, _ := settings.Get(db)
	now := time.Now()

	for i := range keys {
		s.checkApp(db, &keys[i], settingsRow, now)
	}

	return nil
}

// checkApp probes the configured URL and applies state transitions.
func (s *UptimeSweeper) checkApp(db *gorm.DB, key *apikeys.APIKey, settingsRow *settings.Settings, now time.Time) {
	// Bootstrap grace: don't alert during the first 10 minutes after an
	// app is created. Avoids paging on apps that haven't been deployed.
	if now.Sub(key.CreatedAt) < bootstrapWindow {
		return
	}

	if key.PingURL == "" {
		return // no URL set; user adds one via [edit]
	}

	check := s.probe(key.PingURL)
	check.APIKeyID = key.ID
	check.CheckedAt = now
	// Every probe is kept so the app can show response times and failures.
	_ = uptime.Record(db, &check)
	up := check.Up
	key.LastPingedAt = now

	switch {
	case !up:
		key.ConsecutiveFails++
		if key.ConsecutiveFails >= failureThreshold && !key.IsDown {
			key.IsDown = true
			key.DownSince = now
		}
		// Retried every sweep until it goes out: an outage that starts in
		// quiet hours alerts when they end.
		if key.IsDown && !key.DownNotified {
			key.DownNotified = s.notifyDown(settingsRow, key)
		}
	case up && key.IsDown:
		// Recovered.
		downFor := now.Sub(key.DownSince)
		key.IsDown = false
		key.DownNotified = false
		key.DownSince = time.Time{}
		key.ConsecutiveFails = 0
		s.notifyRecovered(settingsRow, key, downFor)
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

// probe requests the URL once. It is up if the server answered with anything
// below 500 within the timeout: a 4xx still means the server is responding.
func (s *UptimeSweeper) probe(url string) uptime.Check {
	ctx, cancel := context.WithTimeout(context.Background(), probeTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return uptime.Check{Error: "invalid URL"}
	}
	req.Header.Set("User-Agent", pingUserAgent)

	start := time.Now()
	resp, err := s.client.Do(req)
	latency := int(time.Since(start).Milliseconds())
	if err != nil {
		return uptime.Check{LatencyMS: latency, Error: probeError(err)}
	}
	defer resp.Body.Close()

	return uptime.Check{Up: resp.StatusCode < 500, Status: resp.StatusCode, LatencyMS: latency}
}

// probeError says why a probe got no response, in a few words.
func probeError(err error) string {
	if errors.Is(err, context.DeadlineExceeded) {
		return fmt.Sprintf("no answer within %s", probeTimeout)
	}
	msg := err.Error()
	if len(msg) > 200 {
		msg = msg[:200]
	}
	return msg
}

// notifyDown reports whether the alert went out.
func (s *UptimeSweeper) notifyDown(settingsRow *settings.Settings, key *apikeys.APIKey) bool {
	if settingsRow == nil || !settingsRow.CanNotify() {
		return false
	}
	subject := fmt.Sprintf("%s — down: %s", key.Name, key.PingURL)
	body := fmt.Sprintf(`%s isn't responding.
URL: %s
3 consecutive failures.

%s/apps/%d/issues`,
		key.Name,
		key.PingURL,
		settingsRow.BaseURL,
		key.ID,
	)
	return settingsRow.SendNotification(subject, body) == nil
}

func (s *UptimeSweeper) notifyRecovered(settingsRow *settings.Settings, key *apikeys.APIKey, downFor time.Duration) {
	if settingsRow == nil || !settingsRow.CanNotify() {
		return
	}
	subject := fmt.Sprintf("%s — recovered after %s down", key.Name, events.FormatDuration(downFor))
	body := fmt.Sprintf(`%s is responding again.
URL: %s
Was down for %s.

%s/apps/%d/issues`,
		key.Name,
		key.PingURL,
		events.FormatDuration(downFor),
		settingsRow.BaseURL,
		key.ID,
	)
	_ = settingsRow.SendNotification(subject, body)
}
