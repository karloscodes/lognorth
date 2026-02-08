package database

import (
	"fmt"
	"log/slog"

	"github.com/karloscodes/cartridge/sqlite"
	"gorm.io/gorm"

	"lognorth/internal/apikeys"
	"lognorth/internal/config"
	"lognorth/internal/events"
	"lognorth/internal/onboarding"
	"lognorth/internal/settings"
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
	err := db.Transaction(func(tx *gorm.DB) error {
		if err := tx.AutoMigrate(
			&apikeys.APIKey{},
			&events.Event{},
			&events.IngestedEvent{},
			&events.IssueSeen{},
			&events.EndpointStat{},
			&events.SpikeState{},
			&settings.Settings{},
			&users.User{},
			&onboarding.OnboardingSession{},
		); err != nil {
			return err
		}

		// SPC alerting tables
		if err := events.AutoMigrateSPC(tx); err != nil {
			return fmt.Errorf("failed to migrate SPC tables: %w", err)
		}

		return nil
	})
	if err != nil {
		dm.logger.Error("Failed to auto-migrate database", slog.Any("error", err))
		return err
	}

	if err := dm.CheckpointWAL("FULL"); err != nil {
		dm.logger.Warn("Failed to checkpoint WAL after migration", slog.Any("error", err))
	}

	dm.logger.Info("Database migration completed successfully")
	return nil
}
