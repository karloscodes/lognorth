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

	"lognorth/internal/apikeys"
	"lognorth/internal/jobs"
	"lognorth/internal/settings"
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
	s.QuietStart = time.Now().Hour()
	s.QuietEnd = (time.Now().Hour() + 1) % 24
	settings.Update(db, s)

	sweeper := jobs.NewUptimeSweeper()
	ctx := &cartridge.JobContext{Context: t.Context(), Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), DB: db}
	for range 3 {
		sweeper.ProcessBatch(ctx)
	}
	duringQuiet := inbox.received()

	s.QuietHoursEnabled = false
	settings.Update(db, s)
	sweeper.ProcessBatch(ctx)
	sweeper.ProcessBatch(ctx)

	if len(duringQuiet) != 0 {
		t.Errorf("alerts during quiet hours = %q, want none", duringQuiet)
	}
	got := inbox.received()
	if len(got) != 1 || !strings.Contains(got[0], "down") {
		t.Errorf("alerts after quiet hours = %q, want exactly one down alert", got)
	}
}
