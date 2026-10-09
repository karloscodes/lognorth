package events

import (
	"testing"
	"time"
)

func TestReleases(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)

	t.Run("keeps the first time a release was seen", func(t *testing.T) {
		db := newTestDB(t)
		NoteRelease(db, 1, "a1b2c3d", now.Add(-time.Hour))
		NoteRelease(db, 1, "a1b2c3d", now)                   // a restart
		NoteRelease(db, 1, "a1b2c3d", now.Add(-2*time.Hour)) // an event that arrived late

		releases := ReleasesBetween(db, 1, now.Add(-24*time.Hour), now)

		if len(releases) != 1 || !releases[0].FirstSeen.Equal(now.Add(-2*time.Hour)) {
			t.Fatalf("expected a1b2c3d first seen 2 hours ago, got %+v", releases)
		}
	})

	t.Run("lists the window's releases of one app, oldest first", func(t *testing.T) {
		db := newTestDB(t)
		NoteRelease(db, 1, "new", now.Add(-time.Hour))
		NoteRelease(db, 1, "old", now.Add(-3*time.Hour))
		NoteRelease(db, 1, "ancient", now.Add(-48*time.Hour))
		NoteRelease(db, 2, "other-app", now.Add(-time.Hour))

		releases := ReleasesBetween(db, 1, now.Add(-24*time.Hour), now)

		if len(releases) != 2 || releases[0].Version != "old" || releases[1].Version != "new" {
			t.Fatalf("expected old then new, got %+v", releases)
		}
	})

	t.Run("ignores an empty release", func(t *testing.T) {
		db := newTestDB(t)

		NoteRelease(db, 1, "", now)

		if releases := ReleasesBetween(db, 1, now.Add(-time.Hour), now); len(releases) != 0 {
			t.Fatalf("expected nothing, got %+v", releases)
		}
	})
}
