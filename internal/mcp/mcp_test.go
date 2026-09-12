package mcp

import (
	"encoding/json"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/karloscodes/cartridge/sqlite"
	"gorm.io/gorm"

	"lognorth/internal/apikeys"
	"lognorth/internal/events"
)

func setupTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	dbPath := "test_" + strings.ReplaceAll(t.Name(), "/", "_") + ".db"
	t.Cleanup(func() {
		os.Remove(dbPath)
		os.Remove(dbPath + "-wal")
		os.Remove(dbPath + "-shm")
	})

	manager := sqlite.NewManager(sqlite.Config{Path: dbPath, EnableWAL: true})
	db, err := manager.Connect()
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	if err := apikeys.AutoMigrate(db); err != nil {
		t.Fatalf("Failed to migrate api keys: %v", err)
	}
	if err := events.AutoMigrate(db); err != nil {
		t.Fatalf("Failed to migrate events: %v", err)
	}
	if err := events.AutoMigrateIssues(db); err != nil {
		t.Fatalf("Failed to migrate issues: %v", err)
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
	want := []string{"list_apps", "search_logs", "get_event", "list_issues"}
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
