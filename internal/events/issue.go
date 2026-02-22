// Package events provides issue tracking for LogNorth
package events

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"
)

// IssueSeen tracks when a user last saw an issue and when it last occurred
type IssueSeen struct {
	ID               uint      `gorm:"primaryKey" json:"id"`
	ErrorHash        string    `gorm:"uniqueIndex;size:64" json:"error_hash"`
	SeenAt           time.Time `json:"seen_at"`
	LastOccurrenceAt time.Time `json:"last_occurrence_at"` // For regression detection
	LastNotifiedAt   time.Time `json:"last_notified_at"`   // Prevent notification spam
	Muted            bool      `gorm:"default:false" json:"muted"`
	MutedAt          time.Time `json:"muted_at,omitempty"`
}

// TableName specifies the table name
func (IssueSeen) TableName() string {
	return "issues_seen"
}

// AutoMigrateIssues creates the issues_seen table
func AutoMigrateIssues(db *gorm.DB) error {
	return db.AutoMigrate(&IssueSeen{})
}

// hashError creates a consistent hash for error grouping scoped to an app (legacy)
// Priority: error_type+location > error message
// Kept for backward compat with events missing the new columns.
func hashError(ctx map[string]any, apiKeyID uint) string {
	var input string

	if errType, ok := ctx["error_type"].(string); ok && errType != "" {
		// Priority 1: error_type + location (SDK-extracted)
		loc, _ := ctx["error_location"].(string)
		input = fmt.Sprintf("%s:%s", errType, loc)
	} else {
		// Fallback: error message (backward compatibility)
		input, _ = ctx["error"].(string)
	}

	if apiKeyID > 0 {
		input = fmt.Sprintf("%s|%d", input, apiKeyID)
	}

	h := sha256.Sum256([]byte(input))
	return hex.EncodeToString(h[:])
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
	FirstSeen  string   `json:"first_seen"`
	LastSeen   string   `json:"last_seen"`
	Paths      []string `json:"paths"`
	Unseen     bool     `json:"unseen"` // New occurrences since last viewed
	Active     bool     `json:"active"` // Had occurrences in last 24h
	Muted      bool     `json:"muted"`  // User muted this issue
}

// IssueAggregateRow represents a row from SQL aggregation
type IssueAggregateRow struct {
	Error         string `gorm:"column:error"`
	ErrorClass    string `gorm:"column:error_class"`
	ErrorFile     string `gorm:"column:error_file"`
	ErrorLine     int    `gorm:"column:error_line"`
	ErrorType     string `gorm:"column:error_type"`
	ErrorLocation string `gorm:"column:error_location"`
	GroupHash     string `gorm:"column:group_hash"`
	APIKeyID      uint   `gorm:"column:api_key_id"`
	Count         int64  `gorm:"column:count"`
	FirstSeen     string `gorm:"column:first_seen"` // SQLite returns string from MIN()
	LastSeen      string `gorm:"column:last_seen"`  // SQLite returns string from MAX()
	Paths         string `gorm:"column:paths"`      // Comma-separated from GROUP_CONCAT
}

// ListIssuesWithState retrieves issues using SQL aggregation (scales to 100k+ events)
// Issues are always scoped per-app (grouped by error + api_key_id)
// Uses three-tier hashing: prefers error_hash_3, falls back to error_hash_2, then error_hash_1.
// For old events without hashes, falls back to legacy json_extract grouping.
func ListIssuesWithState(db *gorm.DB, apiKeyID uint, limit int) ([]IssueWithState, error) {
	if limit <= 0 {
		limit = 50
	}

	// SQL aggregation using the new hash columns
	// Group by the best available hash (h3 > h2 > h1), falling back to legacy for old events
	query := `
		SELECT
			COALESCE(NULLIF(error_message, ''), json_extract(context, '$.error')) as error,
			error_class,
			error_file,
			error_line,
			json_extract(context, '$.error_type') as error_type,
			json_extract(context, '$.error_location') as error_location,
			COALESCE(NULLIF(error_hash_3, ''), NULLIF(error_hash_2, ''), NULLIF(error_hash_1, '')) as group_hash,
			api_key_id,
			COUNT(*) as count,
			MIN(timestamp) as first_seen,
			MAX(timestamp) as last_seen,
			GROUP_CONCAT(DISTINCT json_extract(context, '$.path')) as paths
		FROM events
		WHERE is_error = 1
			AND (
				error_hash_1 != ''
				OR json_extract(context, '$.error') IS NOT NULL AND json_extract(context, '$.error') != ''
				OR json_extract(context, '$.error_type') IS NOT NULL
			)
	`
	args := []any{}

	if apiKeyID > 0 {
		query += " AND api_key_id = ?"
		args = append(args, apiKeyID)
	}

	query += `
		GROUP BY COALESCE(
			NULLIF(error_hash_3, ''),
			NULLIF(error_hash_2, ''),
			NULLIF(error_hash_1, ''),
			json_extract(context, '$.error_type') || ':' || COALESCE(json_extract(context, '$.error_location'), ''),
			json_extract(context, '$.error')
		), api_key_id
		ORDER BY last_seen DESC
		LIMIT ?
	`
	args = append(args, limit)

	var rows []IssueAggregateRow
	if err := db.Raw(query, args...).Scan(&rows).Error; err != nil {
		return nil, err
	}

	// Load seen states
	var seenRecords []IssueSeen
	db.Find(&seenRecords)
	seenMap := make(map[string]IssueSeen)
	for _, s := range seenRecords {
		seenMap[s.ErrorHash] = s
	}

	now := time.Now()
	cutoff24h := now.Add(-24 * time.Hour)

	// Build result
	issues := make([]IssueWithState, 0, len(rows))
	for _, row := range rows {
		// Determine hash: use precomputed group_hash, or fall back to legacy hashError
		hash := row.GroupHash
		if hash == "" {
			ctx := map[string]any{
				"error":          row.Error,
				"error_type":     row.ErrorType,
				"error_location": row.ErrorLocation,
			}
			hash = hashError(ctx, row.APIKeyID)
		}

		// Parse paths from GROUP_CONCAT result
		paths := []string{}
		if row.Paths != "" {
			for _, p := range splitPaths(row.Paths) {
				if len(paths) < 10 {
					paths = append(paths, p)
				}
			}
		}

		lastSeenTime := parseSQLiteTimestamp(row.LastSeen)

		issue := IssueWithState{
			Error:      row.Error,
			ErrorClass: row.ErrorClass,
			ErrorFile:  row.ErrorFile,
			ErrorLine:  row.ErrorLine,
			Hash:       hash,
			APIKeyID:   row.APIKeyID,
			Count:      row.Count,
			FirstSeen:  row.FirstSeen,
			LastSeen:   row.LastSeen,
			Paths:      paths,
			Active:     lastSeenTime.After(cutoff24h),
		}

		// Check seen state
		if seen, exists := seenMap[hash]; exists {
			issue.Unseen = lastSeenTime.After(seen.SeenAt)
			issue.Muted = seen.Muted
		} else {
			issue.Unseen = true
		}

		issues = append(issues, issue)
	}

	// Sort: Active+Unseen first, then Active, then by last_seen
	sortIssues(issues)

	return issues, nil
}

// parseSQLiteTimestamp tries multiple formats that SQLite may return
func parseSQLiteTimestamp(s string) time.Time {
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

func splitPaths(s string) []string {
	if s == "" {
		return []string{}
	}
	result := []string{}
	for _, p := range strings.Split(s, ",") {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}

func sortIssues(issues []IssueWithState) {
	for i := 0; i < len(issues)-1; i++ {
		for j := i + 1; j < len(issues); j++ {
			iScore := scoreIssue(issues[i])
			jScore := scoreIssue(issues[j])
			if jScore > iScore || (jScore == iScore && issues[j].LastSeen > issues[i].LastSeen) {
				issues[i], issues[j] = issues[j], issues[i]
			}
		}
	}
}

func scoreIssue(i IssueWithState) int {
	score := 0
	if i.Active {
		score += 2
	}
	if i.Unseen {
		score += 1
	}
	return score
}

// RelatedLocation represents another location where the same error occurs.
//
// SameFile is true when the related location is in the same file as the current issue
// (hash_2 match — stronger signal). Otherwise it's a hash_1 match (same error, different file).
type RelatedLocation struct {
	File     string `json:"file"`
	Line     int    `json:"line"`
	Count    int64  `json:"count"`
	Hash     string `json:"hash"`      // error_hash_3 for linking
	SameFile bool   `json:"same_file"` // true = hash_2 match (same file)
}

// GetRelatedLocations finds other file+line locations where the same error (h1) occurs.
// currentFile is used to flag hash_2 relationships (same error in the same file).
func GetRelatedLocations(db *gorm.DB, hash1, currentHash3, currentFile string, apiKeyID uint) ([]RelatedLocation, error) {
	if hash1 == "" {
		return nil, nil
	}

	query := `
		SELECT error_file, error_line, error_hash_3, COUNT(*) as count
		FROM events
		WHERE error_hash_1 = ? AND api_key_id = ? AND error_hash_3 != ? AND error_file != ''
		GROUP BY error_file, error_line
		ORDER BY count DESC
		LIMIT 10
	`

	type row struct {
		ErrorFile  string `gorm:"column:error_file"`
		ErrorLine  int    `gorm:"column:error_line"`
		ErrorHash3 string `gorm:"column:error_hash_3"`
		Count      int64  `gorm:"column:count"`
	}

	var rows []row
	if err := db.Raw(query, hash1, apiKeyID, currentHash3).Scan(&rows).Error; err != nil {
		return nil, err
	}

	locations := make([]RelatedLocation, 0, len(rows))
	for _, r := range rows {
		locations = append(locations, RelatedLocation{
			File:     r.ErrorFile,
			Line:     r.ErrorLine,
			Count:    r.Count,
			Hash:     r.ErrorHash3,
			SameFile: currentFile != "" && r.ErrorFile == currentFile,
		})
	}
	return locations, nil
}

// IssueDetail contains full issue info with log entries
type IssueDetail struct {
	IssueWithState
	APIKeyID uint    `json:"api_key_id"`
	Entries  []Event `json:"entries"`
}

// GetIssueByHash retrieves a single issue with all its events.
// Tries new hash columns first, falls back to legacy scan for old events.
func GetIssueByHash(db *gorm.DB, hash string, apiKeyID uint) (*IssueDetail, error) {
	// Try matching by new hash columns first (fast path)
	eventsQuery := db.Model(&Event{}).Where("is_error = ?", true)
	if apiKeyID > 0 {
		eventsQuery = eventsQuery.Where("api_key_id = ?", apiKeyID)
	}
	eventsQuery = eventsQuery.Where("error_hash_3 = ? OR error_hash_2 = ? OR error_hash_1 = ?", hash, hash, hash)

	var matchingEvents []Event
	if err := eventsQuery.Order("timestamp DESC").Limit(100).Find(&matchingEvents).Error; err != nil {
		return nil, err
	}

	// If no results from new hashes, try legacy lookup
	if len(matchingEvents) == 0 {
		return getIssueByHashLegacy(db, hash, apiKeyID)
	}

	// Determine apiKeyID from first result if not scoped
	if apiKeyID == 0 {
		apiKeyID = matchingEvents[0].APIKeyID
	}

	// Get aggregation data
	aggQuery := db.Model(&Event{}).Where("is_error = ? AND api_key_id = ?", true, apiKeyID).
		Where("error_hash_3 = ? OR error_hash_2 = ? OR error_hash_1 = ?", hash, hash, hash)

	var count int64
	aggQuery.Count(&count)

	var aggResult struct {
		FirstSeen string `gorm:"column:first_seen"`
		LastSeen  string `gorm:"column:last_seen"`
		Paths     string `gorm:"column:paths"`
	}
	db.Raw(`
		SELECT
			MIN(timestamp) as first_seen,
			MAX(timestamp) as last_seen,
			GROUP_CONCAT(DISTINCT json_extract(context, '$.path')) as paths
		FROM events
		WHERE is_error = 1 AND api_key_id = ?
			AND (error_hash_3 = ? OR error_hash_2 = ? OR error_hash_1 = ?)
	`, apiKeyID, hash, hash, hash).Scan(&aggResult)

	paths := splitPaths(aggResult.Paths)
	if len(paths) > 10 {
		paths = paths[:10]
	}

	lastSeenTime := parseSQLiteTimestamp(aggResult.LastSeen)
	now := time.Now()
	active := lastSeenTime.After(now.Add(-24 * time.Hour))

	var seenRecord IssueSeen
	db.Where("error_hash = ?", hash).First(&seenRecord)
	unseen := seenRecord.ID == 0 || lastSeenTime.After(seenRecord.SeenAt)

	first := matchingEvents[0]
	return &IssueDetail{
		IssueWithState: IssueWithState{
			Error:      first.ErrorMessage,
			ErrorClass: first.ErrorClass,
			ErrorFile:  first.ErrorFile,
			ErrorLine:  first.ErrorLine,
			Hash:       hash,
			Count:      count,
			FirstSeen:  aggResult.FirstSeen,
			LastSeen:   aggResult.LastSeen,
			Paths:      paths,
			Unseen:     unseen,
			Active:     active,
			Muted:      seenRecord.Muted,
		},
		APIKeyID: apiKeyID,
		Entries:  matchingEvents,
	}, nil
}

// getIssueByHashLegacy handles lookup for old events without hash columns
func getIssueByHashLegacy(db *gorm.DB, hash string, apiKeyID uint) (*IssueDetail, error) {
	errorFilter := findErrorFilterByHash(db, hash, apiKeyID)
	if errorFilter == nil {
		return nil, nil
	}

	eventsQuery := db.Model(&Event{}).
		Where("is_error = ?", true)
	if apiKeyID > 0 {
		eventsQuery = eventsQuery.Where("api_key_id = ?", apiKeyID)
	}

	if errorFilter.ErrorType != "" {
		eventsQuery = eventsQuery.Where("json_extract(context, '$.error_type') = ? AND COALESCE(json_extract(context, '$.error_location'), '') = ?",
			errorFilter.ErrorType, errorFilter.ErrorLocation)
	} else {
		eventsQuery = eventsQuery.Where("json_extract(context, '$.error') = ?", errorFilter.Error)
	}

	var matchingEvents []Event
	if err := eventsQuery.Order("timestamp DESC").Limit(100).Find(&matchingEvents).Error; err != nil {
		return nil, err
	}
	if len(matchingEvents) == 0 {
		return nil, nil
	}

	if apiKeyID == 0 {
		apiKeyID = matchingEvents[0].APIKeyID
	}

	var aggResult struct {
		Count     int64  `gorm:"column:count"`
		FirstSeen string `gorm:"column:first_seen"`
		LastSeen  string `gorm:"column:last_seen"`
		Paths     string `gorm:"column:paths"`
	}

	var whereClause string
	var whereArgs []any
	if errorFilter.ErrorType != "" {
		whereClause = "json_extract(context, '$.error_type') = ? AND COALESCE(json_extract(context, '$.error_location'), '') = ?"
		whereArgs = []any{errorFilter.ErrorType, errorFilter.ErrorLocation}
	} else {
		whereClause = "json_extract(context, '$.error') = ?"
		whereArgs = []any{errorFilter.Error}
	}

	aggQuery := fmt.Sprintf(`
		SELECT COUNT(*) as count, MIN(timestamp) as first_seen, MAX(timestamp) as last_seen,
			GROUP_CONCAT(DISTINCT json_extract(context, '$.path')) as paths
		FROM events WHERE is_error = 1 AND api_key_id = ? AND %s
	`, whereClause)
	db.Raw(aggQuery, append([]any{apiKeyID}, whereArgs...)...).Scan(&aggResult)

	paths := splitPaths(aggResult.Paths)
	if len(paths) > 10 {
		paths = paths[:10]
	}

	lastSeenTime := parseSQLiteTimestamp(aggResult.LastSeen)
	active := lastSeenTime.After(time.Now().Add(-24 * time.Hour))

	var seenRecord IssueSeen
	db.Where("error_hash = ?", hash).First(&seenRecord)
	unseen := seenRecord.ID == 0 || lastSeenTime.After(seenRecord.SeenAt)

	return &IssueDetail{
		IssueWithState: IssueWithState{
			Error:     errorFilter.Error,
			Hash:      hash,
			Count:     aggResult.Count,
			FirstSeen: aggResult.FirstSeen,
			LastSeen:  aggResult.LastSeen,
			Paths:     paths,
			Unseen:    unseen,
			Active:    active,
			Muted:     seenRecord.Muted,
		},
		APIKeyID: apiKeyID,
		Entries:  matchingEvents,
	}, nil
}

// MarkIssueSeen marks an issue as seen
func MarkIssueSeen(db *gorm.DB, errorHash string) error {
	var seen IssueSeen
	result := db.Where("error_hash = ?", errorHash).First(&seen)

	if result.Error == gorm.ErrRecordNotFound {
		seen = IssueSeen{
			ErrorHash: errorHash,
			SeenAt:    time.Now(),
		}
		return db.Create(&seen).Error
	}

	seen.SeenAt = time.Now()
	return db.Save(&seen).Error
}

// MuteIssue mutes an issue to prevent alerts (also marks as seen)
func MuteIssue(db *gorm.DB, errorHash string) error {
	now := time.Now()
	var seen IssueSeen
	result := db.Where("error_hash = ?", errorHash).First(&seen)

	if result.Error == gorm.ErrRecordNotFound {
		seen = IssueSeen{
			ErrorHash: errorHash,
			SeenAt:    now,
			Muted:     true,
			MutedAt:   now,
		}
		return db.Create(&seen).Error
	}

	seen.SeenAt = now
	seen.Muted = true
	seen.MutedAt = now
	return db.Save(&seen).Error
}

// UnmuteIssue unmutes an issue to allow alerts
func UnmuteIssue(db *gorm.DB, errorHash string) error {
	return db.Model(&IssueSeen{}).
		Where("error_hash = ?", errorHash).
		Updates(map[string]any{"muted": false, "muted_at": nil}).Error
}

// IsIssueMuted checks if an issue is muted
func IsIssueMuted(db *gorm.DB, errorHash string) bool {
	var seen IssueSeen
	result := db.Where("error_hash = ? AND muted = ?", errorHash, true).First(&seen)
	return result.Error == nil
}

// OccurrencePoint represents hourly occurrence count for frequency graphs
type OccurrencePoint struct {
	Hour  string `json:"hour"`
	Count int    `json:"count"`
}

// GetIssueOccurrencesByHour returns hourly occurrence counts for an issue over the last N days
func GetIssueOccurrencesByHour(db *gorm.DB, hash string, apiKeyID uint, days int) ([]OccurrencePoint, error) {
	if days <= 0 {
		days = 7
	}

	cutoff := time.Now().AddDate(0, 0, -days).UTC().Format("2006-01-02 15:04:05")

	// Try new hash columns first
	var hashCount int64
	db.Model(&Event{}).Where("(error_hash_3 = ? OR error_hash_2 = ? OR error_hash_1 = ?) AND api_key_id = ?",
		hash, hash, hash, apiKeyID).Count(&hashCount)

	if hashCount > 0 {
		query := `
			SELECT strftime('%Y-%m-%d %H', timestamp) as hour, COUNT(*) as count
			FROM events
			WHERE is_error = 1 AND api_key_id = ? AND datetime(timestamp) >= datetime(?)
				AND (error_hash_3 = ? OR error_hash_2 = ? OR error_hash_1 = ?)
			GROUP BY strftime('%Y-%m-%d %H', timestamp)
			ORDER BY hour ASC
		`
		var points []OccurrencePoint
		if err := db.Raw(query, apiKeyID, cutoff, hash, hash, hash).Scan(&points).Error; err != nil {
			return nil, err
		}
		return points, nil
	}

	// Legacy fallback: find the issue to get its grouping criteria
	issue, err := GetIssueByHash(db, hash, apiKeyID)
	if err != nil || issue == nil {
		return nil, err
	}
	if len(issue.Entries) == 0 {
		return nil, nil
	}

	firstEntry := issue.Entries[0]
	ctx, err := firstEntry.GetContext()
	if err != nil {
		return nil, err
	}

	var whereClause string
	var whereArgs []any
	if ctx.ErrorType != "" {
		whereClause = "json_extract(context, '$.error_type') = ? AND COALESCE(json_extract(context, '$.error_location'), '') = ?"
		whereArgs = []any{ctx.ErrorType, ctx.ErrorLocation}
	} else {
		whereClause = "json_extract(context, '$.error') = ?"
		whereArgs = []any{ctx.Error}
	}

	query := fmt.Sprintf(`
		SELECT strftime('%%Y-%%m-%%d %%H', timestamp) as hour, COUNT(*) as count
		FROM events
		WHERE is_error = 1 AND api_key_id = ? AND datetime(timestamp) >= datetime(?) AND %s
		GROUP BY strftime('%%Y-%%m-%%d %%H', timestamp)
		ORDER BY hour ASC
	`, whereClause)

	args := append([]any{apiKeyID, cutoff}, whereArgs...)
	var points []OccurrencePoint
	if err := db.Raw(query, args...).Scan(&points).Error; err != nil {
		return nil, err
	}
	return points, nil
}

// GetIssueHashForEvent returns the best available issue hash for an error event.
// Prefers new hash columns, falls back to legacy computation.
func GetIssueHashForEvent(event *Event) string {
	if !event.IsError {
		return ""
	}
	// Prefer precomputed hashes
	if event.ErrorHash3 != "" {
		return event.ErrorHash3
	}
	if event.ErrorHash2 != "" {
		return event.ErrorHash2
	}
	if event.ErrorHash1 != "" {
		return event.ErrorHash1
	}
	// Legacy fallback
	ctx, err := event.GetContext()
	if err != nil {
		return ""
	}
	if ctx.Error == "" && ctx.ErrorType == "" {
		return ""
	}
	return hashError(ctx.ToMap(), event.APIKeyID)
}

// MarkAllIssuesSeen marks all issues as seen
func MarkAllIssuesSeen(db *gorm.DB, apiKeyID uint) error {
	// Get all current error hashes
	query := db.Model(&Event{}).Where("is_error = ?", true)
	if apiKeyID > 0 {
		query = query.Where("api_key_id = ?", apiKeyID)
	}

	var eventList []Event
	if err := query.Find(&eventList).Error; err != nil {
		return err
	}

	now := time.Now()
	seen := make(map[string]bool)

	for _, event := range eventList {
		ctx, err := event.GetContext()
		if err != nil {
			continue
		}
		// Skip if no error identifier (error message or error_type)
		if ctx.Error == "" && ctx.ErrorType == "" {
			continue
		}

		hash := hashError(ctx.ToMap(), event.APIKeyID)
		if seen[hash] {
			continue
		}
		seen[hash] = true

		var record IssueSeen
		result := db.Where("error_hash = ?", hash).First(&record)
		if result.Error == gorm.ErrRecordNotFound {
			db.Create(&IssueSeen{ErrorHash: hash, SeenAt: now})
		} else {
			record.SeenAt = now
			db.Save(&record)
		}
	}

	return nil
}
