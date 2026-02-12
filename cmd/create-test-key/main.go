// create-test-key creates a test API key and prints it
// ONLY FOR USE IN TESTS - WILL NOT RUN IN PRODUCTION
package main

import (
	"fmt"
	"log"
	"log/slog"
	"os"

	"lognorth/internal/apikeys"
	"lognorth/internal/config"
	"lognorth/internal/database"
)

func main() {
	cfg := config.GetConfig()

	// SECURITY: Prevent running in production
	if cfg.IsProduction() {
		log.Fatalf("ERROR: create-test-key cannot be run in production environment")
	}

	// SECURITY: Only allow in test/development environments
	env := os.Getenv("LOGNORTH_ENV")
	if env != "test" && env != "development" {
		log.Fatalf("ERROR: create-test-key requires LOGNORTH_ENV=test or LOGNORTH_ENV=development (got: %s)", env)
	}

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	dbManager := database.NewDBManager(cfg, logger)
	if err := dbManager.Init(); err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	if err := dbManager.MigrateDatabase(); err != nil {
		log.Fatalf("Failed to run migrations: %v", err)
	}
	db := dbManager.GetConnection()

	_, fullKey, err := apikeys.Create(db, "SDK Test")
	if err != nil {
		log.Fatalf("Failed to create API key: %v", err)
	}

	fmt.Print(fullKey)
}
