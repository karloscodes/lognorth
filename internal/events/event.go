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
	// hash_3 is the default grouping key. When file/line are missing, falls back to hash_1.
	// hash_1 links related locations ("also happens at") — same error, different files.
	// Notifications use hash_3 when available, hash_1 otherwise.
	ErrorHash1 string `gorm:"column:error_hash_1;size:64" json:"error_hash_1"`
	ErrorHash2 string `gorm:"column:error_hash_2;size:64" json:"error_hash_2"`
	ErrorHash3 string `gorm:"column:error_hash_3;size:64;index" json:"error_hash_3"`
}

// TableName specifies the table name
func (Event) TableName() string {
	return "events"
}

// AutoMigrate creates the events table if it doesn't exist
func AutoMigrate(db *gorm.DB) error {
	return db.AutoMigrate(&Event{})
}

// EventContext represents the parsed Context field
type EventContext struct {
	// HTTP request fields
	Method string `json:"method,omitempty"`
	Path   string `json:"path,omitempty"`
	Status int    `json:"status,omitempty"`

	// Job fields
	Job string `json:"job,omitempty"`

	// Common fields
	Error     string            `json:"error,omitempty"`
	User      string            `json:"user,omitempty"`
	IP        string            `json:"ip,omitempty"`
	UserAgent string            `json:"user_agent,omitempty"`
	Headers   map[string]string `json:"headers,omitempty"`
	Body      string            `json:"body,omitempty"`

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

// ToMap converts EventContext to a map for hashError
func (c *EventContext) ToMap() map[string]any {
	m := make(map[string]any)
	if c.Error != "" {
		m["error"] = c.Error
	}
	if c.ErrorType != "" {
		m["error_type"] = c.ErrorType
	}
	if c.ErrorLocation != "" {
		m["error_location"] = c.ErrorLocation
	}
	return m
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

// DeriveIsError determines if this event is an error based on context
func DeriveIsError(ctx *EventContext) bool {
	if ctx.Error != "" {
		return true
	}
	if ctx.ErrorClass != "" {
		return true
	}
	if ctx.ErrorType != "" {
		return true
	}
	if ctx.Status >= 500 {
		return true
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
}

// Create inserts a new event
func Create(db *gorm.DB, event *Event) error {
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now()
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

// errorFilterMatch holds the criteria to match events for a specific error hash
type errorFilterMatch struct {
	Error         string
	ErrorType     string
	ErrorLocation string
}

// findErrorFilterByHash looks up the error grouping that matches the given hash
func findErrorFilterByHash(db *gorm.DB, hash string, apiKeyID uint) *errorFilterMatch {
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
		return nil
	}

	for _, row := range errorRows {
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
			return &errorFilterMatch{
				Error:         row.Error,
				ErrorType:     row.ErrorType,
				ErrorLocation: row.ErrorLocation,
			}
		}
	}
	return nil
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
		query = query.Where("timestamp >= ?", filters.StartTime)
	}
	if !filters.EndTime.IsZero() {
		query = query.Where("timestamp <= ?", filters.EndTime)
	}
	if filters.ErrorHash != "" {
		// Try matching by new hash columns first
		var hashCount int64
		db.Model(&Event{}).Where("error_hash_3 = ? OR error_hash_2 = ? OR error_hash_1 = ?",
			filters.ErrorHash, filters.ErrorHash, filters.ErrorHash).Count(&hashCount)

		if hashCount > 0 {
			query = query.Where("error_hash_3 = ? OR error_hash_2 = ? OR error_hash_1 = ?",
				filters.ErrorHash, filters.ErrorHash, filters.ErrorHash)
		} else {
			// Fall back to legacy lookup for old events
			errorFilter := findErrorFilterByHash(db, filters.ErrorHash, filters.APIKeyID)
			if errorFilter != nil {
				if errorFilter.ErrorType != "" {
					query = query.Where("json_extract(context, '$.error_type') = ? AND COALESCE(json_extract(context, '$.error_location'), '') = ?",
						errorFilter.ErrorType, errorFilter.ErrorLocation)
				} else {
					query = query.Where("json_extract(context, '$.error') = ?", errorFilter.Error)
				}
			} else {
				query = query.Where("1 = 0")
			}
		}
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

// DeleteOlderThan removes events older than the specified duration
func DeleteOlderThan(db *gorm.DB, duration time.Duration) (int64, error) {
	cutoff := time.Now().Add(-duration)
	result := db.Where("timestamp < ?", cutoff).Delete(&Event{})
	return result.RowsAffected, result.Error
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
	Path      string  `json:"path"`
	Total     int64   `json:"total"`
	Errors    int64   `json:"errors"`
	ErrorRate float64 `json:"error_rate"`
	AvgDuration int   `json:"avg_duration"`
}

// GetEndpointStats returns health metrics grouped by endpoint
func GetEndpointStats(db *gorm.DB, apiKeyID uint, limit int) ([]EndpointStats, error) {
	if limit <= 0 {
		limit = 20
	}

	query := `
		SELECT
			json_extract(context, '$.path') as path,
			COUNT(*) as total,
			SUM(CASE WHEN is_error THEN 1 ELSE 0 END) as errors,
			ROUND(100.0 * SUM(CASE WHEN is_error THEN 1 ELSE 0 END) / COUNT(*), 1) as error_rate,
			CAST(AVG(duration_ms) AS INTEGER) as avg_duration
		FROM events
		WHERE json_extract(context, '$.path') IS NOT NULL
			AND json_extract(context, '$.path') != ''
	`
	args := []any{}

	if apiKeyID > 0 {
		query += " AND api_key_id = ?"
		args = append(args, apiKeyID)
	}

	query += `
		GROUP BY path
		ORDER BY error_rate DESC, total DESC
		LIMIT ?
	`
	args = append(args, limit)

	var stats []EndpointStats
	if err := db.Raw(query, args...).Scan(&stats).Error; err != nil {
		return nil, err
	}

	return stats, nil
}

// GetStats returns event counts
func GetStats(db *gorm.DB, apiKeyID uint) Stats {
	var stats Stats

	errQ := db.Model(&Event{}).Where("is_error = ?", true)
	totalQ := db.Model(&Event{})
	todayQ := db.Model(&Event{}).Where("timestamp >= ?", time.Now().Truncate(24*time.Hour))

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

// GetHistogram returns hourly volume buckets for the given time range
func GetHistogram(db *gorm.DB, apiKeyID uint, startTime, endTime time.Time, buckets int) []HistogramBucket {
	if buckets <= 0 {
		buckets = 24 // default to 24 buckets
	}

	// Default time range: last 24 hours
	now := time.Now()
	if endTime.IsZero() {
		endTime = now
	}
	if startTime.IsZero() {
		startTime = endTime.Add(-24 * time.Hour)
	}

	// Calculate bucket duration
	totalDuration := endTime.Sub(startTime)
	bucketDuration := totalDuration / time.Duration(buckets)
	if bucketDuration < time.Minute {
		bucketDuration = time.Minute
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
		Where("timestamp >= ? AND timestamp < ?", startTime, endTime).
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
