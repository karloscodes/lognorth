package database

import (
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/karloscodes/cartridge/sqlite"
	"gorm.io/gorm"

	"lognorth/internal/alerts"
	"lognorth/internal/events"
)

func migratedDB(t *testing.T) *gorm.DB {
	t.Helper()
	manager := sqlite.NewManager(sqlite.Config{Path: filepath.Join(t.TempDir(), "test.db"), EnableWAL: true})
	db, err := manager.Connect()
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	if err := Migrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

func TestRekeyEndpoints(t *testing.T) {
	bucket := time.Date(2026, 10, 7, 12, 0, 0, 0, time.UTC)
	next := bucket.Add(5 * time.Minute)
	stat := func(db *gorm.DB, path string, at time.Time, total, errors int64, avg int) {
		t.Helper()
		if err := db.Create(&events.EndpointStat{APIKeyID: 1, Path: path, Bucket: at, Total: total, Errors: errors, AvgDuration: avg}).Error; err != nil {
			t.Fatal(err)
		}
	}

	t.Run("merges the URLs of an endpoint, and keeps the totals exact", func(t *testing.T) {
		db := migratedDB(t)
		stat(db, "/users/1", bucket, 10, 1, 100)
		stat(db, "/users/2", bucket, 30, 3, 200)
		stat(db, "/users/2", next, 5, 0, 40)
		stat(db, "/checkout", bucket, 50, 0, 80)

		if err := rekeyEndpoints(db); err != nil {
			t.Fatalf("rekeyEndpoints: %v", err)
		}

		var rows []events.EndpointStat
		db.Order("path, bucket").Find(&rows)
		got := ""
		for _, r := range rows {
			got += fmt.Sprintf("%s %s %d %d %d; ", r.Path, r.Bucket.Format("15:04"), r.Total, r.Errors, r.AvgDuration)
		}
		// (10×100 + 30×200) / 40 = 175
		want := "/checkout 12:00 50 0 80; /users/:id 12:00 40 4 175; /users/:id 12:05 5 0 40; "
		if got != want {
			t.Errorf("rows = %s\nwant   %s", got, want)
		}
	})

	t.Run("uses the route the SDK sent for a path", func(t *testing.T) {
		db := migratedDB(t)
		stat(db, "/posts/hello", bucket, 10, 0, 10)
		stat(db, "/posts/world", bucket, 20, 2, 10)
		for _, path := range []string{"/posts/hello", "/posts/world"} {
			events.Create(db, &events.Event{APIKeyID: 1, Message: "GET", Context: `{"path":"` + path + `","route":"/posts/:slug"}`})
		}

		if err := rekeyEndpoints(db); err != nil {
			t.Fatalf("rekeyEndpoints: %v", err)
		}

		var row events.EndpointStat
		db.First(&row)
		var count int64
		db.Model(&events.EndpointStat{}).Count(&count)
		if count != 1 || row.Path != "/posts/:slug" || row.Total != 30 || row.Errors != 2 {
			t.Errorf("%d rows, first %+v; want one /posts/:slug row with 30 requests and 2 errors", count, row)
		}
	})

	t.Run("merges the alert states, keeping the strongest", func(t *testing.T) {
		db := migratedDB(t)
		alerted := time.Now().UTC().Add(-20 * time.Minute)
		db.Create(&alerts.SpikeState{APIKeyID: 1, Path: "/users/1", Severity: "critical", LastAlertAt: alerted, SpikeStartAt: alerted, ErrorCount: 5})
		db.Create(&alerts.SpikeState{APIKeyID: 1, Path: "/users/2", Severity: "warning", ErrorCount: 3, CleanChecks: 1})
		db.Create(&alerts.SpikeState{APIKeyID: 1, Path: "/checkout", Severity: "warning"})
		db.Create(&alerts.SilenceState{APIKeyID: 1, Path: "/users/9", Severity: "critical", LastAlertAt: alerted})

		if err := rekeyEndpoints(db); err != nil {
			t.Fatalf("rekeyEndpoints: %v", err)
		}

		var spikes []alerts.SpikeState
		db.Order("path").Find(&spikes)
		if len(spikes) != 2 || spikes[0].Path != "/checkout" || spikes[1].Path != "/users/:id" {
			t.Fatalf("spike states = %+v, want /checkout and /users/:id", spikes)
		}
		merged := spikes[1]
		if merged.Severity != "critical" || merged.ErrorCount != 8 || merged.CleanChecks != 0 || !merged.LastAlertAt.Equal(alerted) {
			t.Errorf("merged = %+v, want critical, 8 errors, 0 clean checks, and the last alert kept", merged)
		}
		var silence alerts.SilenceState
		db.First(&silence)
		if silence.Path != "/users/:id" || silence.Severity != "critical" {
			t.Errorf("silence = %+v, want critical on /users/:id", silence)
		}
	})

	t.Run("changes nothing when every path is its own endpoint", func(t *testing.T) {
		db := migratedDB(t)
		stat(db, "/checkout", bucket, 50, 0, 80)

		if err := rekeyEndpoints(db); err != nil {
			t.Fatalf("rekeyEndpoints: %v", err)
		}

		var row events.EndpointStat
		db.First(&row)
		if row.Path != "/checkout" || row.Total != 50 || row.ID != 1 {
			t.Errorf("row = %+v, want the same /checkout row", row)
		}
	})
}
