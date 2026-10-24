package jobs

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sync"
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
	probeTimeout    = 5 * time.Second
	bootstrapWindow = 10 * time.Minute
	pingUserAgent   = "LogNorth Uptime/1.0 (+https://lognorth.com)"
)

// maxParallelProbes caps the probes in flight. Each one can wait
// probeTimeout, so one after another, twelve apps that time out would take
// the whole minute between sweeps.
const maxParallelProbes = 16

// ProcessBatch probes the URL of every app at the same time, then applies
// the results one app at a time.
func (s *UptimeSweeper) ProcessBatch(ctx *cartridge.JobContext) error {
	db := ctx.DB

	var keys []apikeys.APIKey
	if err := db.
		Where("active = ? AND ping_disabled = ?", true, false).
		Find(&keys).Error; err != nil {
		return err
	}

	now := time.Now()
	var due []*apikeys.APIKey
	for i := range keys {
		// Bootstrap grace: no probe in the first 10 minutes after an app is
		// created, so an app that is not deployed yet does not page.
		if keys[i].PingURL != "" && now.Sub(keys[i].CreatedAt) >= bootstrapWindow {
			due = append(due, &keys[i])
		}
	}
	if len(due) == 0 {
		return nil
	}

	checks := make([]uptime.Check, len(due))
	slots := make(chan struct{}, maxParallelProbes)
	var wg sync.WaitGroup
	for i, key := range due {
		slots <- struct{}{}
		wg.Go(func() {
			defer func() { <-slots }()
			checks[i] = s.probe(key.PingURL)
		})
	}
	wg.Wait()

	settingsRow, _ := settings.Get(db)
	for i, key := range due {
		s.apply(db, key, checks[i], settingsRow, now)
	}
	return nil
}

// apply records a probe and applies the state transitions.
func (s *UptimeSweeper) apply(db *gorm.DB, key *apikeys.APIKey, check uptime.Check, settingsRow *settings.Settings, now time.Time) {
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
			key.DownNotified = s.notifyDown(settingsRow, key, surgesNow(db, key.ID, now))
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
func (s *UptimeSweeper) notifyDown(settingsRow *settings.Settings, key *apikeys.APIKey, surges []events.Surge) bool {
	if settingsRow == nil || !settingsRow.CanNotify() {
		return false
	}
	subject := fmt.Sprintf("%s — down: %s", key.Name, key.PingURL)
	return settingsRow.SendNotification(subject, downBody(key, settingsRow.BaseURL, surges)) == nil
}

// downBody says the app is down and, when some issues surged as it went
// down, names up to three of them. They are the likely cause, so the agent
// command points at the first. With no surge it says nothing about errors.
func downBody(key *apikeys.APIKey, baseURL string, surges []events.Surge) string {
	body := fmt.Sprintf("%s isn't responding.\nURL: %s\n%d consecutive failures.\n", key.Name, key.PingURL, uptime.FailuresToDown)
	if len(surges) > 0 {
		body += "\nErrors that surged as it went down:\n"
		for _, sg := range surges[:min(3, len(surges))] {
			body += fmt.Sprintf("  %dx %s\n  %s/issues/%s\n", sg.Count, events.FormatHeadline(sg.ErrorClass, sg.Error), baseURL, sg.Hash)
		}
		body += "\nAsk your agent: /lognorth:investigate " + surges[0].Hash + "\n"
	}
	return body + fmt.Sprintf("\n%s/apps/%d/endpoints", baseURL, key.ID)
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
