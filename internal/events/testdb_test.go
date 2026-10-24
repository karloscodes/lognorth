package events

import (
	"path/filepath"
	"testing"

	"github.com/karloscodes/cartridge/sqlite"
	"gorm.io/gorm"
)

// newTestDB returns a fresh database with every table of this package.
// Package tests cannot import internal/database (it imports this package),
// so they migrate the same models here.
func newTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	manager := sqlite.NewManager(sqlite.Config{Path: filepath.Join(t.TempDir(), "test.db"), EnableWAL: true})
	db, err := manager.Connect()
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	if err := db.AutoMigrate(&Event{}, &IngestedEvent{}, &IssueSeen{}, &EndpointStat{}, &SpikeState{}, &Release{}, &SecretSeen{}, &AppHour{}, &Issue{}); err != nil {
		t.Fatalf("Failed to migrate: %v", err)
	}
	if err := AutoMigrateSPC(db); err != nil {
		t.Fatalf("Failed to migrate SPC tables: %v", err)
	}
	return db
}
