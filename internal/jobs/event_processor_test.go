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

func TestEventProcessor_ProcessesWithoutLicense(t *testing.T) {
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

func TestEventProcessor_BatchWithFailingEvent(t *testing.T) {
	db := setupProcessorDB(t)
	key, _, err := apikeys.Create(db, "Test App")
	if err != nil {
		t.Fatalf("create api key: %v", err)
	}
	seed := []*events.IngestedEvent{
		{APIKeyID: key.ID, Message: "GET /a", Context: `{"path":"/a","status":200}`},
		{APIKeyID: key.ID, Message: "poison", Context: `{"path":"/poison","status":200}`},
		{APIKeyID: key.ID, Message: "GET /b", Context: `{"path":"/b","status":200}`},
	}
	if err := events.CreateIngested(db, seed); err != nil {
		t.Fatalf("create ingested: %v", err)
	}
	// The last step of a move deletes the staging row. Failing there means
	// the event and its endpoint stat were already written, so only a
	// savepoint rollback can undo them.
	if err := db.Exec(`CREATE TRIGGER poison BEFORE DELETE ON ingested_events
		WHEN OLD.message = 'poison' BEGIN SELECT RAISE(ABORT, 'poison'); END`).Error; err != nil {
		t.Fatalf("create trigger: %v", err)
	}

	err = NewEventProcessor().ProcessBatch(newJobCtx(db))

	if err != nil {
		t.Fatalf("ProcessBatch: %v", err)
	}
	var messages []string
	db.Model(&events.Event{}).Order("id").Pluck("message", &messages)
	if len(messages) != 2 || messages[0] != "GET /a" || messages[1] != "GET /b" {
		t.Errorf("events = %v, want [GET /a GET /b]", messages)
	}
	var waiting []string
	db.Model(&events.IngestedEvent{}).Pluck("message", &waiting)
	if len(waiting) != 1 || waiting[0] != "poison" {
		t.Errorf("waiting = %v, want [poison]", waiting)
	}
	var poisonStats int64
	db.Model(&events.EndpointStat{}).Where("path = ?", "/poison").Count(&poisonStats)
	if poisonStats != 0 {
		t.Errorf("endpoint stats for /poison = %d, want 0 (rolled back)", poisonStats)
	}
}

// During a deploy the old and the new container both run a processor. Both
// can read the same staging rows before either moves them.
func TestEventProcessor_TwoProcessorsMoveABatchOnce(t *testing.T) {
	db := setupProcessorDB(t)
	key, _, err := apikeys.Create(db, "Test App")
	if err != nil {
		t.Fatalf("create api key: %v", err)
	}
	if err := events.CreateIngested(db, []*events.IngestedEvent{
		{APIKeyID: key.ID, Message: "GET /a", Context: `{"path":"/a","status":200}`},
		{APIKeyID: key.ID, Message: "GET /b", Context: `{"path":"/b","status":200}`},
	}); err != nil {
		t.Fatalf("create ingested: %v", err)
	}
	batch, err := events.NextIngested(db, 0, processBatchSize) // both processors read this
	if err != nil {
		t.Fatalf("read batch: %v", err)
	}

	first, err := moveEvents(newJobCtx(db), batch)
	if err != nil {
		t.Fatalf("first processor: %v", err)
	}
	second, err := moveEvents(newJobCtx(db), batch)
	if err != nil {
		t.Fatalf("second processor: %v", err)
	}

	if len(first) != 2 || len(second) != 0 {
		t.Errorf("moved %d then %d, want 2 then 0", len(first), len(second))
	}
	var stored int64
	db.Model(&events.Event{}).Count(&stored)
	if stored != 2 {
		t.Errorf("events = %d, want 2 (no copies)", stored)
	}
	var total int64
	db.Model(&events.EndpointStat{}).Select("COALESCE(SUM(total), 0)").Scan(&total)
	if total != 2 {
		t.Errorf("endpoint stat total = %d, want 2", total)
	}
}
