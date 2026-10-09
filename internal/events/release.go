package events

import (
	"time"

	"gorm.io/gorm"
)

// Release is a version of an app and the first time LogNorth saw it, which
// is when it was deployed. The SDKs log "Release <version> started" at boot
// and stamp the release on errors; the processor keeps the earliest time.
type Release struct {
	ID        uint      `gorm:"primaryKey" json:"-"`
	APIKeyID  uint      `gorm:"uniqueIndex:idx_release_app_version,priority:1;index:idx_release_app_time,priority:1" json:"-"`
	Version   string    `gorm:"size:100;uniqueIndex:idx_release_app_version,priority:2" json:"version"`
	FirstSeen time.Time `gorm:"index:idx_release_app_time,priority:2" json:"first_seen"`
}

// NoteRelease records that an app ran a release at a time. It keeps the
// earliest time for each release, so a restart or a late event never moves
// a deploy.
func NoteRelease(db *gorm.DB, appID uint, version string, at time.Time) error {
	if version == "" {
		return nil
	}
	if r := []rune(version); len(r) > 100 {
		version = string(r[:100])
	}
	return db.Exec(`
		INSERT INTO releases (api_key_id, version, first_seen) VALUES (?, ?, ?)
		ON CONFLICT (api_key_id, version) DO UPDATE SET first_seen = MIN(first_seen, excluded.first_seen)
	`, appID, version, at.UTC()).Error
}

// ReleasesBetween returns the app's releases deployed in the window, oldest first.
func ReleasesBetween(db *gorm.DB, appID uint, from, to time.Time) []Release {
	releases := []Release{}
	db.Where("api_key_id = ? AND first_seen >= ? AND first_seen <= ?", appID, from.UTC(), to.UTC()).
		Order("first_seen").
		Find(&releases)
	return releases
}
