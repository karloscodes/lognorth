package jobs

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/karloscodes/cartridge"
	"gorm.io/gorm"

	"lognorth/internal/apikeys"
	"lognorth/internal/settings"
)

// UptimeSweeper actively probes each app's URL every minute and alerts on
// transitions (up -> down, down -> up). CoC: no per-app config; URL is
// auto-discovered from the most recent event's `context.host` and the
// user can override or opt out.
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

// ProcessBatch runs auto-discovery and active probes for all eligible apps.
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

// checkApp resolves the URL (auto-discovering if needed), probes it, and
// applies state transitions.
func (s *UptimeSweeper) checkApp(db *gorm.DB, key *apikeys.APIKey, settingsRow *settings.Settings, now time.Time) {
	// Bootstrap grace: don't alert during the first 10 minutes after an
	// app is created. Avoids paging on apps that haven't been deployed.
	if now.Sub(key.CreatedAt) < bootstrapWindow {
		return
	}

	if key.PingURL == "" {
		// Try auto-discovery from a recent event's host header.
		if url := discoverURL(db, key.ID); url != "" {
			key.PingURL = url
			db.Model(key).Update("ping_url", url)
		} else {
			return // no URL yet — wait for first event with a host
		}
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

// discoverURL pulls the most recent event's host header for this app and
// returns it as a URL. Skips internal/loopback hosts that aren't useful
// to ping from outside the user's network.
func discoverURL(db *gorm.DB, apiKeyID uint) string {
	var host string
	db.Raw(`
		SELECT json_extract(context, '$.host')
		FROM events
		WHERE api_key_id = ?
			AND json_extract(context, '$.host') IS NOT NULL
			AND json_extract(context, '$.host') != ''
		ORDER BY timestamp DESC
		LIMIT 1
	`, apiKeyID).Scan(&host)

	host = strings.TrimSpace(host)
	if host == "" {
		return ""
	}
	if isInternalHost(host) {
		return ""
	}

	// Default to https. If the user explicitly wants http, they can
	// override via the settings UI.
	return "https://" + host
}

// isInternalHost returns true for hosts we shouldn't auto-ping (loopback,
// RFC1918-ish, .local, .internal). User can still set these manually.
func isInternalHost(host string) bool {
	h := strings.ToLower(host)
	// Strip port if present.
	if i := strings.IndexByte(h, ':'); i >= 0 {
		h = h[:i]
	}
	switch {
	case h == "localhost",
		strings.HasPrefix(h, "127."),
		strings.HasPrefix(h, "10."),
		strings.HasPrefix(h, "192.168."),
		strings.HasSuffix(h, ".local"),
		strings.HasSuffix(h, ".internal"):
		return true
	}
	// 172.16.0.0 — 172.31.255.255 (RFC1918)
	if strings.HasPrefix(h, "172.") {
		var second int
		fmt.Sscanf(h, "172.%d.", &second)
		if second >= 16 && second <= 31 {
			return true
		}
	}
	return false
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
