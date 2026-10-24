package events

import (
	"time"

	"gorm.io/gorm"
)

// AppHour counts an app's events in one UTC hour. The processor adds each
// batch in the transaction that stores its events, so the home page, the log
// histogram, and the digest read a few hundred rows instead of counting every
// event.
type AppHour struct {
	ID       uint      `gorm:"primaryKey"`
	APIKeyID uint      `gorm:"uniqueIndex:idx_app_hour;not null"`
	Hour     time.Time `gorm:"uniqueIndex:idx_app_hour;not null"`
	Total    int64     `gorm:"not null;default:0"`
	Errors   int64     `gorm:"not null;default:0"`
}

func (AppHour) TableName() string {
	return "app_hours"
}

type appHourKey struct {
	apiKeyID uint
	hour     time.Time
}

// HourBatch counts a batch of events per app and hour, and writes each hour
// once. Use it in the transaction that stores the events.
type HourBatch map[appHourKey]*AppHour

// Add counts one stored event.
func (b HourBatch) Add(apiKeyID uint, isError bool, at time.Time) {
	key := appHourKey{apiKeyID, at.UTC().Truncate(time.Hour)}
	h := b[key]
	if h == nil {
		h = &AppHour{APIKeyID: apiKeyID, Hour: key.hour}
		b[key] = h
	}
	h.Total++
	if isError {
		h.Errors++
	}
}

// Flush adds the batch to the stored hours: one write per hour, no read.
func (b HourBatch) Flush(tx *gorm.DB) error {
	for _, h := range b {
		err := tx.Exec(`INSERT INTO app_hours (api_key_id, hour, total, errors) VALUES (?, ?, ?, ?)
			ON CONFLICT(api_key_id, hour) DO UPDATE SET total = total + excluded.total, errors = errors + excluded.errors`,
			h.APIKeyID, h.Hour, h.Total, h.Errors).Error
		if err != nil {
			return err
		}
	}
	return nil
}

// RecountAppHours counts the hours from `from` on again from the events; a
// zero `from` counts them all. The data migration fills the table with it,
// the demo seed counts what it wrote, and the cleanup job recounts the last
// hours: during the upgrade deploy, the older LogNorth moves events without
// counting them. SQLite writes the hour in the text Go writes for a UTC time,
// so the processor's upserts find these rows.
func RecountAppHours(db *gorm.DB, from time.Time) error {
	from = from.UTC().Truncate(time.Hour)
	return db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("hour >= ?", from).Delete(&AppHour{}).Error; err != nil {
			return err
		}
		return tx.Exec(`INSERT INTO app_hours (api_key_id, hour, total, errors)
			SELECT api_key_id, strftime('%Y-%m-%d %H:00:00+00:00', timestamp) AS h, COUNT(*), SUM(is_error)
			FROM events WHERE timestamp >= ?
			GROUP BY api_key_id, h`, from).Error
	})
}

// appHoursIn returns an app's stored hours from `from` to before `to`. An
// apiKeyID of 0 means every app, one row per app and hour.
func appHoursIn(db *gorm.DB, apiKeyID uint, from, to time.Time) ([]AppHour, error) {
	query := db.Where("hour >= ? AND hour < ?", from.UTC(), to.UTC())
	if apiKeyID > 0 {
		query = query.Where("api_key_id = ?", apiKeyID)
	}
	var hours []AppHour
	err := query.Find(&hours).Error
	return hours, err
}
