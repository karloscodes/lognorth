package status

import (
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/karloscodes/cartridge/sqlite"
	"gorm.io/gorm"

	"lognorth/internal/apikeys"
	"lognorth/internal/uptime"
)

var now = time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)

func newTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	manager := sqlite.NewManager(sqlite.Config{Path: filepath.Join(t.TempDir(), "test.db"), EnableWAL: true})
	db, err := manager.Connect()
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	if err := db.AutoMigrate(&apikeys.APIKey{}, &uptime.Check{}, &uptime.Day{}, &Page{}, &Comment{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

// newApp creates an app on the page, or on no page with 0.
func newApp(t *testing.T, db *gorm.DB, name string, pageID uint) apikeys.APIKey {
	t.Helper()
	key, _, err := apikeys.Create(db, name)
	if err != nil {
		t.Fatalf("create app: %v", err)
	}
	db.Model(key).Updates(map[string]any{"ping_url": "https://" + name + ".example/up", "status_page_id": pageID})
	var fresh apikeys.APIKey
	db.First(&fresh, key.ID)
	return fresh
}

func newPage(t *testing.T, db *gorm.DB, name, domain string) Page {
	t.Helper()
	page, err := CreatePage(db, name, domain, nil)
	if err != nil {
		t.Fatalf("create page: %v", err)
	}
	return page
}

// pings records one ping a minute from `from`: + is up, - is down.
func pings(db *gorm.DB, app apikeys.APIKey, from time.Time, pattern string) {
	for i, ch := range pattern {
		db.Create(&uptime.Check{APIKeyID: app.ID, CheckedAt: from.Add(time.Duration(i) * time.Minute), Up: ch == '+', Status: 200})
	}
}

func TestBuild(t *testing.T) {
	t.Run("shows only the apps on that page", func(t *testing.T) {
		db := newTestDB(t)
		shop := newPage(t, db, "Shop", "")
		blog := newPage(t, db, "Blog", "")
		newApp(t, db, "shop-web", shop.ID)
		newApp(t, db, "admin", 0)
		newApp(t, db, "shop-api", shop.ID)
		newApp(t, db, "blog-web", blog.ID)

		report := Build(db, shop, now)

		if len(report.Apps) != 2 || report.Apps[0].Name != "shop-api" || report.Apps[1].Name != "shop-web" {
			t.Fatalf("expected shop-api and shop-web, got %+v", report.Apps)
		}
		if len(report.Apps[0].Days) != 90 || report.Apps[0].Days[89].Date != "2026-01-15" {
			t.Errorf("expected 90 days ending today, got %d", len(report.Apps[0].Days))
		}
	})

	t.Run("each outage is an incident, newest first, with its notes", func(t *testing.T) {
		db := newTestDB(t)
		page := newPage(t, db, "Shop", "")
		shop := newApp(t, db, "shop", page.ID)
		pings(db, shop, now.Add(-3*time.Hour), "+---+")
		pings(db, shop, now.Add(-time.Hour), "+----+")
		lastStart := now.Add(-time.Hour + time.Minute)
		if err := AddComment(db, shop.ID, lastStart, "  Disk full. Grew the volume.  ", now); err != nil {
			t.Fatalf("add comment: %v", err)
		}

		report := Build(db, page, now)

		if len(report.Incidents) != 2 {
			t.Fatalf("expected 2 incidents, got %+v", report.Incidents)
		}
		latest, earlier := report.Incidents[0], report.Incidents[1]
		if !latest.Start.Equal(lastStart) || !latest.End.Equal(now.Add(-time.Hour+5*time.Minute)) || latest.AppName != "shop" {
			t.Errorf("expected the latest outage first, got %+v", latest)
		}
		if len(latest.Comments) != 1 || latest.Comments[0].Body != "Disk full. Grew the volume." {
			t.Errorf("expected the note on the latest outage, got %+v", latest.Comments)
		}
		if len(earlier.Comments) != 0 {
			t.Errorf("expected no note on the earlier outage, got %+v", earlier.Comments)
		}
	})
}

func TestAddComment(t *testing.T) {
	t.Run("needs an outage that is on the page", func(t *testing.T) {
		db := newTestDB(t)
		page := newPage(t, db, "Shop", "")
		shop := newApp(t, db, "shop", page.ID)
		hidden := newApp(t, db, "admin", 0)
		pings(db, shop, now.Add(-time.Hour), "+---+")
		pings(db, hidden, now.Add(-time.Hour), "+---+")
		start := now.Add(-time.Hour + time.Minute)

		onPage := AddComment(db, shop.ID, start, "Back up.", now)
		notAnOutage := AddComment(db, shop.ID, start.Add(time.Minute), "Back up.", now)
		offPage := AddComment(db, hidden.ID, start, "Back up.", now)

		if onPage != nil {
			t.Errorf("expected the note on the outage to save, got %v", onPage)
		}
		if !errors.Is(notAnOutage, ErrNoOutage) || !errors.Is(offPage, ErrNoOutage) {
			t.Errorf("expected ErrNoOutage, got %v and %v", notAnOutage, offPage)
		}
	})

	t.Run("refuses an empty note", func(t *testing.T) {
		db := newTestDB(t)
		page := newPage(t, db, "Shop", "")
		shop := newApp(t, db, "shop", page.ID)
		pings(db, shop, now.Add(-time.Hour), "+---+")

		err := AddComment(db, shop.ID, now.Add(-time.Hour+time.Minute), "   ", now)

		if !errors.Is(err, ErrCommentEmpty) {
			t.Errorf("expected ErrCommentEmpty, got %v", err)
		}
	})
}

func TestDeleteOlderThan(t *testing.T) {
	db := newTestDB(t)
	db.Create(&Comment{APIKeyID: 1, OutageStart: now.Add(-40 * 24 * time.Hour), Body: "old"})
	db.Create(&Comment{APIKeyID: 1, OutageStart: now.Add(-time.Hour), Body: "new"})

	deleted, err := DeleteOlderThan(db, now.Add(-uptime.Retention))

	var left []Comment
	db.Find(&left)
	if err != nil || deleted != 1 || len(left) != 1 || left[0].Body != "new" {
		t.Errorf("expected only the new note left, got %d deleted, %+v, err %v", deleted, left, err)
	}
}
