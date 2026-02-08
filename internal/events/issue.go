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
}

// TableName specifies the table name
func (IssueSeen) TableName() string {
	return "issues_seen"
}

// AutoMigrateIssues creates the issues_seen table
func AutoMigrateIssues(db *gorm.DB) error {
	return db.AutoMigrate(&IssueSeen{})
}

// hashError creates a consistent hash for error grouping scoped to an app
// Priority: error_type+location > error message
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

// IssueWithState extends Issue with seen/active state
type IssueWithState struct {
	Error     string   `json:"error"`
	Hash      string   `json:"hash"`
	APIKeyID  uint     `json:"api_key_id"`
	Count     int64    `json:"count"`
	FirstSeen string   `json:"first_seen"`
	LastSeen  string   `json:"last_seen"`
	Paths     []string `json:"paths"`
	Unseen    bool     `json:"unseen"` // New occurrences since last viewed
	Active    bool     `json:"active"` // Had occurrences in last 24h
}

// IssueAggregateRow represents a row from SQL aggregation
type IssueAggregateRow struct {
	Error         string `gorm:"column:error"`
	ErrorType     string `gorm:"column:error_type"`
	ErrorLocation string `gorm:"column:error_location"`
	APIKeyID      uint   `gorm:"column:api_key_id"`
	Count         int64  `gorm:"column:count"`
	FirstSeen     string `gorm:"column:first_seen"` // SQLite returns string from MIN()
	LastSeen      string `gorm:"column:last_seen"`  // SQLite returns string from MAX()
	Paths         string `gorm:"column:paths"`      // Comma-separated from GROUP_CONCAT
}

// ListIssuesWithState retrieves issues using SQL aggregation (scales to 100k+ events)
// Issues are always scoped per-app (grouped by error + api_key_id)
func ListIssuesWithState(db *gorm.DB, apiKeyID uint, limit int) ([]IssueWithState, error) {
	if limit <= 0 {
		limit = 50
	}

	// SQL aggregation - group by error_type+location or error and api_key_id
	// Grouping key matches hashError priority: error_type:location > error
	query := `
		SELECT
			json_extract(context, '$.error') as error,
			json_extract(context, '$.error_type') as error_type,
			json_extract(context, '$.error_location') as error_location,
			api_key_id,
			COUNT(*) as count,
			MIN(timestamp) as first_seen,
			MAX(timestamp) as last_seen,
			GROUP_CONCAT(DISTINCT json_extract(context, '$.path')) as paths
		FROM events
		WHERE is_error = 1
			AND (json_extract(context, '$.error') IS NOT NULL AND json_extract(context, '$.error') != ''
				OR json_extract(context, '$.error_type') IS NOT NULL)
	`
	args := []interface{}{}

	if apiKeyID > 0 {
		query += " AND api_key_id = ?"
		args = append(args, apiKeyID)
	}

	query += `
		GROUP BY COALESCE(
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
	seenMap := make(map[string]time.Time)
	for _, s := range seenRecords {
		seenMap[s.ErrorHash] = s.SeenAt
	}

	now := time.Now()
	cutoff24h := now.Add(-24 * time.Hour)

	// Build result
	issues := make([]IssueWithState, 0, len(rows))
	for _, row := range rows {
		// Build context map for hashError
		ctx := map[string]any{
			"error":          row.Error,
			"error_type":     row.ErrorType,
			"error_location": row.ErrorLocation,
		}
		hash := hashError(ctx, row.APIKeyID)

		// Parse paths from GROUP_CONCAT result
		paths := []string{}
		if row.Paths != "" {
			for _, p := range splitPaths(row.Paths) {
				if len(paths) < 10 {
					paths = append(paths, p)
				}
			}
		}

		// Parse timestamps from SQLite strings
		lastSeenTime, _ := time.Parse(time.RFC3339, row.LastSeen)
		if lastSeenTime.IsZero() {
			// Try SQLite's default format
			lastSeenTime, _ = time.Parse("2006-01-02 15:04:05.999999999-07:00", row.LastSeen)
		}
		if lastSeenTime.IsZero() {
			lastSeenTime, _ = time.Parse("2006-01-02T15:04:05Z", row.LastSeen)
		}

		issue := IssueWithState{
			Error:     row.Error,
			Hash:      hash,
			APIKeyID:  row.APIKeyID,
			Count:     row.Count,
			FirstSeen: row.FirstSeen,
			LastSeen:  row.LastSeen,
			Paths:     paths,
			Active:    lastSeenTime.After(cutoff24h),
		}

		// Check seen state
		if seenAt, exists := seenMap[hash]; exists {
			issue.Unseen = lastSeenTime.After(seenAt)
		} else {
			issue.Unseen = true
		}

		issues = append(issues, issue)
	}

	// Sort: Active+Unseen first, then Active, then by last_seen
	sortIssues(issues)

	return issues, nil
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

// IssueDetail contains full issue info with log entries
type IssueDetail struct {
	IssueWithState
	APIKeyID uint    `json:"api_key_id"`
	Entries  []Event `json:"entries"`
}

// GetIssueByHash retrieves a single issue with all its events using SQL
func GetIssueByHash(db *gorm.DB, hash string, apiKeyID uint) (*IssueDetail, error) {
	// Get distinct error groupings with their api_key_ids to find the one matching this hash
	query := `
		SELECT DISTINCT
			json_extract(context, '$.error') as error,
			json_extract(context, '$.error_type') as error_type,
			json_extract(context, '$.error_location') as error_location,
			api_key_id
		FROM events
		WHERE is_error = 1
			AND (json_extract(context, '$.error') IS NOT NULL AND json_extract(context, '$.error') != ''
				OR json_extract(context, '$.error_type') IS NOT NULL)
	`
	args := []any{}
	if apiKeyID > 0 {
		query += " AND api_key_id = ?"
		args = append(args, apiKeyID)
	}

	type errorWithApp struct {
		Error         string `gorm:"column:error"`
		ErrorType     string `gorm:"column:error_type"`
		ErrorLocation string `gorm:"column:error_location"`
		APIKeyID      uint   `gorm:"column:api_key_id"`
	}
	var errorRows []errorWithApp
	if err := db.Raw(query, args...).Scan(&errorRows).Error; err != nil {
		return nil, err
	}

	// Find the error grouping that matches this hash
	var targetRow *errorWithApp
	var targetAPIKeyID uint
	for i, row := range errorRows {
		checkAPIKeyID := apiKeyID
		if apiKeyID == 0 {
			checkAPIKeyID = row.APIKeyID
		}
		ctx := map[string]any{
			"error":          row.Error,
			"error_type":     row.ErrorType,
			"error_location": row.ErrorLocation,
		}
		if hashError(ctx, checkAPIKeyID) == hash {
			targetRow = &errorRows[i]
			targetAPIKeyID = checkAPIKeyID
			break
		}
	}

	if targetRow == nil {
		return nil, nil
	}

	// Use the found apiKeyID for subsequent queries
	if apiKeyID == 0 {
		apiKeyID = targetAPIKeyID
	}

	// Build query conditions based on how this issue groups
	// Priority: error_type+location > error
	eventsQuery := db.Model(&Event{}).
		Where("is_error = ?", true).
		Where("api_key_id = ?", apiKeyID)

	var whereClause string
	var whereArgs []any
	if targetRow.ErrorType != "" {
		whereClause = "json_extract(context, '$.error_type') = ? AND COALESCE(json_extract(context, '$.error_location'), '') = ?"
		whereArgs = []any{targetRow.ErrorType, targetRow.ErrorLocation}
	} else {
		whereClause = "json_extract(context, '$.error') = ?"
		whereArgs = []any{targetRow.Error}
	}

	eventsQuery = eventsQuery.Where(whereClause, whereArgs...).Order("timestamp DESC")

	var matchingEvents []Event
	if err := eventsQuery.Limit(100).Find(&matchingEvents).Error; err != nil {
		return nil, err
	}

	if len(matchingEvents) == 0 {
		return nil, nil
	}

	// Get count and paths via aggregation
	var aggResult struct {
		Count     int64  `gorm:"column:count"`
		FirstSeen string `gorm:"column:first_seen"`
		LastSeen  string `gorm:"column:last_seen"`
		Paths     string `gorm:"column:paths"`
	}

	aggQuery := fmt.Sprintf(`
		SELECT
			COUNT(*) as count,
			MIN(timestamp) as first_seen,
			MAX(timestamp) as last_seen,
			GROUP_CONCAT(DISTINCT json_extract(context, '$.path')) as paths
		FROM events
		WHERE is_error = 1
			AND api_key_id = ?
			AND %s
	`, whereClause)
	aggArgs := append([]any{apiKeyID}, whereArgs...)

	if err := db.Raw(aggQuery, aggArgs...).Scan(&aggResult).Error; err != nil {
		return nil, err
	}

	// Parse paths
	paths := splitPaths(aggResult.Paths)
	if len(paths) > 10 {
		paths = paths[:10]
	}

	// Parse last_seen for state checks
	lastSeenTime, _ := time.Parse(time.RFC3339, aggResult.LastSeen)
	if lastSeenTime.IsZero() {
		lastSeenTime, _ = time.Parse("2006-01-02 15:04:05.999999999-07:00", aggResult.LastSeen)
	}
	if lastSeenTime.IsZero() {
		lastSeenTime, _ = time.Parse("2006-01-02T15:04:05Z", aggResult.LastSeen)
	}

	// Check seen state
	now := time.Now()
	cutoff24h := now.Add(-24 * time.Hour)
	active := lastSeenTime.After(cutoff24h)

	var seenRecord IssueSeen
	db.Where("error_hash = ?", hash).First(&seenRecord)
	unseen := seenRecord.ID == 0 || lastSeenTime.After(seenRecord.SeenAt)

	return &IssueDetail{
		IssueWithState: IssueWithState{
			Error:     targetRow.Error,
			Hash:      hash,
			Count:     aggResult.Count,
			FirstSeen: aggResult.FirstSeen,
			LastSeen:  aggResult.LastSeen,
			Paths:     paths,
			Unseen:    unseen,
			Active:    active,
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
