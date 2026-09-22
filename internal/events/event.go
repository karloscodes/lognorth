// Package events provides the core event model for LogNorth
package events

import (
	"encoding/json"
	"time"

	"gorm.io/gorm"
)

// Event represents a log event captured by LogNorth
// See docs/EVENT_MODEL.md for the full specification
type Event struct {
	ID         uint      `gorm:"primaryKey" json:"id"`
	APIKeyID   uint      `gorm:"index:idx_apikey_timestamp" json:"api_key_id"`
	Timestamp  time.Time `gorm:"index:idx_apikey_timestamp" json:"timestamp"`
	Message    string    `gorm:"size:1000" json:"message"`
	TraceID    string    `gorm:"index;size:100" json:"trace_id"`
	DurationMS int       `json:"duration_ms"`
	IsError    bool      `gorm:"index" json:"is_error"`
	Context    string    `gorm:"type:text" json:"context"` // JSON with all metadata
	CreatedAt  time.Time `json:"created_at"`

	// Structured error fields — extracted from context during event processing.
	// SDKs send error_class, error_file, error_line, error_caller, stack_trace
	// inside the context JSON. The event processor extracts them into columns
	// for fast SQL grouping and lookup.
	ErrorClass   string `gorm:"size:255" json:"error_class"`
	ErrorMessage string `gorm:"size:1000" json:"error_message"`
	ErrorFile    string `gorm:"size:500" json:"error_file"`
	ErrorLine    int    `gorm:"default:0" json:"error_line"`
	ErrorCaller  string `gorm:"size:255" json:"error_caller"`

	// Three-tier error hashes for issue grouping (all scoped by api_key_id):
	//   hash_1: hash(error_class + error_message) — same error anywhere in the app
	//   hash_2: hash_1 + error_file              — same error in the same file
	//   hash_3: hash_1 + error_file + error_line  — exact location (primary grouping key)
	//
	// hash_1 links related locations ("also happens at") — same error, different files.
	ErrorHash1 string `gorm:"column:error_hash_1;size:64;index" json:"error_hash_1"`
	ErrorHash2 string `gorm:"column:error_hash_2;size:64" json:"error_hash_2"`
	ErrorHash3 string `gorm:"column:error_hash_3;size:64" json:"error_hash_3"`

	// IssueHash groups events into issues: the most specific hash available
	// (hash_3, then hash_2, then hash_1). Issues, mute, seen state, alerts, and
	// the MCP issue filter all key on it.
	IssueHash string `gorm:"size:64;index" json:"issue_hash"`
}

// TableName specifies the table name
func (Event) TableName() string {
	return "events"
}

// EventContext represents the parsed Context field
type EventContext struct {
	// HTTP request fields
	Method string `json:"method,omitempty"`
	Path   string `json:"path,omitempty"`
	Status int    `json:"status,omitempty"`

	// Route enrichment — SDKs populate what their framework exposes.
	// Rails: controller + action. Express/chi/fiber: route pattern + handler name.
	// All optional; display in the UI only when present.
	Controller string `json:"controller,omitempty"`
	Action     string `json:"action,omitempty"`
	Route      string `json:"route,omitempty"`
	Handler    string `json:"handler,omitempty"`

	// Job fields
	Job string `json:"job,omitempty"`

	// Common fields
	Error       string            `json:"error,omitempty"`
	Environment string            `json:"environment,omitempty"`
	User        string            `json:"user,omitempty"`
	IP          string            `json:"ip,omitempty"`
	UserAgent   string            `json:"user_agent,omitempty"`
	Headers     map[string]string `json:"headers,omitempty"`
	Body        string            `json:"body,omitempty"`

	// Error grouping fields (SDK-extracted)
	ErrorType     string `json:"error_type,omitempty"`
	ErrorLocation string `json:"error_location,omitempty"`

	// Structured error fields (new SDKs)
	ErrorClass  string `json:"error_class,omitempty"`
	ErrorCaller string `json:"error_caller,omitempty"`
	ErrorFile   string `json:"error_file,omitempty"`
	ErrorLine   int    `json:"error_line,omitempty"`
	StackTrace  string `json:"stack_trace,omitempty"`

	// Custom fields (anything else)
	Extra map[string]any `json:"extra,omitempty"`
}

// GetContext parses and returns the event context
func (e *Event) GetContext() (*EventContext, error) {
	if e.Context == "" {
		return &EventContext{}, nil
	}
	var ctx EventContext
	if err := json.Unmarshal([]byte(e.Context), &ctx); err != nil {
		return nil, err
	}
	return &ctx, nil
}

// SetContext serializes and sets the event context
func (e *Event) SetContext(ctx *EventContext) error {
	bytes, err := json.Marshal(ctx)
	if err != nil {
		return err
	}
	e.Context = string(bytes)
	return nil
}

// IsErrorContext is the one rule for "this event is an error": the context
// names an error or an error class, or its HTTP status is 5xx.
func IsErrorContext(ctx map[string]any) bool {
	for _, key := range []string{"error", "error_class"} {
		if v, ok := ctx[key]; ok && v != nil && v != "" {
			return true
		}
	}
	switch v := ctx["status"].(type) {
	case float64:
		return v >= 500
	case int:
		return v >= 500
	case int64:
		return v >= 500
	}
	return false
}

// PopulateErrorFields extracts structured error fields from context and computes hashes.
// Called during event processing for error events.
func PopulateErrorFields(event *Event, ctx *EventContext) {
	// ErrorClass: prefer error_class, fall back to error_type
	event.ErrorClass = ctx.ErrorClass
	if event.ErrorClass == "" {
		event.ErrorClass = ctx.ErrorType
	}

	// ErrorMessage: from context.error
	event.ErrorMessage = ctx.Error

	// File, line, caller from context
	event.ErrorFile = ctx.ErrorFile
	event.ErrorLine = ctx.ErrorLine
	event.ErrorCaller = ctx.ErrorCaller

	// Compute three-tier hashes
	if event.ErrorClass != "" || event.ErrorMessage != "" {
		event.ErrorHash1, event.ErrorHash2, event.ErrorHash3 = computeErrorHashes(
			event.ErrorClass, event.ErrorMessage, event.ErrorFile, event.ErrorLine, event.APIKeyID,
		)
	}
	event.IssueHash = event.mostSpecificHash()
}

// mostSpecificHash returns hash_3, then hash_2, then hash_1, whichever is set first.
func (e *Event) mostSpecificHash() string {
	for _, h := range []string{e.ErrorHash3, e.ErrorHash2, e.ErrorHash1} {
		if h != "" {
			return h
		}
	}
	return ""
}

// Create inserts a new event. Timestamps are stored in UTC: SQLite compares
// them as text, so every row must use the same offset.
func Create(db *gorm.DB, event *Event) error {
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now()
	}
	event.Timestamp = event.Timestamp.UTC()
	// Every stored error event carries its error fields and issue hash,
	// whichever path stores it.
	if event.IsError && event.ErrorHash1 == "" {
		if ctx, err := event.GetContext(); err == nil {
			PopulateErrorFields(event, ctx)
		}
	}
	if event.IssueHash == "" {
		event.IssueHash = event.mostSpecificHash()
	}
	return db.Create(event).Error
}

// FindByID retrieves an event by ID
func FindByID(db *gorm.DB, id uint) (*Event, error) {
	var event Event
	if err := db.First(&event, id).Error; err != nil {
		return nil, err
	}
	return &event, nil
}

// FindByTraceID retrieves all events with the same trace ID
func FindByTraceID(db *gorm.DB, traceID string) ([]Event, error) {
	var events []Event
	if err := db.Where("trace_id = ?", traceID).Order("timestamp ASC").Find(&events).Error; err != nil {
		return nil, err
	}
	return events, nil
}

// FindNearby retrieves events around a given event by timestamp (3 before, 3 after)
func FindNearby(db *gorm.DB, appID uint, event *Event) ([]Event, error) {
	var before []Event
	db.Where("api_key_id = ? AND id != ? AND timestamp <= ?", appID, event.ID, event.Timestamp).
		Order("timestamp DESC, id DESC").Limit(3).Find(&before)

	var after []Event
	db.Where("api_key_id = ? AND id != ? AND timestamp >= ?", appID, event.ID, event.Timestamp).
		Order("timestamp ASC, id ASC").Limit(3).Find(&after)

	// Reverse before so it's chronological
	for i, j := 0, len(before)-1; i < j; i, j = i+1, j-1 {
		before[i], before[j] = before[j], before[i]
	}

	result := make([]Event, 0, len(before)+1+len(after))
	result = append(result, before...)
	result = append(result, *event)
	result = append(result, after...)
	return result, nil
}

// ListFilters defines filtering options for listing events
type ListFilters struct {
	APIKeyID  uint
	IsError   *bool  // nil = all, true = errors only, false = success only
	ErrorHash string // filter by error hash (grouped issue)
	TraceID   string // filter by trace ID
	Search    string
	StartTime time.Time
	EndTime   time.Time
	Limit     int
	Offset    int
}

// List retrieves events with optional filters
func List(db *gorm.DB, filters ListFilters) ([]Event, int64, error) {
	query := db.Model(&Event{})

	if filters.APIKeyID > 0 {
		query = query.Where("api_key_id = ?", filters.APIKeyID)
	}
	if filters.IsError != nil {
		query = query.Where("is_error = ?", *filters.IsError)
	}
	if !filters.StartTime.IsZero() {
		query = query.Where("timestamp >= ?", filters.StartTime.UTC())
	}
	if !filters.EndTime.IsZero() {
		query = query.Where("timestamp <= ?", filters.EndTime.UTC())
	}
	if filters.ErrorHash != "" {
		query = query.Where("issue_hash = ?", filters.ErrorHash)
	}
	if filters.TraceID != "" {
		query = query.Where("trace_id = ?", filters.TraceID)
	}
	if filters.Search != "" {
		search := "%" + filters.Search + "%"
		query = query.Where("message LIKE ? OR context LIKE ? OR trace_id LIKE ?", search, search, search)
	}

	// Get total count before pagination
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// Apply pagination
	if filters.Limit > 0 {
		query = query.Limit(filters.Limit)
	} else {
		query = query.Limit(50)
	}
	if filters.Offset > 0 {
		query = query.Offset(filters.Offset)
	}

	var events []Event
	if err := query.Order("timestamp DESC").Find(&events).Error; err != nil {
		return nil, 0, err
	}

	return events, total, nil
}

// DeleteOlderThan removes events older than the specified duration. It deletes
// in chunks, so ingest does not wait on one long write lock.
func DeleteOlderThan(db *gorm.DB, duration time.Duration) (int64, error) {
	cutoff := time.Now().UTC().Add(-duration)
	var total int64
	for {
		result := db.Exec("DELETE FROM events WHERE id IN (SELECT id FROM events WHERE timestamp < ? LIMIT 5000)", cutoff)
		if result.Error != nil {
			return total, result.Error
		}
		total += result.RowsAffected
		if result.RowsAffected < 5000 {
			return total, nil
		}
	}
}

// Stats holds event count statistics
type Stats struct {
	Errors     int64      `json:"errors"`
	Total      int64      `json:"total"`
	TodayTotal int64      `json:"today_total"`
	LastSeen   *time.Time `json:"last_seen"`
}

// EndpointStats holds per-endpoint health metrics
type EndpointStats struct {
	Path        string  `json:"path"`
	Total       int64   `json:"total"`
	Errors      int64   `json:"errors"`
	ErrorRate   float64 `json:"error_rate"`
	AvgDuration int     `json:"avg_duration"`
}

// EndpointStatsWindow is how far back the Endpoints page looks. It reads the
// 5-minute buckets in endpoint_stats, which are kept StatsRetention.
const EndpointStatsWindow = 28 * 24 * time.Hour

// GetEndpointStats returns health metrics per endpoint over the last
// EndpointStatsWindow, most broken first. It sums endpoint_stats buckets
// instead of parsing every event's JSON context.
func GetEndpointStats(db *gorm.DB, apiKeyID uint, limit int) ([]EndpointStats, error) {
	if limit <= 0 {
		limit = 20
	}

	query := `
		SELECT
			path,
			SUM(total) as total,
			SUM(errors) as errors,
			ROUND(100.0 * SUM(errors) / SUM(total), 1) as error_rate,
			CAST(SUM(avg_duration * total) / SUM(total) AS INTEGER) as avg_duration
		FROM endpoint_stats
		WHERE bucket >= ? AND total > 0
	`
	args := []any{time.Now().UTC().Add(-EndpointStatsWindow)}
	if apiKeyID > 0 {
		query += " AND api_key_id = ?"
		args = append(args, apiKeyID)
	}
	query += " GROUP BY path ORDER BY error_rate DESC, total DESC LIMIT ?"
	args = append(args, limit)

	var stats []EndpointStats
	err := db.Raw(query, args...).Scan(&stats).Error
	return stats, err
}

// GetStats returns event counts
func GetStats(db *gorm.DB, apiKeyID uint) Stats {
	var stats Stats

	errQ := db.Model(&Event{}).Where("is_error = ?", true)
	totalQ := db.Model(&Event{})
	todayQ := db.Model(&Event{}).Where("timestamp >= ?", time.Now().UTC().Truncate(24*time.Hour))

	if apiKeyID > 0 {
		errQ = errQ.Where("api_key_id = ?", apiKeyID)
		totalQ = totalQ.Where("api_key_id = ?", apiKeyID)
		todayQ = todayQ.Where("api_key_id = ?", apiKeyID)
	}

	errQ.Count(&stats.Errors)
	totalQ.Count(&stats.Total)
	todayQ.Count(&stats.TodayTotal)

	// Get last event timestamp
	var lastEvent Event
	lastQ := db.Model(&Event{}).Order("timestamp DESC").Limit(1)
	if apiKeyID > 0 {
		lastQ = lastQ.Where("api_key_id = ?", apiKeyID)
	}
	if lastQ.First(&lastEvent).Error == nil {
		stats.LastSeen = &lastEvent.Timestamp
	}

	return stats
}

// HistogramBucket represents a time bucket for the volume histogram
type HistogramBucket struct {
	Timestamp time.Time `json:"timestamp"`
	Total     int64     `json:"total"`
	Errors    int64     `json:"errors"`
}

// GetHistogram returns volume buckets for the given time range with dynamic granularity
// Minimum granularity is 1 hour, scales up for longer time ranges
func GetHistogram(db *gorm.DB, apiKeyID uint, startTime, endTime time.Time) []HistogramBucket {
	// Default time range: last 24 hours
	now := time.Now()
	if endTime.IsZero() {
		endTime = now
	}
	if startTime.IsZero() {
		startTime = endTime.Add(-24 * time.Hour)
	}

	// Round start time to the beginning of the hour for cleaner bucket boundaries
	startTime = startTime.Truncate(time.Hour)

	// Calculate total duration and choose appropriate bucket size
	totalDuration := endTime.Sub(startTime)

	// Choose bucket duration based on time range (minimum 1 hour)
	var bucketDuration time.Duration
	switch {
	case totalDuration <= 48*time.Hour:
		bucketDuration = time.Hour // 1 hour buckets for up to 2 days
	case totalDuration <= 14*24*time.Hour:
		bucketDuration = 6 * time.Hour // 6 hour buckets for up to 2 weeks
	case totalDuration <= 60*24*time.Hour:
		bucketDuration = 24 * time.Hour // daily buckets for up to 2 months
	default:
		bucketDuration = 7 * 24 * time.Hour // weekly buckets for longer
	}

	// Calculate number of buckets
	buckets := int(totalDuration / bucketDuration)
	if buckets < 1 {
		buckets = 1
	}
	if buckets > 100 {
		buckets = 100 // cap at 100 buckets
	}

	// Initialize buckets
	result := make([]HistogramBucket, buckets)
	for i := 0; i < buckets; i++ {
		result[i] = HistogramBucket{
			Timestamp: startTime.Add(time.Duration(i) * bucketDuration),
		}
	}

	// Query events grouped by bucket
	type bucketResult struct {
		BucketIdx int64
		IsError   bool
		Count     int64
	}

	var counts []bucketResult

	// Use SQLite's strftime for bucketing
	bucketSizeSeconds := int64(bucketDuration.Seconds())
	startUnix := startTime.Unix()

	query := db.Model(&Event{}).
		Select("((strftime('%s', timestamp) - ?) / ?) as bucket_idx, is_error, COUNT(*) as count", startUnix, bucketSizeSeconds).
		Where("timestamp >= ? AND timestamp < ?", startTime.UTC(), endTime.UTC()).
		Group("bucket_idx, is_error")

	if apiKeyID > 0 {
		query = query.Where("api_key_id = ?", apiKeyID)
	}

	if err := query.Find(&counts).Error; err != nil {
		return result
	}

	// Fill in the buckets
	for _, c := range counts {
		idx := int(c.BucketIdx)
		if idx >= 0 && idx < buckets {
			if c.IsError {
				result[idx].Errors = c.Count
			}
			result[idx].Total += c.Count
		}
	}

	return result
}
