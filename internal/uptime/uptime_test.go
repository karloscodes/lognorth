package uptime

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/karloscodes/cartridge/sqlite"
	"gorm.io/gorm"

	"lognorth/internal/apikeys"
)

func newTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	manager := sqlite.NewManager(sqlite.Config{Path: filepath.Join(t.TempDir(), "test.db"), EnableWAL: true})
	db, err := manager.Connect()
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	if err := db.AutoMigrate(&apikeys.APIKey{}, &Check{}, &Day{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

var now = time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)

func newApp(t *testing.T, db *gorm.DB, pingURL string) apikeys.APIKey {
	t.Helper()
	key, _, err := apikeys.Create(db, "shop")
	if err != nil {
		t.Fatalf("create app: %v", err)
	}
	db.Model(key).Update("ping_url", pingURL)
	var fresh apikeys.APIKey
	db.First(&fresh, key.ID)
	return fresh
}

func record(t *testing.T, db *gorm.DB, app apikeys.APIKey, ago time.Duration, up bool, latency int) {
	t.Helper()
	check := &Check{APIKeyID: app.ID, CheckedAt: now.Add(-ago), Up: up, Status: 200, LatencyMS: latency}
	if !up {
		check.Status = 503
	}
	if err := Record(db, check); err != nil {
		t.Fatalf("record: %v", err)
	}
}

func TestStates(t *testing.T) {
	t.Run("an app with passing checks is up, with its latest latency and 24h uptime", func(t *testing.T) {
		db := newTestDB(t)
		app := newApp(t, db, "https://shop.example")
		record(t, db, app, 3*time.Minute, false, 5000)
		record(t, db, app, 2*time.Minute, true, 120)
		record(t, db, app, time.Minute, true, 84)
		record(t, db, app, 30*time.Hour, false, 0) // outside the 24h window

		state := States(db, []apikeys.APIKey{app}, now)[app.ID]

		if state.Status != "up" || state.LatencyMS != 84 {
			t.Errorf("expected up at 84ms, got %s at %dms", state.Status, state.LatencyMS)
		}
		if state.Uptime24h == nil || int(*state.Uptime24h) != 66 {
			t.Errorf("expected 66%% uptime over 24h, got %v", state.Uptime24h)
		}
	})

	t.Run("an app without a URL has no uptime", func(t *testing.T) {
		db := newTestDB(t)
		app := newApp(t, db, "")

		state := States(db, []apikeys.APIKey{app}, now)[app.ID]

		if state.Status != "none" || state.Uptime24h != nil {
			t.Errorf("expected none without a percent, got %s %v", state.Status, state.Uptime24h)
		}
	})

	t.Run("a URL that was never probed is pending", func(t *testing.T) {
		db := newTestDB(t)
		app := newApp(t, db, "https://shop.example")

		state := States(db, []apikeys.APIKey{app}, now)[app.ID]

		if state.Status != "pending" {
			t.Errorf("expected pending, got %s", state.Status)
		}
	})

	t.Run("a down app says since when", func(t *testing.T) {
		db := newTestDB(t)
		app := newApp(t, db, "https://shop.example")
		app.IsDown = true
		app.DownSince = now.Add(-12 * time.Minute)
		record(t, db, app, time.Minute, false, 5000)

		state := States(db, []apikeys.APIKey{app}, now)[app.ID]

		if state.Status != "down" || state.DownSince == nil || !state.DownSince.Equal(app.DownSince) {
			t.Errorf("expected down since %v, got %s %v", app.DownSince, state.Status, state.DownSince)
		}
	})
}

func TestReportFor(t *testing.T) {
	t.Run("splits the window into bars and keeps the latest failure", func(t *testing.T) {
		db := newTestDB(t)
		app := newApp(t, db, "https://shop.example")
		record(t, db, app, 23*time.Hour+55*time.Minute, true, 90) // first bar
		record(t, db, app, 10*time.Minute, false, 5000)           // last bar
		record(t, db, app, 5*time.Minute, true, 80)               // last bar

		report := ReportFor(db, app, now, 24*time.Hour, 96)

		if len(report.Bars) != 96 {
			t.Fatalf("expected 96 bars, got %d", len(report.Bars))
		}
		if report.Bars[0].Checks != 1 || report.Bars[95].Checks != 2 || report.Bars[95].Fails != 1 {
			t.Errorf("unexpected bars: first %+v, last %+v", report.Bars[0], report.Bars[95])
		}
		if report.LastFailure == nil || report.LastFailure.Status != 503 {
			t.Errorf("expected the 503 as last failure, got %+v", report.LastFailure)
		}
	})

	t.Run("each bar says why its pings failed and how fast the rest answered", func(t *testing.T) {
		db := newTestDB(t)
		app := newApp(t, db, "https://shop.example")
		record(t, db, app, 4*time.Minute, false, 1204) // 503
		record(t, db, app, 3*time.Minute, false, 900)  // 503
		Record(db, &Check{APIKeyID: app.ID, CheckedAt: now.Add(-2 * time.Minute), Error: "no answer within 5s", LatencyMS: 5000})
		record(t, db, app, time.Minute, true, 80)
		record(t, db, app, 30*time.Second, true, 100)

		bar := ReportFor(db, app, now, 24*time.Hour, 96).Bars[95]

		want := []Reason{{Label: "503", Count: 2}, {Label: "no answer within 5s", Count: 1}}
		if len(bar.Failures) != 2 || bar.Failures[0] != want[0] || bar.Failures[1] != want[1] {
			t.Errorf("expected %v, got %v", want, bar.Failures)
		}
		if bar.LatencyMS != 90 {
			t.Errorf("expected 90ms average over the successful pings, got %d", bar.LatencyMS)
		}
	})
}

func TestDeleteOlderThan(t *testing.T) {
	db := newTestDB(t)
	app := newApp(t, db, "https://shop.example")
	record(t, db, app, 31*24*time.Hour, true, 90)
	record(t, db, app, time.Hour, true, 90)

	deleted, err := DeleteOlderThan(db, now.Add(-Retention))

	if err != nil || deleted != 1 {
		t.Errorf("expected 1 deleted, got %d (%v)", deleted, err)
	}
}
