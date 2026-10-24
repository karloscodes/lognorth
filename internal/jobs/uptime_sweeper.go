package jobs

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/karloscodes/cartridge"

	"lognorth/internal/alerts"
	"lognorth/internal/apikeys"
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
		alerts.ApplyPing(db, key, checks[i], settingsRow, now)
	}
	return nil
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
