package database

import (
	"fmt"
	"log/slog"

	"github.com/karloscodes/cartridge/sqlite"
	"gorm.io/gorm"

	"lognorth/internal/alerts"
	"lognorth/internal/apikeys"
	"lognorth/internal/config"
	"lognorth/internal/events"
	"lognorth/internal/settings"
	"lognorth/internal/status"
	"lognorth/internal/uptime"
	"lognorth/internal/users"
)

// DBManager wraps cartridge's sqlite.Manager with lognorth-specific migration methods.
type DBManager struct {
	*sqlite.Manager
	logger *slog.Logger
}

// NewDBManager creates a new database manager using cartridge's sqlite.Manager.
func NewDBManager(cfg *config.Config, logger *slog.Logger) *DBManager {
	sqliteCfg := sqlite.Config{
		Path:         cfg.DatabaseDSN(),
		MaxOpenConns: cfg.GetMaxOpenConns(),
		MaxIdleConns: cfg.GetMaxIdleConns(),
		Logger:       logger,
		EnableWAL:    true,
		TxImmediate:  true,
		BusyTimeout:  5000,
		// Search and stats scan the events table. Reading it through mmap
		// made them 23-38% faster on 500k events; a larger cache_size made
		// them slower. An I/O error on a mapped page stops the process
		// instead of failing the query.
		Pragmas: []string{"PRAGMA mmap_size = 268435456"},
		// One write connection and a pool of read-only ones. Writes then
		// cannot fail with "database is locked" against each other, and a
		// search does not wait for the ingestion of events.
		ReadPool: true,
	}

	return &DBManager{
		Manager: sqlite.NewManager(sqliteCfg),
		logger:  logger,
	}
}

// Init initializes the database connection.
func (dm *DBManager) Init() error {
	_, err := dm.Manager.Connect()
	return err
}

// MigrateDatabase runs lognorth-specific migrations.
func (dm *DBManager) MigrateDatabase() error {
	db := dm.GetConnection()
	if db == nil {
		return gorm.ErrInvalidDB
	}

	// Run migrations in a transaction
	err := db.Transaction(Migrate)
	if err != nil {
		dm.logger.Error("Failed to auto-migrate database", slog.Any("error", err))
		return err
	}

	// Read connections that were open during the migration would plan
	// their next query with the old schema once.
	dm.SchemaChanged()

	// PASSIVE never waits. A FULL checkpoint waits for every reader, and the
	// live replica keeps a read open on purpose, so FULL always ran into
	// busy_timeout (5s) and gave up.
	if err := dm.CheckpointWAL("PASSIVE"); err != nil {
		dm.logger.Warn("Failed to checkpoint WAL after migration", slog.Any("error", err))
	}

	dm.logger.Info("Database migration completed successfully")
	return nil
}

// Migrate brings the database to the current version: AutoMigrate for the
// schema, then the data migrations that have not run yet. The app and the
// tests both use it, so there is one list of tables.
func Migrate(db *gorm.DB) error {
	if err := db.AutoMigrate(
		&apikeys.APIKey{},
		&events.Event{},
		&events.IngestedEvent{},
		&events.IssueSeen{},
		&events.EndpointStat{},
		&alerts.SpikeState{},
		&events.Release{},
		&alerts.SecretSeen{},
		&events.AppHour{},
		&events.Issue{},
		&settings.Settings{},
		&users.User{},
		&uptime.Check{},
		&uptime.Day{},
		&status.Page{},
		&status.Comment{},
	); err != nil {
		return err
	}
	if err := alerts.AutoMigrateSPC(db); err != nil {
		return fmt.Errorf("failed to migrate SPC tables: %w", err)
	}
	return runDataMigrations(db)
}
