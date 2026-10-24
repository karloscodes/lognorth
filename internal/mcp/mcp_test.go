package mcp

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/karloscodes/cartridge/sqlite"
	"gorm.io/gorm"

	"lognorth/internal/alerts"
	"lognorth/internal/apikeys"
	"lognorth/internal/database"
	"lognorth/internal/events"
	"lognorth/internal/uptime"
)

func setupTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "test.db")

	manager := sqlite.NewManager(sqlite.Config{Path: dbPath, EnableWAL: true})
	db, err := manager.Connect()
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	if err := database.Migrate(db); err != nil {
		t.Fatalf("Failed to migrate: %v", err)
	}

	return db
}

// seedApp creates an app with one failed and one successful request.
func seedApp(t *testing.T, db *gorm.DB, name string) uint {
	t.Helper()

	key, _, err := apikeys.Create(db, name)
	if err != nil {
		t.Fatalf("Failed to create app: %v", err)
	}

	failed := &events.Event{
		APIKeyID:     key.ID,
		Message:      "POST /pay 500",
		TraceID:      "trace-pay",
		IsError:      true,
		ErrorMessage: "card declined",
		ErrorHash1:   "pay-declined",
		Timestamp:    time.Now().Add(-5 * time.Minute),
	}
	failed.SetContext(&events.EventContext{Method: "POST", Path: "/pay", Status: 500, Error: "card declined"})
	if err := events.Create(db, failed); err != nil {
		t.Fatalf("Failed to create event: %v", err)
	}
	ok := &events.Event{
		APIKeyID:  key.ID,
		Message:   "GET /health 200",
		TraceID:   "trace-health",
		Timestamp: time.Now().Add(-5 * time.Minute),
	}
	ok.SetContext(&events.EventContext{Method: "GET", Path: "/health", Status: 200})
	if err := events.Create(db, ok); err != nil {
		t.Fatalf("Failed to create event: %v", err)
	}

	return key.ID
}

// call drives one JSON-RPC round trip the way an MCP client would.
func call(t *testing.T, db *gorm.DB, method, params string) map[string]any {
	t.Helper()

	res, answered := Handle(db, Request{
		JSONRPC: "2.0",
		ID:      json.RawMessage(`1`),
		Method:  method,
		Params:  json.RawMessage(params),
	})

	if !answered {
		t.Fatalf("%s got no response", method)
	}
	if res.Error != nil {
		t.Fatalf("%s failed: %s", method, res.Error.Message)
	}
	out, ok := res.Result.(map[string]any)
	if !ok {
		t.Fatalf("%s returned %T, want a result object", method, res.Result)
	}

	return out
}

func toolText(t *testing.T, result map[string]any) string {
	t.Helper()

	if isError, _ := result["isError"].(bool); isError {
		t.Fatalf("tool reported an error: %v", result["content"])
	}
	content, ok := result["content"].([]map[string]any)
	if !ok || len(content) == 0 {
		t.Fatalf("tool returned no content: %#v", result)
	}
	text, _ := content[0]["text"].(string)

	return text
}

func toolErrorText(t *testing.T, result map[string]any) string {
	t.Helper()

	if isError, _ := result["isError"].(bool); !isError {
		t.Fatalf("tool succeeded, want an error: %#v", result)
	}
	text, _ := result["content"].([]map[string]any)[0]["text"].(string)

	return text
}

func TestInitialize(t *testing.T) {
	t.Run("agrees on a protocol the client asked for", func(t *testing.T) {
		db := setupTestDB(t)

		result := call(t, db, "initialize", `{"protocolVersion":"2024-11-05"}`)

		if got := result["protocolVersion"]; got != "2024-11-05" {
			t.Errorf("protocolVersion = %v, want 2024-11-05", got)
		}
	})

	t.Run("falls back to the newest protocol for an unknown one", func(t *testing.T) {
		db := setupTestDB(t)

		result := call(t, db, "initialize", `{"protocolVersion":"1999-01-01"}`)

		if got := result["protocolVersion"]; got != SupportedProtocols[0] {
			t.Errorf("protocolVersion = %v, want %v", got, SupportedProtocols[0])
		}
	})
}

func TestNotificationsGetNoReply(t *testing.T) {
	db := setupTestDB(t)

	_, answered := Handle(db, Request{JSONRPC: "2.0", Method: "notifications/initialized"})

	if answered {
		t.Error("answered a notification, want silence")
	}
}

func TestToolsList(t *testing.T) {
	db := setupTestDB(t)

	result := call(t, db, "tools/list", `{}`)

	listed, ok := result["tools"].([]map[string]any)
	if !ok {
		t.Fatalf("tools = %T, want a list", result["tools"])
	}
	want := []string{"list_apps", "list_alerts", "list_endpoints", "endpoint_timeline", "uptime_timeline", "list_issues", "search_logs", "get_event"}
	if len(listed) != len(want) {
		t.Fatalf("listed %d tools, want %d", len(listed), len(want))
	}
	for i, name := range want {
		if listed[i]["name"] != name {
			t.Errorf("tool %d = %v, want %s", i, listed[i]["name"], name)
		}
	}
}

func TestListApps(t *testing.T) {
	db := setupTestDB(t)
	seedApp(t, db, "checkout-prod")

	result := call(t, db, "tools/call", `{"name":"list_apps","arguments":{}}`)

	if text := toolText(t, result); !strings.Contains(text, "checkout-prod") {
		t.Errorf("text = %q, want the app name", text)
	}
}

func TestSearchLogs(t *testing.T) {
	t.Run("finds a request by path", func(t *testing.T) {
		db := setupTestDB(t)
		seedApp(t, db, "checkout-prod")

		result := call(t, db, "tools/call", `{"name":"search_logs","arguments":{"search":"/pay"}}`)

		text := toolText(t, result)
		if !strings.Contains(text, "POST /pay") {
			t.Errorf("text = %q, want the failing request", text)
		}
		if strings.Contains(text, "/health") {
			t.Errorf("text = %q, want the search to exclude other requests", text)
		}
	})

	t.Run("errors_only hides the requests that worked", func(t *testing.T) {
		db := setupTestDB(t)
		seedApp(t, db, "checkout-prod")

		result := call(t, db, "tools/call", `{"name":"search_logs","arguments":{"errors_only":true}}`)

		text := toolText(t, result)
		if !strings.Contains(text, "/pay") {
			t.Errorf("text = %q, want the failed request", text)
		}
		if strings.Contains(text, "/health") {
			t.Errorf("text = %q, want no successful requests", text)
		}
	})

	t.Run("the default window is one hour", func(t *testing.T) {
		db := setupTestDB(t)
		key, _, _ := apikeys.Create(db, "checkout-prod")
		events.Create(db, &events.Event{
			APIKeyID:  key.ID,
			Message:   "GET /old 200",
			Timestamp: time.Now().Add(-3 * time.Hour),
		})

		result := call(t, db, "tools/call", `{"name":"search_logs","arguments":{}}`)

		if text := toolText(t, result); strings.Contains(text, "/old") {
			t.Errorf("text = %q, want events older than an hour left out", text)
		}
	})

	t.Run("since reaches further back", func(t *testing.T) {
		db := setupTestDB(t)
		key, _, _ := apikeys.Create(db, "checkout-prod")
		events.Create(db, &events.Event{
			APIKeyID:  key.ID,
			Message:   "GET /old 200",
			Timestamp: time.Now().Add(-3 * 24 * time.Hour),
		})

		result := call(t, db, "tools/call", `{"name":"search_logs","arguments":{"since":"7d"}}`)

		if text := toolText(t, result); !strings.Contains(text, "/old") {
			t.Errorf("text = %q, want the older event included", text)
		}
	})

	t.Run("app_id keeps other apps out", func(t *testing.T) {
		db := setupTestDB(t)
		wanted := seedApp(t, db, "checkout-prod")
		seedApp(t, db, "billing-prod")

		result := call(t, db, "tools/call", `{"name":"search_logs","arguments":{"app_id":`+itoa(wanted)+`}}`)

		var payload struct {
			Total int64 `json:"total"`
		}
		json.Unmarshal([]byte(toolText(t, result)), &payload)
		if payload.Total != 2 {
			t.Errorf("total = %d, want the 2 events of one app", payload.Total)
		}
	})

	t.Run("issue keeps only the events of that issue", func(t *testing.T) {
		db := setupTestDB(t)
		seedApp(t, db, "checkout-prod")

		result := call(t, db, "tools/call", `{"name":"search_logs","arguments":{"issue":"pay-declined"}}`)

		text := toolText(t, result)
		if !strings.Contains(text, "POST /pay") {
			t.Errorf("text = %q, want the event of the issue", text)
		}
		if strings.Contains(text, "/health") {
			t.Errorf("text = %q, want other events left out", text)
		}
	})

	t.Run("until ends the window", func(t *testing.T) {
		db := setupTestDB(t)
		key, _, _ := apikeys.Create(db, "checkout-prod")
		events.Create(db, &events.Event{
			APIKeyID:  key.ID,
			Message:   "GET /old 200",
			Timestamp: time.Now().Add(-3 * time.Hour),
		})

		before := call(t, db, "tools/call", `{"name":"search_logs","arguments":{"since":"1d","until":"4h"}}`)
		after := call(t, db, "tools/call", `{"name":"search_logs","arguments":{"since":"1d","until":"2h"}}`)

		if text := toolText(t, before); strings.Contains(text, "/old") {
			t.Errorf("text = %q, want the event after until left out", text)
		}
		if text := toolText(t, after); !strings.Contains(text, "/old") {
			t.Errorf("text = %q, want the event inside the window", text)
		}
	})

	t.Run("since takes an RFC 3339 time", func(t *testing.T) {
		db := setupTestDB(t)
		key, _, _ := apikeys.Create(db, "checkout-prod")
		events.Create(db, &events.Event{
			APIKeyID:  key.ID,
			Message:   "GET /old 200",
			Timestamp: time.Now().Add(-3 * time.Hour),
		})
		since := time.Now().Add(-4 * time.Hour).UTC().Format(time.RFC3339)

		result := call(t, db, "tools/call", `{"name":"search_logs","arguments":{"since":"`+since+`"}}`)

		if text := toolText(t, result); !strings.Contains(text, "/old") {
			t.Errorf("text = %q, want the event after the given time", text)
		}
	})

	t.Run("after_id returns the newer events oldest first", func(t *testing.T) {
		db := setupTestDB(t)
		appID := seedApp(t, db, "checkout-prod")
		var newest events.Event
		db.Order("id DESC").First(&newest)
		for _, msg := range []string{"GET /a 200", "GET /b 200"} {
			e := &events.Event{APIKeyID: appID, Message: msg, Timestamp: time.Now()}
			if err := events.Create(db, e); err != nil {
				t.Fatalf("Failed to create event: %v", err)
			}
		}

		result := call(t, db, "tools/call", `{"name":"search_logs","arguments":{"after_id":`+itoa(newest.ID)+`}}`)

		var payload struct {
			Events []events.Event `json:"events"`
		}
		json.Unmarshal([]byte(toolText(t, result)), &payload)
		if len(payload.Events) != 2 || payload.Events[0].Message != "GET /a 200" || payload.Events[1].Message != "GET /b 200" {
			t.Errorf("events = %+v, want /a then /b", payload.Events)
		}
	})

	t.Run("after_id finds an event whose timestamp arrived late", func(t *testing.T) {
		db := setupTestDB(t)
		appID := seedApp(t, db, "checkout-prod")
		var newest events.Event
		db.Order("id DESC").First(&newest)
		late := &events.Event{APIKeyID: appID, Message: "GET /late 200", Timestamp: time.Now().Add(-3 * time.Hour)}
		if err := events.Create(db, late); err != nil {
			t.Fatalf("Failed to create event: %v", err)
		}

		result := call(t, db, "tools/call", `{"name":"search_logs","arguments":{"after_id":`+itoa(newest.ID)+`}}`)

		if text := toolText(t, result); !strings.Contains(text, "GET /late 200") {
			t.Errorf("text = %q, want the late event", text)
		}
	})

	t.Run("rejects a since it cannot read", func(t *testing.T) {
		db := setupTestDB(t)

		result := call(t, db, "tools/call", `{"name":"search_logs","arguments":{"since":"last tuesday"}}`)

		if text := toolErrorText(t, result); !strings.Contains(text, "15m") {
			t.Errorf("text = %q, want an example of a readable since", text)
		}
	})
}

func TestGetEvent(t *testing.T) {
	t.Run("returns the event and its trace", func(t *testing.T) {
		db := setupTestDB(t)
		seedApp(t, db, "checkout-prod")

		found := call(t, db, "tools/call", `{"name":"search_logs","arguments":{"errors_only":true}}`)
		var search struct {
			Events []struct{ ID uint } `json:"events"`
		}
		json.Unmarshal([]byte(toolText(t, found)), &search)
		if len(search.Events) == 0 {
			t.Fatal("search returned no events to fetch")
		}
		result := call(t, db, "tools/call", `{"name":"get_event","arguments":{"id":`+itoa(search.Events[0].ID)+`}}`)

		text := toolText(t, result)
		if !strings.Contains(text, "POST /pay") {
			t.Errorf("text = %q, want the event message", text)
		}
		if !strings.Contains(text, "relatedEvents") {
			t.Errorf("text = %q, want the trace siblings", text)
		}
	})

	t.Run("accepts an id the client quoted", func(t *testing.T) {
		db := setupTestDB(t)
		seedApp(t, db, "checkout-prod")

		result := call(t, db, "tools/call", `{"name":"get_event","arguments":{"id":"1"}}`)

		if text := toolText(t, result); !strings.Contains(text, "message") {
			t.Errorf("text = %q, want the event", text)
		}
	})

	t.Run("asks for an id when it is missing", func(t *testing.T) {
		db := setupTestDB(t)

		result := call(t, db, "tools/call", `{"name":"get_event","arguments":{}}`)

		if text := toolErrorText(t, result); !strings.Contains(text, "id is required") {
			t.Errorf("text = %q, want it to ask for an id", text)
		}
	})

	t.Run("says so when the event is gone", func(t *testing.T) {
		db := setupTestDB(t)

		result := call(t, db, "tools/call", `{"name":"get_event","arguments":{"id":9999}}`)

		if text := toolErrorText(t, result); !strings.Contains(text, "9999") {
			t.Errorf("text = %q, want the id in the message", text)
		}
	})
}

func TestListIssues(t *testing.T) {
	db := setupTestDB(t)
	seedApp(t, db, "checkout-prod")

	result := call(t, db, "tools/call", `{"name":"list_issues","arguments":{}}`)

	if text := toolText(t, result); !strings.Contains(text, "issues") {
		t.Errorf("text = %q, want an issues list", text)
	}
}

func TestListAlerts(t *testing.T) {
	t.Run("returns the endpoints alerting now", func(t *testing.T) {
		db := setupTestDB(t)
		appID := seedApp(t, db, "checkout-prod")
		db.Create(&alerts.SpikeState{APIKeyID: appID, Path: "/pay", Severity: "critical", SpikeStartAt: time.Now(), ErrorCount: 42})
		db.Create(&alerts.SpikeState{APIKeyID: appID, Path: "/health"})

		result := call(t, db, "tools/call", `{"name":"list_alerts","arguments":{}}`)

		var payload struct {
			Alerts []alerts.Alert `json:"alerts"`
		}
		json.Unmarshal([]byte(toolText(t, result)), &payload)
		if len(payload.Alerts) != 1 {
			t.Fatalf("alerts = %+v, want only the alerting endpoint", payload.Alerts)
		}
		alert := payload.Alerts[0]
		if alert.Path != "/pay" || alert.Kind != "error_rate" || alert.Severity != "critical" {
			t.Errorf("alert = %+v, want a critical error rate alert on /pay", alert)
		}
		if !strings.Contains(alert.Summary, "42 errors") {
			t.Errorf("summary = %q, want the error count", alert.Summary)
		}
	})

	t.Run("returns apps that stopped answering their ping", func(t *testing.T) {
		db := setupTestDB(t)
		appID := seedApp(t, db, "checkout-prod")
		db.Model(&apikeys.APIKey{}).Where("id = ?", appID).
			Updates(map[string]any{"ping_url": "https://shop.example.com/up", "is_down": true, "down_since": time.Now()})

		result := call(t, db, "tools/call", `{"name":"list_alerts","arguments":{}}`)

		if text := toolText(t, result); !strings.Contains(text, "https://shop.example.com/up") {
			t.Errorf("text = %q, want the app that is down", text)
		}
	})
}

func TestEndpointTimeline(t *testing.T) {
	record := func(db *gorm.DB, appID uint, path string, at time.Time, total, errors int64) {
		db.Create(&events.EndpointStat{APIKeyID: appID, Path: path, Bucket: at, Total: total, Errors: errors})
	}

	t.Run("returns the traffic in 5-minute steps", func(t *testing.T) {
		db := setupTestDB(t)
		appID := seedApp(t, db, "checkout-prod")
		at := events.LastCompleteBucketTime()
		record(db, appID, "/pay", at, 100, 5)

		result := call(t, db, "tools/call", `{"name":"endpoint_timeline","arguments":{"path":"/pay","since":"1h"}}`)

		var payload struct {
			StepMinutes int                    `json:"step_minutes"`
			Points      []events.TimelinePoint `json:"points"`
		}
		json.Unmarshal([]byte(toolText(t, result)), &payload)
		if payload.StepMinutes != 5 {
			t.Errorf("step = %d, want 5 minutes", payload.StepMinutes)
		}
		var found bool
		for _, p := range payload.Points {
			if p.At.Equal(at) {
				found = p.Requests == 100 && p.Errors == 5 && p.ErrorRate == 5
			}
		}
		if !found {
			t.Errorf("points = %+v, want 100 requests and 5 errors at %s", payload.Points, at)
		}
		if len(payload.Points) < 12 {
			t.Errorf("got %d points, want the empty steps too", len(payload.Points))
		}
	})

	t.Run("uses 1-hour steps beyond a day", func(t *testing.T) {
		db := setupTestDB(t)
		appID := seedApp(t, db, "checkout-prod")
		record(db, appID, "/pay", events.LastCompleteBucketTime(), 100, 5)

		result := call(t, db, "tools/call", `{"name":"endpoint_timeline","arguments":{"path":"/pay","since":"3d"}}`)

		if text := toolText(t, result); !strings.Contains(text, `"step_minutes":60`) {
			t.Errorf("text = %q, want hourly steps", text)
		}
	})

	t.Run("asks for app_id when two apps share the path", func(t *testing.T) {
		db := setupTestDB(t)
		record(db, seedApp(t, db, "checkout-prod"), "/pay", events.LastCompleteBucketTime(), 100, 5)
		record(db, seedApp(t, db, "checkout-staging"), "/pay", events.LastCompleteBucketTime(), 10, 0)

		result := call(t, db, "tools/call", `{"name":"endpoint_timeline","arguments":{"path":"/pay"}}`)

		if text := toolErrorText(t, result); !strings.Contains(text, "app_id") {
			t.Errorf("text = %q, want a request for app_id", text)
		}
	})

	t.Run("requires a path", func(t *testing.T) {
		db := setupTestDB(t)

		result := call(t, db, "tools/call", `{"name":"endpoint_timeline","arguments":{}}`)

		if text := toolErrorText(t, result); !strings.Contains(text, "path") {
			t.Errorf("text = %q, want the missing path named", text)
		}
	})
}

func TestListEndpoints(t *testing.T) {
	record := func(db *gorm.DB, appID uint, path string, at time.Time, total, errors int64) {
		db.Create(&events.EndpointStat{APIKeyID: appID, Path: path, Bucket: at, Total: total, Errors: errors, AvgDuration: 40})
	}

	t.Run("lists the last hour, most broken first", func(t *testing.T) {
		db := setupTestDB(t)
		appID := seedApp(t, db, "checkout-prod")
		at := events.LastCompleteBucketTime()
		record(db, appID, "/health", at, 200, 0)
		record(db, appID, "/pay", at, 100, 20)
		record(db, appID, "/old", at.Add(-3*time.Hour), 50, 50)

		result := call(t, db, "tools/call", `{"name":"list_endpoints","arguments":{}}`)

		var payload struct {
			Endpoints []events.EndpointStats `json:"endpoints"`
		}
		json.Unmarshal([]byte(toolText(t, result)), &payload)
		if len(payload.Endpoints) != 2 {
			t.Fatalf("endpoints = %+v, want /pay and /health from the last hour", payload.Endpoints)
		}
		if first := payload.Endpoints[0]; first.Path != "/pay" || first.Total != 100 || first.ErrorRate != 20 {
			t.Errorf("first = %+v, want /pay at 20%%", first)
		}
	})

	t.Run("since reaches further back", func(t *testing.T) {
		db := setupTestDB(t)
		appID := seedApp(t, db, "checkout-prod")
		record(db, appID, "/old", events.LastCompleteBucketTime().Add(-3*time.Hour), 50, 50)

		result := call(t, db, "tools/call", `{"name":"list_endpoints","arguments":{"since":"6h"}}`)

		if text := toolText(t, result); !strings.Contains(text, "/old") {
			t.Errorf("text = %q, want the endpoint from 3 hours ago", text)
		}
	})

	t.Run("asks for app_id when there are two apps", func(t *testing.T) {
		db := setupTestDB(t)
		seedApp(t, db, "checkout-prod")
		seedApp(t, db, "checkout-staging")

		result := call(t, db, "tools/call", `{"name":"list_endpoints","arguments":{}}`)

		if text := toolErrorText(t, result); !strings.Contains(text, "pass app_id") {
			t.Errorf("error = %q, want a request for app_id", text)
		}
	})
}

func TestUptimeTimeline(t *testing.T) {
	t.Run("returns the latest ping and why pings failed", func(t *testing.T) {
		db := setupTestDB(t)
		appID := seedApp(t, db, "checkout-prod")
		db.Model(&apikeys.APIKey{}).Where("id = ?", appID).Update("ping_url", "https://shop.example.com/up")
		now := time.Now()
		uptime.Record(db, &uptime.Check{APIKeyID: appID, CheckedAt: now.Add(-10 * time.Minute), Status: 503, LatencyMS: 900})
		uptime.Record(db, &uptime.Check{APIKeyID: appID, CheckedAt: now.Add(-time.Minute), Up: true, Status: 200, LatencyMS: 84})

		result := call(t, db, "tools/call", `{"name":"uptime_timeline","arguments":{}}`)

		var report uptime.Report
		json.Unmarshal([]byte(toolText(t, result)), &report)
		if report.Status != "up" || report.LatencyMS != 84 {
			t.Errorf("state = %s %dms, want up 84ms", report.Status, report.LatencyMS)
		}
		if len(report.Bars) != 96 {
			t.Errorf("got %d bars, want 96", len(report.Bars))
		}
		if report.LastFailure == nil || report.LastFailure.Status != 503 {
			t.Errorf("last failure = %+v, want the 503", report.LastFailure)
		}
	})

	t.Run("lists outages with the issues that surged in them", func(t *testing.T) {
		db := setupTestDB(t)
		appID := seedApp(t, db, "checkout-prod")
		start := time.Now().Add(-time.Hour).Truncate(time.Minute)
		for i := range 4 {
			uptime.Record(db, &uptime.Check{APIKeyID: appID, CheckedAt: start.Add(time.Duration(i) * time.Minute), Status: 503})
		}
		uptime.Record(db, &uptime.Check{APIKeyID: appID, CheckedAt: start.Add(4 * time.Minute), Up: true, Status: 200})
		for i := range 3 {
			ctx := events.EventContext{Error: "redis refused", ErrorFile: "store.rb", ErrorLine: 18}
			event := &events.Event{APIKeyID: appID, Timestamp: start.Add(time.Duration(i) * time.Minute), IsError: true, Message: "redis refused"}
			event.SetContext(&ctx)
			events.PopulateErrorFields(event, &ctx)
			events.Create(db, event)
		}

		result := call(t, db, "tools/call", `{"name":"uptime_timeline","arguments":{}}`)

		var report struct {
			Outages []uptime.OutageSurges `json:"outages"`
		}
		json.Unmarshal([]byte(toolText(t, result)), &report)
		if len(report.Outages) != 1 || len(report.Outages[0].Issues) != 1 || report.Outages[0].Issues[0].Error != "redis refused" {
			t.Errorf("outages = %+v, want one with the redis issue", report.Outages)
		}
	})

	t.Run("says so when the app does not exist", func(t *testing.T) {
		db := setupTestDB(t)

		result := call(t, db, "tools/call", `{"name":"uptime_timeline","arguments":{"app_id":42}}`)

		if text := toolErrorText(t, result); !strings.Contains(text, "no app with id 42") {
			t.Errorf("error = %q, want the missing app", text)
		}
	})
}

func TestIssueCausesAndSteps(t *testing.T) {
	fail := func(t *testing.T, db *gorm.DB, appID uint, class, trace, user string, at time.Time) {
		t.Helper()
		ctx := events.EventContext{ErrorClass: class, Error: class, ErrorFile: "app.rb", ErrorLine: 1, User: user}
		event := &events.Event{APIKeyID: appID, Timestamp: at, IsError: true, Message: class, TraceID: trace}
		event.SetContext(&ctx)
		events.PopulateErrorFields(event, &ctx)
		events.Create(db, event)
	}

	t.Run("list_issues names the issue that failed first in a symptom's requests", func(t *testing.T) {
		db := setupTestDB(t)
		appID := seedApp(t, db, "checkout-prod")
		db.Create(&events.EndpointStat{APIKeyID: appID, Path: "/", Bucket: time.Now().Add(-time.Hour), Total: 1000})
		for i := range 5 {
			at := time.Now().Add(-time.Duration(i+1) * time.Hour)
			fail(t, db, appID, "Redis::TimeoutError", fmt.Sprint("t", i), "", at)
			fail(t, db, appID, "NoMethodError", fmt.Sprint("t", i), "", at.Add(time.Millisecond))
		}

		result := call(t, db, "tools/call", `{"name":"list_issues","arguments":{}}`)

		var out struct {
			Issues []struct {
				ErrorClass string `json:"error_class"`
				After      []struct {
					ErrorClass string `json:"error_class"`
					Traces     int    `json:"traces"`
				} `json:"after"`
			} `json:"issues"`
		}
		json.Unmarshal([]byte(toolText(t, result)), &out)
		for _, issue := range out.Issues {
			if issue.ErrorClass == "NoMethodError" && (len(issue.After) != 1 || issue.After[0].ErrorClass != "Redis::TimeoutError" || issue.After[0].Traces != 5) {
				t.Errorf("after = %+v, want Redis in 5 requests", issue.After)
			}
			if issue.ErrorClass == "Redis::TimeoutError" && len(issue.After) != 0 {
				t.Errorf("Redis has after = %+v, want none", issue.After)
			}
		}
	})

	t.Run("get_event lists the user's steps before the error", func(t *testing.T) {
		db := setupTestDB(t)
		appID := seedApp(t, db, "checkout-prod")
		now := time.Now()
		ctx := events.EventContext{Method: "GET", Path: "/cart", Status: 200, User: "42"}
		step := &events.Event{APIKeyID: appID, Timestamp: now.Add(-time.Minute), Message: "GET /cart"}
		step.SetContext(&ctx)
		events.Create(db, step)
		fail(t, db, appID, "NoMethodError", "t1", "42", now)
		var errorID uint
		db.Model(&events.Event{}).Where("is_error = 1").Select("id").Scan(&errorID)

		result := call(t, db, "tools/call", fmt.Sprintf(`{"name":"get_event","arguments":{"id":%d}}`, errorID))

		var out struct {
			StepsBefore []struct {
				Message string `json:"message"`
			} `json:"stepsBefore"`
		}
		json.Unmarshal([]byte(toolText(t, result)), &out)
		if len(out.StepsBefore) != 1 || out.StepsBefore[0].Message != "GET /cart" {
			t.Errorf("stepsBefore = %+v, want GET /cart", out.StepsBefore)
		}
	})
}

func TestUnknownToolAndMethod(t *testing.T) {
	t.Run("names the tool it does not have", func(t *testing.T) {
		db := setupTestDB(t)

		result := call(t, db, "tools/call", `{"name":"delete_everything","arguments":{}}`)

		if text := toolErrorText(t, result); !strings.Contains(text, "delete_everything") {
			t.Errorf("text = %q, want the tool name in the message", text)
		}
	})

	t.Run("returns a JSON-RPC error for an unknown method", func(t *testing.T) {
		db := setupTestDB(t)

		res, _ := Handle(db, Request{JSONRPC: "2.0", ID: json.RawMessage(`1`), Method: "resources/list"})

		if res.Error == nil || res.Error.Code != -32601 {
			t.Errorf("error = %#v, want code -32601", res.Error)
		}
	})
}

func TestClampLimit(t *testing.T) {
	cases := []struct {
		asked int
		want  int
	}{
		{asked: 0, want: 20},
		{asked: -5, want: 20},
		{asked: 50, want: 50},
		{asked: 50000, want: 200},
	}

	for _, c := range cases {
		if got := clampLimit(c.asked); got != c.want {
			t.Errorf("clampLimit(%d) = %d, want %d", c.asked, got, c.want)
		}
	}
}

func itoa(n uint) string {
	return strconv.FormatUint(uint64(n), 10)
}
