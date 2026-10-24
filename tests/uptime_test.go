package tests

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/karloscodes/cartridge"

	"lognorth/internal/alerts"
	"lognorth/internal/apikeys"
	"lognorth/internal/jobs"
	"lognorth/internal/settings"
	"lognorth/internal/uptime"
)

func TestUptimeAlertsAfterQuietHours(t *testing.T) {
	db, inbox := setupErrorAlerts(t)
	broken := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	t.Cleanup(broken.Close)

	key, _, _ := apikeys.Create(db, "shop")
	db.Model(key).Updates(map[string]any{"ping_url": broken.URL, "created_at": time.Now().Add(-time.Hour)})

	// Quiet hours cover the current hour.
	s, _ := settings.Get(db)
	s.QuietHoursEnabled = true
	s.TimeZone = "UTC"
	s.QuietStart = time.Now().UTC().Hour()
	s.QuietEnd = (time.Now().UTC().Hour() + 1) % 24
	settings.Update(db, s)

	sweeper := jobs.NewUptimeSweeper()
	ctx := &cartridge.JobContext{Context: t.Context(), Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), DB: db}
	for range 3 {
		sweeper.ProcessBatch(ctx)
	}
	alerts.Deliver(db, time.Now().Add(time.Minute))
	duringQuiet := inbox.received()

	s.QuietHoursEnabled = false
	settings.Update(db, s)
	sweeper.ProcessBatch(ctx)
	sweeper.ProcessBatch(ctx)
	alerts.Deliver(db, time.Now().Add(2*time.Minute))

	if len(duringQuiet) != 0 {
		t.Errorf("alerts during quiet hours = %q, want none", duringQuiet)
	}
	got := inbox.received()
	if len(got) != 1 || !strings.Contains(got[0], "down") {
		t.Errorf("alerts after quiet hours = %q, want exactly one down alert", got)
	}
}

func TestUptimeRecordsEachCheck(t *testing.T) {
	probeOnce := func(t *testing.T, url string) uptime.Check {
		t.Helper()
		db, _ := setupErrorAlerts(t)
		key, _, _ := apikeys.Create(db, "shop")
		db.Model(key).Updates(map[string]any{"ping_url": url, "created_at": time.Now().Add(-time.Hour)})
		ctx := &cartridge.JobContext{Context: t.Context(), Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), DB: db}

		jobs.NewUptimeSweeper().ProcessBatch(ctx)

		var checks []uptime.Check
		db.Where("api_key_id = ?", key.ID).Find(&checks)
		if len(checks) != 1 {
			t.Fatalf("expected 1 check, got %d", len(checks))
		}
		return checks[0]
	}

	t.Run("a healthy URL records up with its status and response time", func(t *testing.T) {
		ok := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
		t.Cleanup(ok.Close)

		check := probeOnce(t, ok.URL)

		if !check.Up || check.Status != 200 || check.LatencyMS < 0 || check.CheckedAt.IsZero() {
			t.Errorf("expected an up 200 check, got %+v", check)
		}
	})

	t.Run("a 5xx records down with the status", func(t *testing.T) {
		broken := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadGateway)
		}))
		t.Cleanup(broken.Close)

		check := probeOnce(t, broken.URL)

		if check.Up || check.Status != 502 {
			t.Errorf("expected a down 502 check, got %+v", check)
		}
	})

	t.Run("no answer records down with the reason", func(t *testing.T) {
		gone := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
		gone.Close()

		check := probeOnce(t, gone.URL)

		if check.Up || check.Status != 0 || check.Error == "" {
			t.Errorf("expected a down check with an error, got %+v", check)
		}
	})
}
