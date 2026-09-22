package jobs

import (
	"context"
	"io"
	"log/slog"
	"path/filepath"
	"testing"

	"github.com/karloscodes/cartridge"
	"github.com/karloscodes/cartridge/sqlite"
	"gorm.io/gorm"

	"lognorth/internal/apikeys"
	"lognorth/internal/database"
	"lognorth/internal/events"
	"lognorth/internal/license"
)

func setupProcessorDB(t *testing.T) *gorm.DB {
	t.Helper()

	manager := sqlite.NewManager(sqlite.Config{Path: filepath.Join(t.TempDir(), "test.db"), EnableWAL: true})
	db, err := manager.Connect()
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	if err := database.Migrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

func seedIngested(t *testing.T, db *gorm.DB) uint {
	t.Helper()
	key, _, err := apikeys.Create(db, "Test App")
	if err != nil {
		t.Fatalf("create api key: %v", err)
	}
	if err := events.CreateIngested(db, []*events.IngestedEvent{{
		APIKeyID: key.ID,
		Message:  "GET /healthz",
		Context:  `{"method":"GET","path":"/healthz","status":200}`,
	}}); err != nil {
		t.Fatalf("create ingested: %v", err)
	}
	return key.ID
}

func newJobCtx(db *gorm.DB) *cartridge.JobContext {
	return &cartridge.JobContext{
		Context: context.Background(),
		Logger:  slog.New(slog.NewTextHandler(io.Discard, nil)),
		DB:      db,
	}
}

func TestEventProcessor_SkipsWhenLicenseMissing(t *testing.T) {
	db := setupProcessorDB(t)
	seedIngested(t, db)

	prev := license.IsConfigured()
	license.SetConfigured(false)
	t.Cleanup(func() { license.SetConfigured(prev) })

	if err := NewEventProcessor().ProcessBatch(newJobCtx(db)); err != nil {
		t.Fatalf("ProcessBatch: %v", err)
	}

	var unprocessed int64
	if err := db.Model(&events.IngestedEvent{}).Count(&unprocessed).Error; err != nil {
		t.Fatalf("count unprocessed: %v", err)
	}
	if unprocessed != 1 {
		t.Fatalf("expected 1 unprocessed event (license blocks processing), got %d", unprocessed)
	}

	var processed int64
	if err := db.Model(&events.Event{}).Count(&processed).Error; err != nil {
		t.Fatalf("count events: %v", err)
	}
	if processed != 0 {
		t.Fatalf("expected 0 processed events, got %d", processed)
	}
}

func TestEventProcessor_ProcessesWhenLicensed(t *testing.T) {
	db := setupProcessorDB(t)
	seedIngested(t, db)

	prev := license.IsConfigured()
	license.SetConfigured(true)
	t.Cleanup(func() { license.SetConfigured(prev) })

	if err := NewEventProcessor().ProcessBatch(newJobCtx(db)); err != nil {
		t.Fatalf("ProcessBatch: %v", err)
	}

	var unprocessed int64
	if err := db.Model(&events.IngestedEvent{}).Count(&unprocessed).Error; err != nil {
		t.Fatalf("count unprocessed: %v", err)
	}
	if unprocessed != 0 {
		t.Fatalf("expected 0 unprocessed events, got %d", unprocessed)
	}

	var processed int64
	if err := db.Model(&events.Event{}).Count(&processed).Error; err != nil {
		t.Fatalf("count events: %v", err)
	}
	if processed != 1 {
		t.Fatalf("expected 1 processed event, got %d", processed)
	}
}
