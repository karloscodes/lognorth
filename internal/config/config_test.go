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
