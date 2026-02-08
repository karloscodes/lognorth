package events

import (
	"os"
	"testing"
	"time"

	"github.com/karloscodes/cartridge/sqlite"
	"gorm.io/gorm"
)

func setupIngestedTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	dbPath := "test_ingested_" + t.Name() + ".db"
	t.Cleanup(func() {
		os.Remove(dbPath)
		os.Remove(dbPath + "-wal")
		os.Remove(dbPath + "-shm")
	})

	manager := sqlite.NewManager(sqlite.Config{Path: dbPath, EnableWAL: true})
	db, err := manager.Connect()
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}

	if err := AutoMigrateIngested(db); err != nil {
		t.Fatalf("Failed to migrate: %v", err)
	}

	return db
}

func TestCreateIngested(t *testing.T) {
	db := setupIngestedTestDB(t)

	event := &IngestedEvent{
		APIKeyID: 1,
		Message:  "GET /api/users → 200",
		TraceID:  "req-123",
	}

	err := CreateIngested(db, event)

	if err != nil {
		t.Fatalf("CreateIngested failed: %v", err)
	}
	if event.ID == 0 {
		t.Error("expected ID to be set")
	}
	if event.Processed != 0 {
		t.Error("expected Processed to be 0")
	}
	if event.Timestamp.IsZero() {
		t.Error("expected Timestamp to be set")
	}
	if event.CreatedAt.IsZero() {
		t.Error("expected CreatedAt to be set")
	}
}

func TestCreateIngested_WithTimestamp(t *testing.T) {
	db := setupIngestedTestDB(t)
	customTime := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)

	event := &IngestedEvent{
		APIKeyID:  1,
		Message:   "Custom timestamp",
		Timestamp: customTime,
	}

	CreateIngested(db, event)

	if !event.Timestamp.Equal(customTime) {
		t.Error("should preserve custom timestamp")
	}
}

func TestGetUnprocessed(t *testing.T) {
	db := setupIngestedTestDB(t)

	// Create events
	CreateIngested(db, &IngestedEvent{APIKeyID: 1, Message: "event1"})
	CreateIngested(db, &IngestedEvent{APIKeyID: 1, Message: "event2"})
	CreateIngested(db, &IngestedEvent{APIKeyID: 1, Message: "event3"})

	events, err := GetUnprocessed(db, 10)

	if err != nil {
		t.Fatalf("GetUnprocessed failed: %v", err)
	}
	if len(events) != 3 {
		t.Errorf("expected 3 events, got %d", len(events))
	}
}

func TestGetUnprocessed_RespectsLimit(t *testing.T) {
	db := setupIngestedTestDB(t)

	for i := 0; i < 10; i++ {
		CreateIngested(db, &IngestedEvent{APIKeyID: 1, Message: "event"})
	}

	events, _ := GetUnprocessed(db, 5)

	if len(events) != 5 {
		t.Errorf("expected 5 events (limit), got %d", len(events))
	}
}

func TestGetUnprocessed_OrdersByCreatedAt(t *testing.T) {
	db := setupIngestedTestDB(t)

	CreateIngested(db, &IngestedEvent{APIKeyID: 1, Message: "first"})
	time.Sleep(10 * time.Millisecond)
	CreateIngested(db, &IngestedEvent{APIKeyID: 1, Message: "second"})

	events, _ := GetUnprocessed(db, 10)

	if events[0].Message != "first" {
		t.Error("expected oldest event first (FIFO)")
	}
}

func TestMarkProcessed(t *testing.T) {
	db := setupIngestedTestDB(t)

	CreateIngested(db, &IngestedEvent{APIKeyID: 1, Message: "event1"})
	CreateIngested(db, &IngestedEvent{APIKeyID: 1, Message: "event2"})

	events, _ := GetUnprocessed(db, 10)
	ids := []uint{events[0].ID}

	err := MarkProcessed(db, ids)

	if err != nil {
		t.Fatalf("MarkProcessed failed: %v", err)
	}

	remaining, _ := GetUnprocessed(db, 10)
	if len(remaining) != 1 {
		t.Errorf("expected 1 unprocessed event, got %d", len(remaining))
	}
}

func TestMarkProcessed_EmptySlice(t *testing.T) {
	db := setupIngestedTestDB(t)

	err := MarkProcessed(db, []uint{})

	if err != nil {
		t.Error("empty slice should not error")
	}
}

func TestCountUnprocessed(t *testing.T) {
	db := setupIngestedTestDB(t)

	count, _ := CountUnprocessed(db)
	if count != 0 {
		t.Errorf("expected 0, got %d", count)
	}

	CreateIngested(db, &IngestedEvent{APIKeyID: 1, Message: "event1"})
	CreateIngested(db, &IngestedEvent{APIKeyID: 1, Message: "event2"})

	count, _ = CountUnprocessed(db)
	if count != 2 {
		t.Errorf("expected 2, got %d", count)
	}
}

func TestDeleteProcessed(t *testing.T) {
	db := setupIngestedTestDB(t)

	// Create and mark as processed
	CreateIngested(db, &IngestedEvent{APIKeyID: 1, Message: "old"})
	events, _ := GetUnprocessed(db, 10)
	MarkProcessed(db, []uint{events[0].ID})

	// Manually set created_at to old time
	db.Model(&IngestedEvent{}).Where("id = ?", events[0].ID).
		Update("created_at", time.Now().Add(-48*time.Hour))

	// Create unprocessed event
	CreateIngested(db, &IngestedEvent{APIKeyID: 1, Message: "new"})

	deleted, err := DeleteProcessed(db, 24*time.Hour)

	if err != nil {
		t.Fatalf("DeleteProcessed failed: %v", err)
	}
	if deleted != 1 {
		t.Errorf("expected 1 deleted, got %d", deleted)
	}

	// Unprocessed event should remain
	remaining, _ := CountUnprocessed(db)
	if remaining != 1 {
		t.Errorf("expected 1 unprocessed remaining, got %d", remaining)
	}
}

func TestDeleteProcessed_DoesNotDeleteRecent(t *testing.T) {
	db := setupIngestedTestDB(t)

	// Create and mark as processed (recent)
	CreateIngested(db, &IngestedEvent{APIKeyID: 1, Message: "recent"})
	events, _ := GetUnprocessed(db, 10)
	MarkProcessed(db, []uint{events[0].ID})

	deleted, _ := DeleteProcessed(db, 24*time.Hour)

	if deleted != 0 {
		t.Errorf("expected 0 deleted (too recent), got %d", deleted)
	}
}
