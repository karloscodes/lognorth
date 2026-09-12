// Package mcp answers Model Context Protocol calls, so a coding agent can read
// production logs while it debugs.
//
// The transport is Streamable HTTP: one JSON-RPC request per POST, one JSON
// response back. It lives in the main server process and reads the same
// contexts the web UI reads. Every tool is read-only. An agent can look,
// never touch.
package mcp

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"

	"lognorth/internal/apikeys"
	"lognorth/internal/events"
)

// Protocol revisions we understand. We answer with the client's revision when
// we know it, otherwise with the newest one we support.
var SupportedProtocols = []string{"2025-06-18", "2025-03-26", "2024-11-05"}

// Version is the server version reported to clients. The binary sets it.
var Version = "dev"

type Request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
}

type Response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result,omitempty"`
	Error   *Error          `json:"error,omitempty"`
}

type Error struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// Handle answers one JSON-RPC request. The second return is false for a
// notification, which the spec says to acknowledge without a body.
func Handle(db *gorm.DB, req Request) (*Response, bool) {
	if req.ID == nil {
		return nil, false
	}

	res := &Response{JSONRPC: "2.0", ID: req.ID}

	switch req.Method {
	case "initialize":
		res.Result = initialize(req.Params)
	case "tools/list":
		res.Result = map[string]any{"tools": Tools}
	case "tools/call":
		res.Result = callTool(db, req.Params)
	case "ping":
		res.Result = map[string]any{}
	default:
		res.Error = &Error{Code: -32601, Message: "unknown method: " + req.Method}
	}

	return res, true
}

func initialize(params json.RawMessage) map[string]any {
	var p struct {
		ProtocolVersion string `json:"protocolVersion"`
	}
	json.Unmarshal(params, &p)

	protocol := SupportedProtocols[0]
	for _, v := range SupportedProtocols {
		if p.ProtocolVersion == v {
			protocol = v
			break
		}
	}

	return map[string]any{
		"protocolVersion": protocol,
		"capabilities":    map[string]any{"tools": map[string]any{}},
		"serverInfo":      map[string]any{"name": "lognorth", "version": Version},
	}
}

// Tools describes what an agent may do. The descriptions are written for the
// agent reading them, so they say when to reach for each one.
var Tools = []map[string]any{
	{
		"name":        "list_apps",
		"description": "List the apps sending logs to LogNorth, with their ids. Call this first when you need an app_id for the other tools.",
		"inputSchema": map[string]any{"type": "object", "properties": map[string]any{}},
	},
	{
		"name": "search_logs",
		"description": "Search production request logs. Use this to find out what actually happened " +
			"before an error: the failing requests, their status codes, durations, and context. " +
			"Narrow with errors_only and since before raising the limit.",
		"inputSchema": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"search":      map[string]any{"type": "string", "description": "Text to match in the message and context, e.g. a path or an error string."},
				"errors_only": map[string]any{"type": "boolean", "description": "Only requests that failed. Default false."},
				"since":       map[string]any{"type": "string", "description": "How far back to look: 15m, 2h, 7d. Default 1h."},
				"app_id":      map[string]any{"type": "integer", "description": "Restrict to one app, from list_apps."},
				"limit":       map[string]any{"type": "integer", "description": "Max events, 1 to 200. Default 20."},
			},
		},
	},
	{
		"name": "get_event",
		"description": "Fetch one log event by id, with its full context and every other event sharing its trace. " +
			"Use it after search_logs to see the whole request that failed.",
		"inputSchema": map[string]any{
			"type":       "object",
			"properties": map[string]any{"id": map[string]any{"type": "integer", "description": "Event id from search_logs."}},
			"required":   []string{"id"},
		},
	},
	{
		"name": "list_issues",
		"description": "List grouped errors with occurrence counts and when each was last seen. " +
			"Start here when asked what is broken in production.",
		"inputSchema": map[string]any{
			"type":       "object",
			"properties": map[string]any{"app_id": map[string]any{"type": "integer", "description": "Restrict to one app, from list_apps."}},
		},
	},
}

func callTool(db *gorm.DB, params json.RawMessage) map[string]any {
	var p struct {
		Name      string         `json:"name"`
		Arguments map[string]any `json:"arguments"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return toolError("could not read the tool call: " + err.Error())
	}

	payload, err := runTool(db, p.Name, p.Arguments)
	if err != nil {
		return toolError(err.Error())
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return toolError("could not encode the result: " + err.Error())
	}

	return map[string]any{
		"content": []map[string]any{{"type": "text", "text": string(body)}},
	}
}

func runTool(db *gorm.DB, name string, args map[string]any) (any, error) {
	switch name {
	case "list_apps":
		return listApps(db)
	case "search_logs":
		return searchLogs(db, args)
	case "get_event":
		return getEvent(db, args)
	case "list_issues":
		return listIssues(db, args)
	}

	return nil, fmt.Errorf("unknown tool: %s", name)
}

func listApps(db *gorm.DB) (any, error) {
	keys, err := apikeys.List(db)
	if err != nil {
		return nil, fmt.Errorf("could not read the apps: %w", err)
	}

	apps := make([]map[string]any, len(keys))
	for i, k := range keys {
		apps[i] = map[string]any{"id": k.ID, "name": k.Name}
	}

	return map[string]any{"apps": apps}, nil
}

func searchLogs(db *gorm.DB, args map[string]any) (any, error) {
	since, err := parseSince(argString(args, "since"))
	if err != nil {
		return nil, err
	}

	filters := events.ListFilters{
		Search:    argString(args, "search"),
		StartTime: time.Now().Add(-since),
		Limit:     clampLimit(argInt(args, "limit")),
		APIKeyID:  uint(argInt(args, "app_id")),
	}
	if errorsOnly, ok := args["errors_only"].(bool); ok && errorsOnly {
		filters.IsError = &errorsOnly
	}

	found, total, err := events.List(db, filters)
	if err != nil {
		return nil, fmt.Errorf("could not read the log: %w", err)
	}

	return map[string]any{"events": found, "total": total}, nil
}

func getEvent(db *gorm.DB, args map[string]any) (any, error) {
	id := argInt(args, "id")
	if id <= 0 {
		return nil, fmt.Errorf("id is required and must be a positive event id")
	}

	event, err := events.FindByID(db, uint(id))
	if err != nil {
		return nil, fmt.Errorf("no event with id %d", id)
	}

	related := []events.Event{}
	if event.TraceID != "" {
		related, _ = events.FindByTraceID(db, event.TraceID)
	}
	eventContext, _ := event.GetContext()

	return map[string]any{
		"event":         event,
		"eventContext":  eventContext,
		"relatedEvents": related,
	}, nil
}

func listIssues(db *gorm.DB, args map[string]any) (any, error) {
	issues, err := events.ListIssuesWithState(db, uint(argInt(args, "app_id")), 100)
	if err != nil {
		return nil, fmt.Errorf("could not read the issues: %w", err)
	}

	return map[string]any{"issues": issues}, nil
}

func clampLimit(limit int) int {
	switch {
	case limit <= 0:
		return 20
	case limit > 200:
		return 200
	default:
		return limit
	}
}

// parseSince accepts Go durations plus a day suffix, because an agent asked
// for last week's errors writes "7d", not "168h".
func parseSince(s string) (time.Duration, error) {
	if s == "" {
		return time.Hour, nil
	}
	if days, ok := strings.CutSuffix(s, "d"); ok {
		n, err := strconv.Atoi(days)
		if err != nil || n <= 0 {
			return 0, fmt.Errorf("could not read since=%q: try 15m, 2h, or 7d", s)
		}
		return time.Duration(n) * 24 * time.Hour, nil
	}
	d, err := time.ParseDuration(s)
	if err != nil || d <= 0 {
		return 0, fmt.Errorf("could not read since=%q: try 15m, 2h, or 7d", s)
	}

	return d, nil
}

func argString(args map[string]any, key string) string {
	s, _ := args[key].(string)
	return strings.TrimSpace(s)
}

// argInt reads a number that arrived as JSON, so float64, or as a string from
// a client that quotes its arguments.
func argInt(args map[string]any, key string) int {
	switch v := args[key].(type) {
	case float64:
		return int(v)
	case string:
		n, _ := strconv.Atoi(strings.TrimSpace(v))
		return n
	}
	return 0
}

func toolError(message string) map[string]any {
	return map[string]any{
		"isError": true,
		"content": []map[string]any{{"type": "text", "text": message}},
	}
}
