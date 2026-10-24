package events

import (
	"encoding/json"
	"fmt"
	"sort"
	"testing"
	"time"

	"gorm.io/gorm"
)

// oracleRow is one issue as the aggregation query returns it.
type oracleRow struct {
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

// listIssuesFromEvents is the issue list as it was built before the issues
// table: from every error event, on each load. It is the oracle that
// ListIssuesWithState must match.
// It returned the most recent issues, one per issue_hash, with
// their counts, trend, and seen state. An apiKeyID of 0 means every app.
func listIssuesFromEvents(db *gorm.DB, apiKeyID uint, limit int) ([]IssueWithState, error) {
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

	var rows []oracleRow
	if err := db.Raw(query, args...).Scan(&rows).Error; err != nil {
		return nil, err
	}

	hashes := make([]string, len(rows))
	for i, row := range rows {
		hashes[i] = row.IssueHash
	}
	seenMap := loadSeen(db, hashes)
	daily := oracleDailyCounts(db, apiKeyID, hashes, now, sparklineDays)

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
func oracleDailyCounts(db *gorm.DB, apiKeyID uint, hashes []string, now time.Time, days int) map[string][]int64 {
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

// The issue list from the issues table matches, byte for byte in JSON, the
// list built from every event, whether the processor kept the issues batch
// by batch or the data migration built them. north and the plugin read it.
func TestListIssuesWithStateMatchesTheEvents(t *testing.T) {
	db := setupTestDB(t)
	now := time.Now().UTC()
	type occurrence struct {
		app                  uint
		class, message, file string
		line                 int
		path                 string
		ago                  time.Duration
	}
	day := 24 * time.Hour
	var all []occurrence
	add := func(o occurrence, agos ...time.Duration) {
		for _, ago := range agos {
			o.ago = ago
			all = append(all, o)
		}
	}
	add(occurrence{app: 1, class: "TypeError", message: "boom", file: "app.ts", line: 7, path: "/checkout"},
		20*day, 16*day, 10*day, 6*day, 30*time.Hour, 23*time.Hour, 3*time.Hour, 90*time.Minute, 5*time.Minute)
	add(occurrence{app: 1, class: "TypeError", message: "boom", file: "app.ts", line: 7, path: "/cart"}, 9*day, 2*time.Hour+10*time.Minute)
	add(occurrence{app: 1, class: "Timeout", message: "db", path: ""}, 12*day, 40*time.Minute)
	add(occurrence{app: 1, class: "OldError", message: "gone quiet", file: "old.rb", line: 3, path: "/old"}, 18*day, 15*day)
	add(occurrence{app: 2, class: "NoMethodError", message: "nil", file: "users.rb", line: 12, path: "/users"}, 2*day, 3*time.Hour+20*time.Minute)
	for i := range 12 {
		add(occurrence{app: 2, class: "KeyError", message: "missing id", file: "show.rb", line: 4, path: fmt.Sprintf("/users/%d", i)},
			time.Duration(13-i)*day+time.Duration(i)*time.Minute)
	}
	sort.SliceStable(all, func(i, j int) bool { return all[i].ago > all[j].ago }) // oldest first: ids follow time

	batch := IssueBatch{}
	for i, o := range all {
		ctx, _ := json.Marshal(map[string]any{"error": o.message, "error_class": o.class, "error_file": o.file, "error_line": o.line, "path": o.path})
		event := &Event{APIKeyID: o.app, Message: "failed", IsError: true, Context: string(ctx), Timestamp: now.Add(-o.ago)}
		if err := Create(db, event); err != nil {
			t.Fatal(err)
		}
		var path string
		if c, err := event.GetContext(); err == nil {
			path = c.Path
		}
		if err := batch.Add(db, event, path); err != nil {
			t.Fatal(err)
		}
		if i%7 == 6 { // several batches, as several processor runs
			batch.Flush(db)
			batch = IssueBatch{}
		}
	}
	batch.Flush(db)
	var hashes []string
	db.Model(&Event{}).Distinct("issue_hash").Pluck("issue_hash", &hashes)
	sort.Strings(hashes)
	MarkIssueSeen(db, hashes[0])
	MuteIssue(db, hashes[1])
	MarkAllIssuesSeen(db, 2)

	compare := func(t *testing.T) {
		for _, c := range []struct {
			app   uint
			limit int
		}{{0, 200}, {1, 100}, {2, 100}, {0, 2}} {
			want, _ := listIssuesFromEvents(db, c.app, c.limit)
			got, err := ListIssuesWithState(db, c.app, c.limit)

			if err != nil {
				t.Fatalf("ListIssuesWithState: %v", err)
			}
			wantJSON, _ := json.Marshal(want)
			gotJSON, _ := json.Marshal(got)
			if string(gotJSON) != string(wantJSON) {
				t.Errorf("app %d, limit %d:\n got %s\nwant %s", c.app, c.limit, gotJSON, wantJSON)
			}
		}
	}

	t.Run("when the processor kept the issues", compare)

	t.Run("when the data migration built them", func(t *testing.T) {
		db.Exec("DELETE FROM issues")
		if err := RecountAllIssues(db); err != nil {
			t.Fatal(err)
		}
		compare(t)
	})

	t.Run("after retention deleted the events of 14 days ago and older", func(t *testing.T) {
		if _, err := DeleteOlderThan(db, 14*day); err != nil {
			t.Fatal(err)
		}
		var gone int64
		db.Model(&Issue{}).Where("error_class = ?", "OldError").Count(&gone)
		if gone != 0 {
			t.Errorf("the issue whose events are all deleted is still listed")
		}
		compare(t)
	})
}
