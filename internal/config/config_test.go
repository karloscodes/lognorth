package config

import (
	"os"
	"testing"
)

func TestGetBaseURL(t *testing.T) {
	t.Cleanup(func() {
		Reset()
	})

	t.Run("with domain set", func(t *testing.T) {
		Reset()
		os.Setenv("LOGNORTH_DOMAIN", "logs.example.com")
		defer os.Unsetenv("LOGNORTH_DOMAIN")

		c := GetConfig()
		url := c.GetBaseURL()

		if url != "https://logs.example.com" {
			t.Errorf("expected 'https://logs.example.com', got %s", url)
		}
	})

	t.Run("with BASE_URL from Chasen and no domain", func(t *testing.T) {
		Reset()
		os.Unsetenv("LOGNORTH_DOMAIN")
		t.Setenv("BASE_URL", "https://logs.example.com")

		c := GetConfig()

		if url := c.GetBaseURL(); url != "https://logs.example.com" {
			t.Errorf("expected 'https://logs.example.com', got %s", url)
		}
	})

	t.Run("development without domain", func(t *testing.T) {
		Reset()
		os.Unsetenv("LOGNORTH_DOMAIN")
		os.Setenv("LOGNORTH_ENV", "development")
		defer os.Unsetenv("LOGNORTH_ENV")

		c := GetConfig()
		url := c.GetBaseURL()

		if url != "http://localhost:3000" {
			t.Errorf("expected 'http://localhost:3000', got %s", url)
		}
	})

	t.Run("production without domain", func(t *testing.T) {
		Reset()
		os.Unsetenv("LOGNORTH_DOMAIN")
		os.Setenv("LOGNORTH_ENV", "production")
		os.Setenv("LOGNORTH_PRIVATE_KEY", "test-key-for-production")
		defer os.Unsetenv("LOGNORTH_ENV")
		defer os.Unsetenv("LOGNORTH_PRIVATE_KEY")

		c := GetConfig()
		url := c.GetBaseURL()

		if url != "" {
			t.Errorf("expected empty string, got %s", url)
		}
	})
}

func TestIsDevelopment(t *testing.T) {
	Reset()
	os.Setenv("LOGNORTH_ENV", "development")
	defer os.Unsetenv("LOGNORTH_ENV")

	c := GetConfig()

	if !c.IsDevelopment() {
		t.Error("expected IsDevelopment to be true")
	}
	if c.IsProduction() {
		t.Error("expected IsProduction to be false")
	}
}

func TestDefaultValues(t *testing.T) {
	Reset()
	os.Unsetenv("LOGNORTH_ENV")
	os.Unsetenv("LOGNORTH_PORT")

	c := GetConfig()

	if c.Environment != Development {
		t.Errorf("expected default environment 'development', got %s", c.Environment)
	}
	if c.AppPort != "8080" {
		t.Errorf("expected default port '8080', got %s", c.AppPort)
	}
}

func TestDatabaseDSN(t *testing.T) {
	Reset()
	os.Setenv("LOGNORTH_STORAGE_PATH", "/data")
	os.Setenv("LOGNORTH_APP_NAME", "testapp")
	os.Setenv("LOGNORTH_ENV", "development")
	defer os.Unsetenv("LOGNORTH_STORAGE_PATH")
	defer os.Unsetenv("LOGNORTH_APP_NAME")
	defer os.Unsetenv("LOGNORTH_ENV")

	c := GetConfig()
	dsn := c.DatabaseDSN()

	expected := "/data/testapp-development.db"
	if dsn != expected {
		t.Errorf("expected DSN %s, got %s", expected, dsn)
	}
}

func TestPrivateKey(t *testing.T) {
	t.Cleanup(Reset)

	t.Run("production reads PRIVATE_KEY, the name Chasen gives it", func(t *testing.T) {
		Reset()
		os.Unsetenv("LOGNORTH_PRIVATE_KEY")
		t.Setenv("LOGNORTH_ENV", "production")
		t.Setenv("PRIVATE_KEY", "key-from-chasen")

		c := GetConfig()

		if c.PrivateKey != "key-from-chasen" {
			t.Errorf("expected the key from PRIVATE_KEY, got %q", c.PrivateKey)
		}
	})

	t.Run("LOGNORTH_PRIVATE_KEY wins over PRIVATE_KEY", func(t *testing.T) {
		Reset()
		t.Setenv("LOGNORTH_ENV", "production")
		t.Setenv("LOGNORTH_PRIVATE_KEY", "own-key")
		t.Setenv("PRIVATE_KEY", "key-from-chasen")

		c := GetConfig()

		if c.PrivateKey != "own-key" {
			t.Errorf("expected LOGNORTH_PRIVATE_KEY, got %q", c.PrivateKey)
		}
	})
}

func TestLogDirectory(t *testing.T) {
	t.Cleanup(Reset)

	t.Run("defaults to logs in the working directory", func(t *testing.T) {
		Reset()
		os.Unsetenv("LOGNORTH_LOGS_DIR")

		if dir := GetConfig().GetLogDirectory(); dir != "logs" {
			t.Errorf("expected logs, got %s", dir)
		}
	})

	t.Run("reads LOGNORTH_LOGS_DIR", func(t *testing.T) {
		Reset()
		t.Setenv("LOGNORTH_LOGS_DIR", "/app/logs")

		if dir := GetConfig().GetLogDirectory(); dir != "/app/logs" {
			t.Errorf("expected /app/logs, got %s", dir)
		}
	})
}
