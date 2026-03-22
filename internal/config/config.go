// Package config provides configuration management for LogNorth
package config

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
)

// Environment types
const (
	Development = "development"
	Production  = "production"
	Test        = "test"
)

// Config holds all configuration parameters for the application
type Config struct {
	AppName        string
	AppPort        string
	Environment    string
	PrivateKey     string
	StoragePath    string
	Domain         string
	ManagerVersion string
}

var (
	cfg  *Config
	once sync.Once
)

// GetConfig returns the application configuration
func GetConfig() *Config {
	once.Do(func() {
		env := getEnv("LOGNORTH_ENV", Development)
		storagePath := getEnv("LOGNORTH_STORAGE_PATH", "storage")
		privateKey := getPrivateKey(env, storagePath)

		cfg = &Config{
			AppName:        getEnv("LOGNORTH_APP_NAME", "lognorth"),
			AppPort:        getEnv("LOGNORTH_PORT", "8080"),
			Environment:    env,
			PrivateKey:     privateKey,
			StoragePath:    storagePath,
			Domain:         os.Getenv("LOGNORTH_DOMAIN"),
			ManagerVersion: os.Getenv("MATCHA_MANAGER_VERSION"),
		}
	})
	return cfg
}

// getPrivateKey returns the private key from env, file, or generates a new one
func getPrivateKey(env string, storagePath string) string {
	key := os.Getenv("LOGNORTH_PRIVATE_KEY")

	// Production requires explicit private key (unless demo mode)
	if env == Production && os.Getenv("LOGNORTH_DEMO") != "true" {
		if key == "" {
			log.Fatal("Production requires LOGNORTH_PRIVATE_KEY to be set")
		}
		return key
	}

	// Development/test: use env key if set
	if key != "" {
		return key
	}

	// Try to read from file (persists across restarts)
	keyFile := filepath.Join(storagePath, ".private_key")
	if data, err := os.ReadFile(keyFile); err == nil && len(data) > 0 {
		return string(data)
	}

	// Generate new key and persist it
	bytes := make([]byte, 32)
	rand.Read(bytes)
	key = hex.EncodeToString(bytes)

	// Ensure storage directory exists
	os.MkdirAll(storagePath, 0755)

	// Save key to file (ignore errors - worst case we regenerate next time)
	os.WriteFile(keyFile, []byte(key), 0600)

	return key
}

// getEnv gets an env var or returns a default
func getEnv(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}

// IsDevelopment returns true if the environment is development
func (c *Config) IsDevelopment() bool {
	return c.Environment == Development
}

// IsProduction returns true if the environment is production
func (c *Config) IsProduction() bool {
	return c.Environment == Production
}

// IsTest returns true if the environment is test
func (c *Config) IsTest() bool {
	return c.Environment == Test
}

// GetPort returns the HTTP server port (implements cartridge.Config interface)
func (c *Config) GetPort() string {
	return c.AppPort
}

// GetPublicDirectory returns the path to public/static assets
func (c *Config) GetPublicDirectory() string {
	return "web/dist/assets"
}

// GetAssetsPrefix returns the URL prefix for static assets
func (c *Config) GetAssetsPrefix() string {
	return "/assets"
}

// GetAppName returns the application name
func (c *Config) GetAppName() string {
	return c.AppName
}

// DatabaseDSN returns the database connection string
func (c *Config) DatabaseDSN() string {
	return filepath.Join(c.StoragePath, fmt.Sprintf("%s-%s.db", c.AppName, c.Environment))
}

// GetSessionSecret returns the session encryption key
func (c *Config) GetSessionSecret() string {
	return c.PrivateKey
}

// GetSessionTimeout returns the session timeout in seconds
func (c *Config) GetSessionTimeout() int {
	return 1800 // 30 minutes
}

// GetLoginSessionTimeout returns the login session timeout in seconds
func (c *Config) GetLoginSessionTimeout() int {
	return 604800 // 1 week
}

// GetMaxOpenConns returns the max open connections
func (c *Config) GetMaxOpenConns() int {
	if c.Environment == Test {
		return 1
	}
	return 10
}

// GetMaxIdleConns returns the max idle connections
func (c *Config) GetMaxIdleConns() int {
	if c.Environment == Test {
		return 1
	}
	return 5
}

// GetLogLevel returns the log level
func (c *Config) GetLogLevel() string {
	switch c.Environment {
	case Production:
		return "warn"
	case Test:
		return "error"
	default:
		return "debug"
	}
}

// GetLogDirectory returns the logs directory
func (c *Config) GetLogDirectory() string {
	return "logs"
}

// GetLogMaxSizeMB returns the max log file size in MB
func (c *Config) GetLogMaxSizeMB() int {
	return 20
}

// GetLogMaxBackups returns the max number of log backups
func (c *Config) GetLogMaxBackups() int {
	return 10
}

// GetLogMaxAgeDays returns the max age in days for log files
func (c *Config) GetLogMaxAgeDays() int {
	return 30
}


// GetBaseURL returns the base URL for links in notifications
func (c *Config) GetBaseURL() string {
	if c.Domain != "" {
		return "https://" + c.Domain
	}
	if c.IsDevelopment() {
		return "http://localhost:3000"
	}
	return ""
}

// IsDemo returns true if the application is running in demo mode
func (c *Config) IsDemo() bool {
	return os.Getenv("LOGNORTH_DEMO") == "true"
}

// CLINeedsUpdate returns true when the CLI binary version is unknown
func (c *Config) CLINeedsUpdate() bool {
	return c.ManagerVersion == "" || c.ManagerVersion == "dev"
}

// Reset clears the cached configuration; intended for tests
func Reset() {
	once = sync.Once{}
	cfg = nil
}
