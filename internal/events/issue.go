// Package events provides issue tracking for LogNorth
package events

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"time"

	"gorm.io/gorm"
)

// IssueSeen tracks, per issue hash, when the user last saw the issue, when it
// last occurred (for regression alerts), and whether it is muted.
type IssueSeen struct {
	ID               uint      `gorm:"primaryKey" json:"id"`
	ErrorHash        string    `gorm:"uniqueIndex;size:64" json:"error_hash"`
	SeenAt           time.Time `json:"seen_at"`
	LastOccurrenceAt time.Time `json:"last_occurrence_at"` // For regression detection
	Muted            bool      `gorm:"default:false" json:"muted"`
}

// TableName specifies the table name
func (IssueSeen) TableName() string {
	return "issues_seen"
}

// computeErrorHashes returns three-tier hashes for error grouping.
//
//	h1: error_class + error_message (same error anywhere)
//	h2: h1 + error_file (same error, same file)
//	h3: h1 + error_file + error_line (exact location — primary grouping key)
//
// When error_class is empty, uses error_message only.
// When error_file/error_line missing, h2 and h3 stay empty.
func computeErrorHashes(errorClass, errorMessage, errorFile string, errorLine int, apiKeyID uint) (h1, h2, h3 string) {
	// Build the identity part: class + message
	identity := errorClass + ":" + errorMessage
	if errorClass == "" {
		identity = errorMessage
	}

	h1Input := fmt.Sprintf("%s|%d", identity, apiKeyID)
	sum := sha256.Sum256([]byte(h1Input))
	h1 = hex.EncodeToString(sum[:])

	if errorFile == "" {
		return h1, "", ""
	}

	h2Input := h1Input + ":" + errorFile
	sum = sha256.Sum256([]byte(h2Input))
	h2 = hex.EncodeToString(sum[:])

	if errorLine <= 0 {
		return h1, h2, ""
	}

	h3Input := fmt.Sprintf("%s:%d", h2Input, errorLine)
	sum = sha256.Sum256([]byte(h3Input))
	h3 = hex.EncodeToString(sum[:])

	return h1, h2, h3
}

// IssueWithState extends Issue with seen/active state
type IssueWithState struct {
	Error      string   `json:"error"`
	ErrorClass string   `json:"error_class,omitempty"`
	ErrorFile  string   `json:"error_file,omitempty"`
	ErrorLine  int      `json:"error_line,omitempty"`
	Hash       string   `json:"hash"`
	APIKeyID   uint     `json:"api_key_id"`
	Count      int64    `json:"count"`
	Count24h   int64    `json:"count_24h"` // Occurrences in last 24 hours
	Count7d    int64    `json:"count_7d"`  // Occurrences in last 7 days
	FirstSeen  string   `json:"first_seen"`
	LastSeen   string   `json:"last_seen"`
	Paths      []string `json:"paths"`
	Unseen     bool     `json:"unseen"` // New occurrences since last viewed
	Active     bool     `json:"active"` // Had occurrences in last 7 days
	Muted      bool     `json:"muted"`  // User muted this issue
	IsNew      bool     `json:"is_new"` // First seen < 24h ago
	Trend      string   `json:"trend"`  // "up", "down", "stable"
	Daily      []int64  `json:"daily"`  // Errors per UTC day, last 14 days, oldest first
}

// sparklineDays is how many days of daily counts each issue row carries.
const sparklineDays = 14

// issueRow is one issue as the aggregation query returns it.
type issueRow struct {
	Error      string `gorm:"column:error"`
	ErrorClass string `gorm:"column:error_class"`
	ErrorFile  string `gorm:"column:error_file"`
	ErrorLine  int    `gorm:"column:error_line"`
	IssueHash  string `gorm:"column:issue_hash"`
	APIKeyID   uint   `gorm:"column:api_key_id"`
	Count      int64  `gorm:"column:count"`
	FirstSeen  string `gorm:"column:first_seen"` // SQLite returns string from MIN()
	LastSeen   string `gorm:"column:last_seen"`  // SQLite returns string from MAX()
	Paths      string `gorm:"column:paths"`      // Comma-separated from GROUP_CONCAT
	Count24h   int64  `gorm:"column:count_24h"`
	Count7d    int64  `gorm:"column:count_7d"`
	Recent     int64  `gorm:"column:recent"`   // Last 2 hours, for the trend
	Previous   int64  `gorm:"column:previous"` // 2 to 4 hours ago, for the trend
}

// ListIssuesWithState returns the most recent issues, one per issue_hash, with
// their counts, trend, and seen state. An apiKeyID of 0 means every app.
func ListIssuesWithState(db *gorm.DB, apiKeyID uint, limit int) ([]IssueWithState, error) {
	if limit <= 0 {
		limit = 50
	}

	now := time.Now().UTC()
	query := `
		SELECT
			error_message as error,
			error_class,
			error_file,
			error_line,
			issue_hash,
			api_key_id,
			COUNT(*) as count,
			MIN(timestamp) as first_seen,
			MAX(timestamp) as last_seen,
			GROUP_CONCAT(DISTINCT json_extract(context, '$.path')) as paths,
			SUM(CASE WHEN timestamp >= ? THEN 1 ELSE 0 END) as count_24h,
			SUM(CASE WHEN timestamp >= ? THEN 1 ELSE 0 END) as count_7d,
			SUM(CASE WHEN timestamp >= ? THEN 1 ELSE 0 END) as recent,
			SUM(CASE WHEN timestamp >= ? AND timestamp < ? THEN 1 ELSE 0 END) as previous
		FROM events
		WHERE is_error = 1 AND issue_hash != ''
	`
	args := []any{
		now.Add(-24 * time.Hour), now.Add(-7 * 24 * time.Hour),
		now.Add(-2 * time.Hour), now.Add(-4 * time.Hour), now.Add(-2 * time.Hour),
	}
	if apiKeyID > 0 {
		query += " AND api_key_id = ?"
		args = append(args, apiKeyID)
	}
	query += " GROUP BY issue_hash ORDER BY last_seen DESC LIMIT ?"
	args = append(args, limit)

	var rows []issueRow
	if err := db.Raw(query, args...).Scan(&rows).Error; err != nil {
		return nil, err
	}

	hashes := make([]string, len(rows))
	for i, row := range rows {
		hashes[i] = row.IssueHash
	}
	seenMap := loadSeen(db, hashes)
	daily := dailyCounts(db, apiKeyID, hashes, now, sparklineDays)

	issues := make([]IssueWithState, 0, len(rows))
	for _, row := range rows {
		lastSeen := ParseSQLiteTimestamp(row.LastSeen)
		firstSeen := ParseSQLiteTimestamp(row.FirstSeen)

		issue := IssueWithState{
			Error:      row.Error,
			ErrorClass: row.ErrorClass,
			ErrorFile:  row.ErrorFile,
			ErrorLine:  row.ErrorLine,
			Hash:       row.IssueHash,
			APIKeyID:   row.APIKeyID,
			Count:      row.Count,
			Count24h:   row.Count24h,
			Count7d:    row.Count7d,
			FirstSeen:  formatTimestampISO(firstSeen),
			LastSeen:   formatTimestampISO(lastSeen),
			Paths:      firstPaths(row.Paths),
			Active:     lastSeen.After(now.Add(-7 * 24 * time.Hour)),
			IsNew:      firstSeen.After(now.Add(-24 * time.Hour)),
			Trend:      trend(row.Recent, row.Previous),
			Daily:      daily[row.IssueHash],
			Unseen:     true,
		}
		if issue.Daily == nil {
			issue.Daily = make([]int64, sparklineDays)
		}
		if seen, ok := seenMap[row.IssueHash]; ok {
			issue.Unseen = lastSeen.After(seen.SeenAt)
			issue.Muted = seen.Muted
		}
		issues = append(issues, issue)
	}

	// Active and unseen first, then active, then by last seen.
	sort.SliceStable(issues, func(i, j int) bool {
		si, sj := scoreIssue(issues[i]), scoreIssue(issues[j])
		if si != sj {
			return si > sj
		}
		return issues[i].LastSeen > issues[j].LastSeen
	})

	return issues, nil
}

// dailyCounts returns each issue's error count per UTC day for the last `days`
// days, oldest first. One grouped query covers the whole list, so no N+1.
func dailyCounts(db *gorm.DB, apiKeyID uint, hashes []string, now time.Time, days int) map[string][]int64 {
	counts := make(map[string][]int64, len(hashes))
	if len(hashes) == 0 {
		return counts
	}
	start := now.UTC().Truncate(24*time.Hour).AddDate(0, 0, -(days - 1))

	var rows []struct {
		IssueHash string
		Day       string
		Count     int64
	}
	query := db.Table("events").
		Select("issue_hash, date(timestamp) as day, COUNT(*) as count").
		Where("is_error = 1 AND issue_hash IN ? AND timestamp >= ?", hashes, start)
	if apiKeyID > 0 {
		query = query.Where("api_key_id = ?", apiKeyID)
	}
	if err := query.Group("issue_hash, day").Scan(&rows).Error; err != nil {
		return counts
	}

	for _, r := range rows {
		day, err := time.Parse("2006-01-02", r.Day)
		if err != nil {
			continue
		}
		i := int(day.Sub(start).Hours() / 24)
		if i < 0 || i >= days {
			continue
		}
		if counts[r.IssueHash] == nil {
			counts[r.IssueHash] = make([]int64, days)
		}
		counts[r.IssueHash][i] = r.Count
	}
	return counts
}

// loadSeen returns the seen state of the given issue hashes.
func loadSeen(db *gorm.DB, hashes []string) map[string]IssueSeen {
	seenMap := make(map[string]IssueSeen, len(hashes))
	if len(hashes) == 0 {
		return seenMap
	}
	var records []IssueSeen
	db.Where("error_hash IN ?", hashes).Find(&records)
	for _, r := range records {
		seenMap[r.ErrorHash] = r
	}
	return seenMap
}

// trend compares the last 2 hours with the 2 hours before.
func trend(recent, previous int64) string {
	switch {
	case recent > previous*2 && recent >= 3:
		return "up"
	case previous > recent*2 && previous >= 3:
		return "down"
	default:
		return "stable"
	}
}

func scoreIssue(i IssueWithState) int {
	score := 0
	if i.Active {
		score += 2
	}
	if i.Unseen {
		score++
	}
	return score
}

// formatTimestampISO renders a time as RFC3339Nano UTC, which the browser's
// `new Date()` parses unambiguously. Returns "" for the zero time so the
// frontend can distinguish "missing" from a real timestamp.
func formatTimestampISO(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339Nano)
}

// ParseSQLiteTimestamp tries multiple formats that SQLite may return
func ParseSQLiteTimestamp(s string) time.Time {
	for _, layout := range []string{
		time.RFC3339,
		"2006-01-02 15:04:05.999999999-07:00",
		"2006-01-02T15:04:05Z",
		"2006-01-02 15:04:05",
	} {
		if t, err := time.Parse(layout, s); err == nil {
			return t
		}
	}
	return time.Time{}
}

// firstPaths splits a GROUP_CONCAT result and keeps the first 10 paths.
func firstPaths(s string) []string {
	paths := []string{}
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" && len(paths) < 10 {
			paths = append(paths, p)
		}
	}
	return paths
}

// RelatedLocation represents another location where the same error occurs.
//
// SameFile is true when the related location is in the same file as the current issue
// (hash_2 match — stronger signal). Otherwise it's a hash_1 match (same error, different file).
type RelatedLocation struct {
	File     string `json:"file"`
	Line     int    `json:"line"`
	Count    int64  `json:"count"`
	Hash     string `json:"hash"`      // issue hash of that location, for linking
	SameFile bool   `json:"same_file"` // true = hash_2 match (same file)
}

// GetRelatedLocations finds other file+line locations where the same error (h1) occurs.
// currentFile is used to flag hash_2 relationships (same error in the same file).
func GetRelatedLocations(db *gorm.DB, hash1, currentIssueHash, currentFile string, apiKeyID uint) ([]RelatedLocation, error) {
	if hash1 == "" {
		return nil, nil
	}

	query := `
		SELECT error_file, error_line, issue_hash, COUNT(*) as count
		FROM events
		WHERE error_hash_1 = ? AND api_key_id = ? AND issue_hash != ? AND error_file != ''
		GROUP BY issue_hash
		ORDER BY count DESC
		LIMIT 10
	`

	type row struct {
		ErrorFile string `gorm:"column:error_file"`
		ErrorLine int    `gorm:"column:error_line"`
		IssueHash string `gorm:"column:issue_hash"`
		Count     int64  `gorm:"column:count"`
	}

	var rows []row
	if err := db.Raw(query, hash1, apiKeyID, currentIssueHash).Scan(&rows).Error; err != nil {
		return nil, err
	}

	locations := make([]RelatedLocation, 0, len(rows))
	for _, r := range rows {
		locations = append(locations, RelatedLocation{
			File:     r.ErrorFile,
			Line:     r.ErrorLine,
			Count:    r.Count,
			Hash:     r.IssueHash,
			SameFile: currentFile != "" && r.ErrorFile == currentFile,
		})
	}
	return locations, nil
}

// PathCount is how many times an issue happened on one endpoint.
type PathCount struct {
	Path  string `json:"path"`
	Count int64  `json:"count"`
}

// GetIssuePaths returns where an issue happened: its errors grouped by request
// path, most first. Events without a path (jobs, plain logs) are left out.
// Only error events have an issue hash, so the query names no is_error (see
// GetIssueReach).
func GetIssuePaths(db *gorm.DB, hash string, apiKeyID uint, limit int) ([]PathCount, error) {
	query := `
		SELECT json_extract(context, '$.path') as path, COUNT(*) as count
		FROM events
		WHERE issue_hash = ? AND api_key_id = ?
			AND json_extract(context, '$.path') IS NOT NULL AND json_extract(context, '$.path') != ''
		GROUP BY path
		ORDER BY count DESC, path
		LIMIT ?
	`
	paths := []PathCount{}
	err := db.Raw(query, hash, apiKeyID, limit).Scan(&paths).Error
	return paths, err
}

// IssueDetail contains full issue info with log entries
type IssueDetail struct {
	IssueWithState
	APIKeyID uint    `json:"api_key_id"`
	Entries  []Event `json:"entries"`
}

// GetIssueByHash returns one issue with its 100 most recent events, or nil
// when no event has that hash. An apiKeyID of 0 means every app.
func GetIssueByHash(db *gorm.DB, hash string, apiKeyID uint) (*IssueDetail, error) {
	eventsQuery := db.Where("issue_hash = ?", hash)
	if apiKeyID > 0 {
		eventsQuery = eventsQuery.Where("api_key_id = ?", apiKeyID)
	}

	var entries []Event
	if err := eventsQuery.Order("timestamp DESC, id DESC").Limit(100).Find(&entries).Error; err != nil {
		return nil, err
	}
	if len(entries) == 0 {
		return nil, nil
	}
	if apiKeyID == 0 {
		apiKeyID = entries[0].APIKeyID
	}

	var agg struct {
		Count     int64  `gorm:"column:count"`
		FirstSeen string `gorm:"column:first_seen"`
		LastSeen  string `gorm:"column:last_seen"`
		Paths     string `gorm:"column:paths"`
	}
	db.Raw(`
		SELECT COUNT(*) as count, MIN(timestamp) as first_seen, MAX(timestamp) as last_seen,
			GROUP_CONCAT(DISTINCT json_extract(context, '$.path')) as paths
		FROM events
		WHERE issue_hash = ? AND api_key_id = ?
	`, hash, apiKeyID).Scan(&agg)

	lastSeen := ParseSQLiteTimestamp(agg.LastSeen)
	firstSeen := ParseSQLiteTimestamp(agg.FirstSeen)

	var seen IssueSeen
	db.Where("error_hash = ?", hash).First(&seen)

	first := entries[0]
	return &IssueDetail{
		IssueWithState: IssueWithState{
			Error:      first.ErrorMessage,
			ErrorClass: first.ErrorClass,
			ErrorFile:  first.ErrorFile,
			ErrorLine:  first.ErrorLine,
			Hash:       hash,
			Count:      agg.Count,
			FirstSeen:  formatTimestampISO(firstSeen),
			LastSeen:   formatTimestampISO(lastSeen),
			Paths:      firstPaths(agg.Paths),
			Unseen:     seen.ID == 0 || lastSeen.After(seen.SeenAt),
			Active:     lastSeen.After(time.Now().Add(-7 * 24 * time.Hour)),
			Muted:      seen.Muted,
		},
		APIKeyID: apiKeyID,
		Entries:  entries,
	}, nil
}

// MarkIssueSeen marks an issue as seen
func MarkIssueSeen(db *gorm.DB, errorHash string) error {
	return upsertSeen(db, errorHash, map[string]any{"seen_at": time.Now()})
}

// MuteIssue mutes an issue to prevent alerts (also marks as seen)
func MuteIssue(db *gorm.DB, errorHash string) error {
	return upsertSeen(db, errorHash, map[string]any{"seen_at": time.Now(), "muted": true})
}

// UnmuteIssue unmutes an issue to allow alerts
func UnmuteIssue(db *gorm.DB, errorHash string) error {
	return db.Model(&IssueSeen{}).Where("error_hash = ?", errorHash).Update("muted", false).Error
}

// upsertSeen sets columns on an issue's seen row, creating the row if needed.
func upsertSeen(db *gorm.DB, errorHash string, columns map[string]any) error {
	var seen IssueSeen
	if err := db.Where("error_hash = ?", errorHash).First(&seen).Error; err == gorm.ErrRecordNotFound {
		seen = IssueSeen{ErrorHash: errorHash}
		if err := db.Create(&seen).Error; err != nil {
			return err
		}
	} else if err != nil {
		return err
	}
	return db.Model(&seen).Updates(columns).Error
}

// OccurrencePoint represents hourly occurrence count for frequency graphs.
// Hour is the start of the hour bucket as RFC3339 UTC (e.g. "2026-04-14T11:00:00Z")
// so the browser can `new Date(p.hour)` and group into the user's local timezone.
type OccurrencePoint struct {
	Hour  string `json:"hour"`
	Count int    `json:"count"`
}

// GetIssueOccurrencesByHour returns hourly occurrence counts for an issue over the last N days
func GetIssueOccurrencesByHour(db *gorm.DB, hash string, apiKeyID uint, days int) ([]OccurrencePoint, error) {
	if days <= 0 {
		days = 7
	}

	var points []OccurrencePoint
	err := db.Raw(`
		SELECT strftime('%Y-%m-%dT%H:00:00Z', timestamp) as hour, COUNT(*) as count
		FROM events
		WHERE issue_hash = ? AND api_key_id = ? AND timestamp >= ?
		GROUP BY hour
		ORDER BY hour ASC
	`, hash, apiKeyID, time.Now().UTC().AddDate(0, 0, -days)).Scan(&points).Error
	return points, err
}

// MarkAllIssuesSeen marks every issue as seen in one statement.
// An apiKeyID of 0 means every app.
func MarkAllIssuesSeen(db *gorm.DB, apiKeyID uint) error {
	query := `
		INSERT INTO issues_seen (error_hash, seen_at, last_occurrence_at, muted)
		SELECT DISTINCT issue_hash, ?, ?, false FROM events
		WHERE is_error = 1 AND issue_hash != ''`
	args := []any{time.Now(), time.Time{}}
	if apiKeyID > 0 {
		query += " AND api_key_id = ?"
		args = append(args, apiKeyID)
	}
	query += " ON CONFLICT(error_hash) DO UPDATE SET seen_at = excluded.seen_at"
	return db.Exec(query, args...).Error
}
