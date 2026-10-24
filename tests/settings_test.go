package tests

import (
	"testing"

	"lognorth/internal/alerts"
	"lognorth/internal/settings"
)

// =============================================================================
// SETTINGS NOTIFICATION TESTS
// Test the notification channel configuration logic
// =============================================================================

func TestSettings_CanNotify(t *testing.T) {
	t.Run("empty settings cannot notify", func(t *testing.T) {
		s := &settings.Settings{}

		if alerts.CanNotify(s) {
			t.Error("empty settings should not be able to notify")
		}
	})

	t.Run("ntfy channel with URL can notify", func(t *testing.T) {
		s := &settings.Settings{
			NotifyChannel: "ntfy",
			NtfyURL:       "https://ntfy.sh/my-topic",
		}

		if !alerts.CanNotify(s) {
			t.Error("ntfy channel with URL should be able to notify")
		}
	})

	t.Run("ntfy channel without URL cannot notify", func(t *testing.T) {
		s := &settings.Settings{
			NotifyChannel: "ntfy",
			NtfyURL:       "",
		}

		if alerts.CanNotify(s) {
			t.Error("ntfy channel without URL should not be able to notify")
		}
	})

	t.Run("email channel fully configured can notify", func(t *testing.T) {
		s := &settings.Settings{
			NotifyChannel: "email",
			SMTPHost:      "smtp.example.com",
			SMTPPort:      587,
			SMTPFrom:      "alerts@example.com",
			NotifyEmail:   "user@example.com",
		}

		if !alerts.CanNotify(s) {
			t.Error("email channel fully configured should be able to notify")
		}
	})

	t.Run("email channel missing host cannot notify", func(t *testing.T) {
		s := &settings.Settings{
			NotifyChannel: "email",
			SMTPHost:      "",
			SMTPPort:      587,
			SMTPFrom:      "alerts@example.com",
			NotifyEmail:   "user@example.com",
		}

		if alerts.CanNotify(s) {
			t.Error("email channel without host should not be able to notify")
		}
	})

	t.Run("email channel missing notify email cannot notify", func(t *testing.T) {
		s := &settings.Settings{
			NotifyChannel: "email",
			SMTPHost:      "smtp.example.com",
			SMTPPort:      587,
			SMTPFrom:      "alerts@example.com",
			NotifyEmail:   "",
		}

		if alerts.CanNotify(s) {
			t.Error("email channel without notify email should not be able to notify")
		}
	})

	t.Run("unknown channel cannot notify", func(t *testing.T) {
		s := &settings.Settings{
			NotifyChannel: "telegram",
			NtfyURL:       "https://ntfy.sh/my-topic",
		}

		if alerts.CanNotify(s) {
			t.Error("unknown channel should not be able to notify")
		}
	})
}

func TestSettings_IsNtfyConfigured(t *testing.T) {
	t.Run("configured with URL", func(t *testing.T) {
		s := &settings.Settings{NtfyURL: "https://ntfy.sh/topic"}

		if !s.IsNtfyConfigured() {
			t.Error("should be configured when URL is set")
		}
	})

	t.Run("not configured without URL", func(t *testing.T) {
		s := &settings.Settings{NtfyURL: ""}

		if s.IsNtfyConfigured() {
			t.Error("should not be configured without URL")
		}
	})
}

func TestSettings_IsEmailConfigured(t *testing.T) {
	t.Run("fully configured", func(t *testing.T) {
		s := &settings.Settings{
			SMTPHost:    "smtp.example.com",
			SMTPPort:    587,
			SMTPFrom:    "alerts@example.com",
			NotifyEmail: "user@example.com",
		}

		if !s.IsEmailConfigured() {
			t.Error("should be configured with all fields")
		}
	})

	t.Run("missing host", func(t *testing.T) {
		s := &settings.Settings{
			SMTPHost:    "",
			SMTPPort:    587,
			SMTPFrom:    "alerts@example.com",
			NotifyEmail: "user@example.com",
		}

		if s.IsEmailConfigured() {
			t.Error("should not be configured without host")
		}
	})

	t.Run("missing port", func(t *testing.T) {
		s := &settings.Settings{
			SMTPHost:    "smtp.example.com",
			SMTPPort:    0,
			SMTPFrom:    "alerts@example.com",
			NotifyEmail: "user@example.com",
		}

		if s.IsEmailConfigured() {
			t.Error("should not be configured without port")
		}
	})

	t.Run("missing from address", func(t *testing.T) {
		s := &settings.Settings{
			SMTPHost:    "smtp.example.com",
			SMTPPort:    587,
			SMTPFrom:    "",
			NotifyEmail: "user@example.com",
		}

		if s.IsEmailConfigured() {
			t.Error("should not be configured without from address")
		}
	})

	t.Run("missing notify email", func(t *testing.T) {
		s := &settings.Settings{
			SMTPHost:    "smtp.example.com",
			SMTPPort:    587,
			SMTPFrom:    "alerts@example.com",
			NotifyEmail: "",
		}

		if s.IsEmailConfigured() {
			t.Error("should not be configured without notify email")
		}
	})
}

func TestSettings_HasPassword(t *testing.T) {
	t.Run("with password", func(t *testing.T) {
		s := &settings.Settings{SMTPPassword: "secret123"}

		if !s.HasPassword() {
			t.Error("should have password when set")
		}
	})

	t.Run("without password", func(t *testing.T) {
		s := &settings.Settings{SMTPPassword: ""}

		if s.HasPassword() {
			t.Error("should not have password when empty")
		}
	})
}
