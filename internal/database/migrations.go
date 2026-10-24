package database

import (
	"fmt"
	"time"

	"gorm.io/gorm"

	"lognorth/internal/events"
)

// A data migration changes rows, not schema: a backfill, a delete, a format
// change. Each runs once, in order, and is recorded in schema_migrations.
// Schema changes (tables, columns, indexes) stay with AutoMigrate, which is
// safe to run on every boot.
//
// Append new migrations at the end. Never edit or remove one that shipped:
// installs that already ran it will not run it again.
var dataMigrations = []struct {
	id string
	up func(*gorm.DB) error
}{
	{"2026-09-22-ingested-drop-processed-state", events.DropProcessedState},
	{"2026-09-22-events-error-fields", events.BackfillErrorFields},
	{"2026-09-22-events-issue-hash", events.BackfillIssueHashes},
	{"2026-09-22-events-timestamps-utc", func(db *gorm.DB) error { return events.NormalizeToUTC(db, "events", "timestamp") }},
	{"2026-09-22-endpoint-stats-buckets-utc", func(db *gorm.DB) error { return events.NormalizeToUTC(db, "endpoint_stats", "bucket") }},
	{"2026-09-22-drop-onboarding-sessions", func(db *gorm.DB) error { return db.Exec("DROP TABLE IF EXISTS onboarding_sessions").Error }},
	{"2026-10-24-events-drop-is-error-index", events.DropIsErrorIndex},
}

// schemaMigration records one data migration that ran.
type schemaMigration struct {
	ID        string `gorm:"primaryKey;size:100"`
	AppliedAt time.Time
}

func (schemaMigration) TableName() string {
	return "schema_migrations"
}

// runDataMigrations runs every data migration not recorded yet.
func runDataMigrations(db *gorm.DB) error {
	if err := db.AutoMigrate(&schemaMigration{}); err != nil {
		return err
	}

	var applied []string
	if err := db.Model(&schemaMigration{}).Pluck("id", &applied).Error; err != nil {
		return err
	}
	done := make(map[string]bool, len(applied))
	for _, id := range applied {
		done[id] = true
	}

	for _, m := range dataMigrations {
		if done[m.id] {
			continue
		}
		if err := m.up(db); err != nil {
			return fmt.Errorf("data migration %s: %w", m.id, err)
		}
		if err := db.Create(&schemaMigration{ID: m.id, AppliedAt: time.Now().UTC()}).Error; err != nil {
			return err
		}
	}
	return nil
}
