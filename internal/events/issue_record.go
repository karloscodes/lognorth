package events

import (
	"encoding/json"
	"slices"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Issue is one error of an app, grouped by its issue hash: a row of the
// issue list. The processor keeps it as errors arrive, so the list reads one
// row per issue instead of every error event. Its count and first seen are
// all-time: retention deletes an issue only when its last event goes. The
// owner's state of an issue (seen, muted, the regression clock) stays in
// issues_seen: that state is not derived, and it outlives the events.
type Issue struct {
	ID           uint      `gorm:"primaryKey"`
	APIKeyID     uint      `gorm:"index:idx_issues_app_last;not null"`
	Hash         string    `gorm:"uniqueIndex;size:64;not null"`
	ErrorClass   string    `gorm:"size:255"`
	ErrorMessage string    `gorm:"size:1000"`
	ErrorFile    string    `gorm:"size:500"`
	ErrorLine    int       `gorm:"default:0"`
	FirstSeen    time.Time `gorm:"not null"`
	LastSeen     time.Time `gorm:"index:idx_issues_app_last;not null"`
	Count        int64     `gorm:"not null;default:0"`
	// Paths holds the first maxIssuePaths request paths of the issue, in the
	// order they first failed, as a JSON list.
	Paths string `gorm:"type:text;not null;default:'[]'"`
}

func (Issue) TableName() string {
	return "issues"
}

// maxIssuePaths is how many paths an issue keeps: the issue list shows 10.
const maxIssuePaths = 10

// pathList returns the issue's paths, never nil.
func (i *Issue) pathList() []string {
	paths := []string{}
	json.Unmarshal([]byte(i.Paths), &paths)
	return paths
}

// addPath keeps a path the issue failed on, while it has room.
func (i *Issue) addPath(path string) {
	path = strings.TrimSpace(path)
	if path == "" {
		return
	}
	paths := i.pathList()
	if len(paths) >= maxIssuePaths || slices.Contains(paths, path) {
		return
	}
	encoded, _ := json.Marshal(append(paths, path))
	i.Paths = string(encoded)
}

// IssueBatch counts a batch of moved error events into their issues, and
// writes each issue once. Use it in the transaction that stores the events.
type IssueBatch map[string]*Issue

// Add counts one stored event. The first time it sees an issue it reads the
// issue's row. With no row, it builds the row from the stored events, which
// already hold this event, and any event an older LogNorth moved without
// counting it during the upgrade deploy.
func (b IssueBatch) Add(tx *gorm.DB, event *Event, path string) error {
	if !event.IsError || event.IssueHash == "" {
		return nil
	}
	issue, ok := b[event.IssueHash]
	if !ok {
		row, err := findIssue(tx, event.IssueHash)
		if err != nil {
			return err
		}
		if row == nil {
			if err := RecountIssues(tx, []string{event.IssueHash}); err != nil {
				return err
			}
			row, err = findIssue(tx, event.IssueHash)
			if err != nil || row == nil {
				return err
			}
			b[event.IssueHash] = row
			return nil
		}
		b[event.IssueHash] = row
		issue = row
	}
	issue.Count++
	if event.Timestamp.Before(issue.FirstSeen) {
		issue.FirstSeen = event.Timestamp
	}
	if event.Timestamp.After(issue.LastSeen) {
		issue.LastSeen = event.Timestamp
	}
	issue.addPath(path)
	return nil
}

// Flush writes every issue the batch touched.
func (b IssueBatch) Flush(tx *gorm.DB) error {
	for _, issue := range b {
		if err := tx.Save(issue).Error; err != nil {
			return err
		}
	}
	return nil
}

func findIssue(db *gorm.DB, hash string) (*Issue, error) {
	var issue Issue
	if err := db.Where("hash = ?", hash).Limit(1).Find(&issue).Error; err != nil {
		return nil, err
	}
	if issue.ID == 0 {
		return nil, nil
	}
	return &issue, nil
}

// RecountIssues builds the issues of the given hashes again from their error
// events, and deletes those that have no event left. The data migration
// fills the table with it, and the processor builds a new issue with it.
func RecountIssues(db *gorm.DB, hashes []string) error {
	for start := 0; start < len(hashes); start += 500 {
		chunk := hashes[start:min(start+500, len(hashes))]
		// Only error events have an issue hash, and they share the class,
		// message, file, and line the hash is made of.
		var rows []struct {
			APIKeyID     uint
			Hash         string
			ErrorClass   string
			ErrorMessage string
			ErrorFile    string
			ErrorLine    int
			FirstSeen    string
			LastSeen     string
			Count        int64
		}
		err := db.Raw(`SELECT api_key_id, issue_hash AS hash, error_class, error_message, error_file, error_line,
				MIN(timestamp) AS first_seen, MAX(timestamp) AS last_seen, COUNT(*) AS count
			FROM events WHERE issue_hash IN ?
			GROUP BY issue_hash`, chunk).Scan(&rows).Error
		if err != nil {
			return err
		}

		found := map[string]bool{}
		for _, r := range rows {
			found[r.Hash] = true
			issue := Issue{
				APIKeyID: r.APIKeyID, Hash: r.Hash,
				ErrorClass: r.ErrorClass, ErrorMessage: r.ErrorMessage, ErrorFile: r.ErrorFile, ErrorLine: r.ErrorLine,
				FirstSeen: ParseSQLiteTimestamp(r.FirstSeen), LastSeen: ParseSQLiteTimestamp(r.LastSeen),
				Count: r.Count, Paths: "[]",
			}
			paths, err := firstIssuePaths(db, r.Hash)
			if err != nil {
				return err
			}
			for _, p := range paths {
				issue.addPath(p)
			}
			err = db.Clauses(clause.OnConflict{
				Columns:   []clause.Column{{Name: "hash"}},
				DoUpdates: clause.AssignmentColumns([]string{"api_key_id", "error_class", "error_message", "error_file", "error_line", "first_seen", "last_seen", "count", "paths"}),
			}).Create(&issue).Error
			if err != nil {
				return err
			}
		}

		var gone []string
		for _, hash := range chunk {
			if !found[hash] {
				gone = append(gone, hash)
			}
		}
		if len(gone) > 0 {
			if err := db.Where("hash IN ?", gone).Delete(&Issue{}).Error; err != nil {
				return err
			}
		}
	}
	return nil
}

// firstIssuePaths returns the first maxIssuePaths paths an issue failed on,
// in the order they first failed.
func firstIssuePaths(db *gorm.DB, hash string) ([]string, error) {
	var paths []string
	err := db.Raw(`SELECT path FROM (
			SELECT CAST(json_extract(context, '$.path') AS TEXT) AS path, MIN(id) AS first
			FROM events WHERE issue_hash = ? AND json_valid(context) GROUP BY path
		) WHERE path IS NOT NULL AND TRIM(path) != ''
		ORDER BY first LIMIT ?`, hash, maxIssuePaths).Scan(&paths).Error
	return paths, err
}

// RecountAllIssues builds the issues table from the stored error events.
func RecountAllIssues(db *gorm.DB) error {
	var hashes []string
	if err := db.Model(&Event{}).Where("issue_hash != ''").Distinct("issue_hash").Pluck("issue_hash", &hashes).Error; err != nil {
		return err
	}
	return RecountIssues(db, hashes)
}
