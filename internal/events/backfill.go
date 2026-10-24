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

// RehashIssues moves the error events whose message holds an id, a number,
// or an address to the issue of the message without it (normalizeMessage),
// and merges the issues they leave. Before, each id made its own issue.
// issues_seen merges too: the latest seen and last occurrence, and muted if
// any was, so a muted noisy error stays muted, and the upgrade sends no "new
// error" alert for an issue the owner knows. A second run changes nothing.
func RehashIssues(db *gorm.DB) error {
	var issues []Issue
	if err := db.Find(&issues).Error; err != nil {
		return err
	}

	moves := map[string][]string{} // the new issue hash, and the old ones it takes
	for _, issue := range issues {
		var event Event
		event.ErrorHash1, event.ErrorHash2, event.ErrorHash3 = computeErrorHashes(
			issue.ErrorClass, issue.ErrorMessage, issue.ErrorFile, issue.ErrorLine, issue.APIKeyID)
		hash := event.mostSpecificHash()
		if hash == "" || hash == issue.Hash {
			continue
		}
		err := db.Model(&Event{}).Where("issue_hash = ?", issue.Hash).Updates(map[string]any{
			"error_hash_1": event.ErrorHash1,
			"error_hash_2": event.ErrorHash2,
			"error_hash_3": event.ErrorHash3,
			"issue_hash":   hash,
		}).Error
		if err != nil {
			return err
		}
		moves[hash] = append(moves[hash], issue.Hash)
	}

	for hash, olds := range moves {
		if err := mergeSeen(db, hash, olds); err != nil {
			return err
		}
		for start := 0; start < len(olds); start += 500 {
			chunk := olds[start:min(start+500, len(olds))]
			if err := db.Where("hash IN ?", chunk).Delete(&Issue{}).Error; err != nil {
				return err
			}
		}
		if err := RecountIssues(db, []string{hash}); err != nil {
			return err
		}
	}
	return nil
}

// mergeSeen folds the issues_seen rows of the old hashes into the row of the
// new one.
func mergeSeen(db *gorm.DB, hash string, olds []string) error {
	merged := IssueSeen{ErrorHash: hash}
	found := false
	all := append([]string{hash}, olds...)
	for start := 0; start < len(all); start += 500 {
		var rows []IssueSeen
		if err := db.Where("error_hash IN ?", all[start:min(start+500, len(all))]).Find(&rows).Error; err != nil {
			return err
		}
		for _, row := range rows {
			found = true
			if row.SeenAt.After(merged.SeenAt) {
				merged.SeenAt = row.SeenAt
			}
			if row.LastOccurrenceAt.After(merged.LastOccurrenceAt) {
				merged.LastOccurrenceAt = row.LastOccurrenceAt
			}
			merged.Muted = merged.Muted || row.Muted
		}
		if err := db.Where("error_hash IN ?", all[start:min(start+500, len(all))]).Delete(&IssueSeen{}).Error; err != nil {
			return err
		}
	}
	if !found {
		return nil
	}
	return db.Create(&merged).Error
}
