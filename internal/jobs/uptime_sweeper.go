package jobs

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/karloscodes/cartridge"
	"gorm.io/gorm"

	"lognorth/internal/apikeys"
	"lognorth/internal/settings"
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
	probeTimeout      = 5 * time.Second
	failureThreshold  = 3
	bootstrapWindow   = 10 * time.Minute
	pingUserAgent     = "LogNorth Uptime/1.0 (+https://lognorth.com)"
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

	up := s.probe(key.PingURL)
	key.LastPingedAt = now

	switch {
	case !up:
		key.ConsecutiveFails++
		if key.ConsecutiveFails >= failureThreshold && !key.IsDown {
			key.IsDown = true
			key.DownSince = now
			s.notifyDown(settingsRow, key)
		}
	case up && key.IsDown:
		// Recovered.
		downFor := now.Sub(key.DownSince)
		key.IsDown = false
		key.DownSince = time.Time{}
		key.ConsecutiveFails = 0
		s.notifyRecovered(settingsRow, key, downFor)
	default:
		// Healthy probe; reset the fail counter (handles flap).
		key.ConsecutiveFails = 0
	}

	db.Save(key)
}

// probe returns true if the URL responded with anything other than a
// 5xx or a transport error within the timeout. 4xx is "up" — the server
// is responding.
func (s *UptimeSweeper) probe(url string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), probeTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return false
	}
	req.Header.Set("User-Agent", pingUserAgent)

	resp, err := s.client.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()

	return resp.StatusCode < 500
}

func (s *UptimeSweeper) notifyDown(settingsRow *settings.Settings, key *apikeys.APIKey) {
	if settingsRow == nil || !settingsRow.CanNotify() {
		return
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
	_ = settingsRow.SendNotification(subject, body)
}

func (s *UptimeSweeper) notifyRecovered(settingsRow *settings.Settings, key *apikeys.APIKey, downFor time.Duration) {
	if settingsRow == nil || !settingsRow.CanNotify() {
		return
	}
	subject := fmt.Sprintf("%s — recovered after %d min down", key.Name, int(downFor.Minutes()))
	body := fmt.Sprintf(`%s is responding again.
URL: %s
Was down for %d min.

%s/apps/%d/issues`,
		key.Name,
		key.PingURL,
		int(downFor.Minutes()),
		settingsRow.BaseURL,
		key.ID,
	)
	_ = settingsRow.SendNotification(subject, body)
}
