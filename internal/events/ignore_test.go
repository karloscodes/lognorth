package events

import "testing"

func TestShouldIgnorePath(t *testing.T) {
	t.Run("ignores dotfile paths", func(t *testing.T) {
		paths := []string{
			"/.env", "/.env.backup", "/.env.local", "/.env.staging",
			"/.git/config", "/.git/HEAD",
			"/.aws/config", "/.aws/credentials",
			"/.svn/entries",
			"/.docker/config.json",
			"/.vscode/settings.json",
		}

		for _, p := range paths {
			if !ShouldIgnorePath(p) {
				t.Errorf("expected %q to be ignored", p)
			}
		}
	})

	t.Run("is case insensitive", func(t *testing.T) {
		if !ShouldIgnorePath("/.ENV") {
			t.Error("expected /.ENV to be ignored")
		}
		if !ShouldIgnorePath("/.Git/Config") {
			t.Error("expected /.Git/Config to be ignored")
		}
	})

	t.Run("allows real app routes", func(t *testing.T) {
		paths := []string{
			"/", "/login", "/settings", "/broadcasts",
			"/api/v1/events", "/users/123", "/up",
			"/sitemap.xml", "/feed.xml",
			"/debug", "/server-status",
			"/wp-admin",
			"/info.php",
		}

		for _, p := range paths {
			if ShouldIgnorePath(p) {
				t.Errorf("expected %q to NOT be ignored", p)
			}
		}
	})

	t.Run("empty path is not ignored", func(t *testing.T) {
		if ShouldIgnorePath("") {
			t.Error("expected empty path to not be ignored")
		}
	})
}
