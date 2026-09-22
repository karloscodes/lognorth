package events

import (
	"time"

	"gorm.io/gorm"
)

// BackfillErrorFields computes the error columns and hashes for error events
// stored before those columns existed.
func BackfillErrorFields(db *gorm.DB) error {
	var lastID uint
	for {
		var batch []Event
		err := db.Where("is_error = 1 AND (error_hash_1 IS NULL OR error_hash_1 = '') AND id > ?", lastID).
			Order("id").Limit(500).Find(&batch).Error
		if err != nil || len(batch) == 0 {
			return err
		}
		for _, e := range batch {
			lastID = e.ID
			ctx, err := e.GetContext()
			if err != nil {
				continue
			}
			PopulateErrorFields(&e, ctx)
			if e.IssueHash == "" {
				continue // No class and no message: not an issue
			}
			err = db.Model(&Event{}).Where("id = ?", e.ID).Updates(map[string]any{
				"error_class":   e.ErrorClass,
				"error_message": e.ErrorMessage,
				"error_file":    e.ErrorFile,
				"error_line":    e.ErrorLine,
				"error_caller":  e.ErrorCaller,
				"error_hash_1":  e.ErrorHash1,
				"error_hash_2":  e.ErrorHash2,
				"error_hash_3":  e.ErrorHash3,
				"issue_hash":    e.IssueHash,
			}).Error
			if err != nil {
				return err
			}
		}
	}
}

// BackfillIssueHashes sets issue_hash on error events stored before it existed,
// and seeds alert state for every existing issue: alerts now key on
// issue_hash, and without a row each old issue would send a "new error" alert
// on its next occurrence.
func BackfillIssueHashes(db *gorm.DB) error {
	err := db.Exec(`
		UPDATE events
		SET issue_hash = COALESCE(NULLIF(error_hash_3, ''), NULLIF(error_hash_2, ''), error_hash_1)
		WHERE is_error = 1 AND (issue_hash IS NULL OR issue_hash = '') AND error_hash_1 != ''
	`).Error
	if err != nil {
		return err
	}

	return db.Exec(`
		INSERT INTO issues_seen (error_hash, seen_at, last_occurrence_at, muted)
		SELECT issue_hash, ?, MAX(timestamp), false FROM events
		WHERE is_error = 1 AND issue_hash != ''
		GROUP BY issue_hash
		ON CONFLICT(error_hash) DO NOTHING
	`, time.Time{}).Error
}

// NormalizeToUTC rewrites times stored with a non-UTC offset. SQLite compares
// times as text, so a row stored as 10:00+02:00 sorts after 09:00+00:00 even
// though it is earlier.
func NormalizeToUTC(db *gorm.DB, table, column string) error {
	type row struct {
		ID uint
		T  time.Time
	}
	for {
		var rows []row
		err := db.Table(table).Select("id, "+column+" as t").
			Where(column+" NOT LIKE ?", "%+00:00").Limit(500).Scan(&rows).Error
		if err != nil || len(rows) == 0 {
			return err
		}
		for _, r := range rows {
			if err := db.Table(table).Where("id = ?", r.ID).Update(column, r.T.UTC()).Error; err != nil {
				return err
			}
		}
	}
}
