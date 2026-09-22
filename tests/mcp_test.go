package tests

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/karloscodes/cartridge"
	"github.com/karloscodes/cartridge/sqlite"

	"lognorth/internal/apikeys"
	"lognorth/internal/database"
	"lognorth/internal/events"
	lbhttp "lognorth/internal/http"
	"lognorth/internal/jobs"
	"lognorth/internal/license"
	"lognorth/internal/settings"
)

// =============================================================================
// MCP INTEGRATION TESTS
// These mount the real handlers on a fiber app and drive them over HTTP, so
// the auth check, the JSON-RPC envelope, and the database reads are all the
// production code paths.
// =============================================================================

type mcpTestApp struct {
	app      *fiber.App
	db       *sqlite.Manager
	agentKey string
	appKey   string
	apiKeyID uint
}

func setupMCPTestApp(t *testing.T) *mcpTestApp {
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

	agentKey, err := settings.CreateAgentKey(db)
	if err != nil {
		t.Fatalf("Failed to create agent key: %v", err)
	}
	key, appKey, err := apikeys.Create(db, "checkout-prod")
	if err != nil {
		t.Fatalf("Failed to create app key: %v", err)
	}

	// Mount the real handlers. The wrapper is what cartridge's router does.
	app := fiber.New()
	handle := func(h func(*cartridge.Context) error) fiber.Handler {
		return func(c *fiber.Ctx) error {
			return h(&cartridge.Context{Ctx: c, DBManager: manager})
		}
	}
	app.Post("/mcp", handle(lbhttp.MCPAction))
	app.Post("/api/v1/events", handle(lbhttp.IngestAction))

	return &mcpTestApp{app: app, db: manager, agentKey: agentKey, appKey: appKey, apiKeyID: key.ID}
}

// rpc posts one JSON-RPC request to /mcp with the given bearer token.
func (ta *mcpTestApp) rpc(t *testing.T, token, body string) (int, string) {
	t.Helper()

	req := httptest.NewRequest("POST", "/mcp", bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	res, err := ta.app.Test(req)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	defer res.Body.Close()
	payload, _ := io.ReadAll(res.Body)

	return res.StatusCode, string(payload)
}

// callTool posts a tools/call and returns the decoded tool payload.
func (ta *mcpTestApp) callTool(t *testing.T, name, args string) map[string]any {
	t.Helper()

	status, body := ta.rpc(t, ta.agentKey,
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"`+name+`","arguments":`+args+`}}`)
	if status != 200 {
		t.Fatalf("%s returned %d: %s", name, status, body)
	}

	var res struct {
		Result struct {
			IsError bool `json:"isError"`
			Content []struct {
				Text string `json:"text"`
			} `json:"content"`
		} `json:"result"`
	}
	if err := json.Unmarshal([]byte(body), &res); err != nil {
		t.Fatalf("%s returned unreadable JSON: %v", name, err)
	}
	if res.Result.IsError {
		t.Fatalf("%s reported an error: %s", name, res.Result.Content[0].Text)
	}
	if len(res.Result.Content) == 0 {
		t.Fatalf("%s returned no content: %s", name, body)
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(res.Result.Content[0].Text), &payload); err != nil {
		t.Fatalf("%s returned unreadable tool text: %v", name, err)
	}

	return payload
}

// ingest posts an event through the real ingest handler.
func (ta *mcpTestApp) ingest(t *testing.T, body string) {
	t.Helper()

	req := httptest.NewRequest("POST", "/api/v1/events", bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+ta.appKey)

	res, err := ta.app.Test(req)
	if err != nil {
		t.Fatalf("Ingest failed: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != 201 {
		payload, _ := io.ReadAll(res.Body)
		t.Fatalf("Ingest returned %d: %s", res.StatusCode, payload)
	}
}

// process runs the event processor, the job that moves ingested events into
// the events table the MCP tools read.
func (ta *mcpTestApp) process(t *testing.T) {
	t.Helper()

	license.SetConfigured(true)
	t.Cleanup(func() { license.SetConfigured(false) })

	ctx := &cartridge.JobContext{
		Context: t.Context(),
		Logger:  slog.New(slog.NewTextHandler(io.Discard, nil)),
		DB:      ta.db.GetConnection(),
	}
	if err := jobs.NewEventProcessor().ProcessBatch(ctx); err != nil {
		t.Fatalf("Failed to process events: %v", err)
	}
}

func TestMCPRejectsEveryKeyButTheAgentKey(t *testing.T) {
	initialize := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18"}}`

	t.Run("with no Authorization header", func(t *testing.T) {
		ta := setupMCPTestApp(t)

		status, body := ta.rpc(t, "", initialize)

		if status != 401 {
			t.Errorf("status = %d, want 401", status)
		}
		if !bytes.Contains([]byte(body), []byte("API key required")) {
			t.Errorf("body = %s, want the API key error", body)
		}
		// The bug this guards: the 401 used to ship the real result as well.
		if bytes.Contains([]byte(body), []byte("serverInfo")) {
			t.Errorf("body = %s, want no protocol result under a 401", body)
		}
	})

	t.Run("with a wrong key", func(t *testing.T) {
		ta := setupMCPTestApp(t)

		status, body := ta.rpc(t, "lgn-agent-nope", initialize)

		if status != 401 {
			t.Errorf("status = %d, want 401", status)
		}
		if bytes.Contains([]byte(body), []byte("serverInfo")) {
			t.Errorf("body = %s, want no protocol result under a 401", body)
		}
	})

	t.Run("with an app key instead of an agent key", func(t *testing.T) {
		ta := setupMCPTestApp(t)

		status, body := ta.rpc(t, ta.appKey, initialize)

		if status != 401 {
			t.Errorf("status = %d, want 401: an ingest key must not open MCP", status)
		}
		if bytes.Contains([]byte(body), []byte("serverInfo")) {
			t.Errorf("body = %s, want no protocol result under a 401", body)
		}
	})
}

func TestMCPHandshake(t *testing.T) {
	ta := setupMCPTestApp(t)

	status, body := ta.rpc(t, ta.agentKey,
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18"}}`)

	if status != 200 {
		t.Fatalf("status = %d: %s", status, body)
	}
	var res struct {
		JSONRPC string `json:"jsonrpc"`
		Result  struct {
			ProtocolVersion string `json:"protocolVersion"`
			ServerInfo      struct {
				Name string `json:"name"`
			} `json:"serverInfo"`
		} `json:"result"`
	}
	json.Unmarshal([]byte(body), &res)
	if res.JSONRPC != "2.0" {
		t.Errorf("jsonrpc = %q, want 2.0", res.JSONRPC)
	}
	if res.Result.ProtocolVersion != "2025-06-18" {
		t.Errorf("protocolVersion = %q, want the version the client asked for", res.Result.ProtocolVersion)
	}
	if res.Result.ServerInfo.Name != "lognorth" {
		t.Errorf("serverInfo.name = %q, want lognorth", res.Result.ServerInfo.Name)
	}
}

func TestMCPToolsList(t *testing.T) {
	ta := setupMCPTestApp(t)

	status, body := ta.rpc(t, ta.agentKey, `{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}`)

	if status != 200 {
		t.Fatalf("status = %d: %s", status, body)
	}
	var res struct {
		Result struct {
			Tools []struct {
				Name        string         `json:"name"`
				InputSchema map[string]any `json:"inputSchema"`
			} `json:"tools"`
		} `json:"result"`
	}
	json.Unmarshal([]byte(body), &res)
	want := []string{"list_apps", "list_alerts", "endpoint_timeline", "list_issues", "search_logs", "get_event"}
	if len(res.Result.Tools) != len(want) {
		t.Fatalf("listed %d tools, want %d", len(res.Result.Tools), len(want))
	}
	for i, name := range want {
		if res.Result.Tools[i].Name != name {
			t.Errorf("tool %d = %q, want %q", i, res.Result.Tools[i].Name, name)
		}
		if res.Result.Tools[i].InputSchema == nil {
			t.Errorf("tool %q has no inputSchema, so a client cannot call it", name)
		}
	}
}

func TestMCPNotificationIsAcknowledgedWithNoBody(t *testing.T) {
	ta := setupMCPTestApp(t)

	status, body := ta.rpc(t, ta.agentKey, `{"jsonrpc":"2.0","method":"notifications/initialized"}`)

	if status != 202 {
		t.Errorf("status = %d, want 202", status)
	}
	if body != "" {
		t.Errorf("body = %q, want it empty", body)
	}
}

func TestMCPMalformedJSON(t *testing.T) {
	ta := setupMCPTestApp(t)

	status, body := ta.rpc(t, ta.agentKey, `{"jsonrpc":"2.0", oops`)

	if status != 400 {
		t.Errorf("status = %d, want 400", status)
	}
	if !bytes.Contains([]byte(body), []byte("-32700")) {
		t.Errorf("body = %s, want the JSON-RPC parse error code", body)
	}
}

// The whole path: an SDK posts events, the processor moves them, and an agent
// reads them back over MCP.
func TestMCPReadsIngestedEvents(t *testing.T) {
	ta := setupMCPTestApp(t)

	for i := range 3 {
		ta.ingest(t, `{
			"message": "POST /api/checkout 500",
			"trace_id": "trace-checkout-`+strconv.Itoa(i+1)+`",
			"duration_ms": 121,
			"context": {"method":"POST","path":"/api/checkout","status":500,"error":"Stripe timeout after 30s"}
		}`)
	}
	ta.ingest(t, `{
		"message": "GET /api/health 200",
		"trace_id": "trace-checkout-1",
		"duration_ms": 3,
		"context": {"method":"GET","path":"/api/health","status":200}
	}`)
	ta.process(t)

	t.Run("list_apps names the app that sent them", func(t *testing.T) {
		payload := ta.callTool(t, "list_apps", `{}`)

		apps, _ := payload["apps"].([]any)
		if len(apps) != 1 {
			t.Fatalf("apps = %v, want the one app", apps)
		}
		if name := apps[0].(map[string]any)["name"]; name != "checkout-prod" {
			t.Errorf("name = %v, want checkout-prod", name)
		}
	})

	t.Run("search_logs returns only the failures", func(t *testing.T) {
		payload := ta.callTool(t, "search_logs", `{"errors_only":true}`)

		if total, _ := payload["total"].(float64); total != 3 {
			t.Errorf("total = %v, want the 3 failed checkouts", payload["total"])
		}
		found, _ := payload["events"].([]any)
		for _, e := range found {
			event := e.(map[string]any)
			if event["message"] != "POST /api/checkout 500" {
				t.Errorf("message = %v, want only checkout failures", event["message"])
			}
			if event["error_message"] != "Stripe timeout after 30s" {
				t.Errorf("error_message = %v, want the processor to have extracted it", event["error_message"])
			}
		}
	})

	t.Run("search_logs matches on text", func(t *testing.T) {
		payload := ta.callTool(t, "search_logs", `{"search":"Stripe"}`)

		if total, _ := payload["total"].(float64); total != 3 {
			t.Errorf("total = %v, want the 3 events mentioning Stripe", payload["total"])
		}
	})

	t.Run("search_logs honours the time window", func(t *testing.T) {
		ta.ingest(t, `{
			"message": "GET /api/old 200",
			"timestamp": "`+time.Now().Add(-72*time.Hour).UTC().Format(time.RFC3339)+`",
			"context": {"method":"GET","path":"/api/old","status":200}
		}`)
		ta.process(t)

		recent := ta.callTool(t, "search_logs", `{"search":"/api/old"}`)
		week := ta.callTool(t, "search_logs", `{"search":"/api/old","since":"7d"}`)

		if total, _ := recent["total"].(float64); total != 0 {
			t.Errorf("default window total = %v, want the 3 day old event left out", recent["total"])
		}
		if total, _ := week["total"].(float64); total != 1 {
			t.Errorf("since=7d total = %v, want the old event included", week["total"])
		}
	})

	t.Run("get_event returns the context and the trace", func(t *testing.T) {
		found := ta.callTool(t, "search_logs", `{"errors_only":true}`)
		id := float64(0)
		for _, e := range found["events"].([]any) {
			if event := e.(map[string]any); event["trace_id"] == "trace-checkout-1" {
				id = event["id"].(float64)
			}
		}
		if id == 0 {
			t.Fatal("search_logs did not return the failure on trace-checkout-1")
		}

		payload := ta.callTool(t, "get_event", `{"id":`+strconv.Itoa(int(id))+`}`)

		context, _ := payload["eventContext"].(map[string]any)
		if context["path"] != "/api/checkout" {
			t.Errorf("context path = %v, want /api/checkout", context["path"])
		}
		if context["error"] != "Stripe timeout after 30s" {
			t.Errorf("context error = %v, want the Stripe timeout", context["error"])
		}
		related, _ := payload["relatedEvents"].([]any)
		if len(related) != 2 {
			t.Errorf("relatedEvents = %d, want the checkout failure and the health check sharing trace-checkout-1", len(related))
		}
	})

	t.Run("list_issues groups the failures", func(t *testing.T) {
		payload := ta.callTool(t, "list_issues", `{}`)

		issues, _ := payload["issues"].([]any)
		if len(issues) != 1 {
			t.Fatalf("issues = %d, want the 3 failures grouped into 1", len(issues))
		}
		issue := issues[0].(map[string]any)
		if count, _ := issue["count"].(float64); count != 3 {
			t.Errorf("count = %v, want 3", issue["count"])
		}
	})

	t.Run("app_id keeps other apps out", func(t *testing.T) {
		payload := ta.callTool(t, "search_logs", `{"app_id":999,"errors_only":true}`)

		if total, _ := payload["total"].(float64); total != 0 {
			t.Errorf("total = %v, want nothing for an app that does not exist", payload["total"])
		}
	})
}

func TestMCPExposesNoWriteTool(t *testing.T) {
	ta := setupMCPTestApp(t)
	ta.ingest(t, `{"message":"GET /api/health 200","context":{"method":"GET","path":"/api/health","status":200}}`)
	ta.process(t)

	_, body := ta.rpc(t, ta.agentKey,
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"delete_event","arguments":{"id":1}}}`)

	if !bytes.Contains([]byte(body), []byte("unknown tool")) {
		t.Errorf("body = %s, want an unknown tool error", body)
	}
	var count int64
	ta.db.GetConnection().Model(&events.Event{}).Count(&count)
	if count != 1 {
		t.Errorf("events = %d after the call, want the event untouched", count)
	}
}
