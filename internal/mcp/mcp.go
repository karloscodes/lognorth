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
	"lognorth/internal/uptime"
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
// agent reading them, so they say when to reach for each one. The order is the
// order of an investigation.
var Tools = []map[string]any{
	{
		"name":        "list_apps",
		"description": "List the apps sending logs to LogNorth, with their ids. Call this first when you need an app_id for the other tools.",
		"inputSchema": map[string]any{"type": "object", "properties": map[string]any{}},
	},
	{
		"name": "list_alerts",
		"description": "What LogNorth is alerting on right now: endpoints whose error rate is above their normal level, " +
			"endpoints whose traffic dropped, and apps that stopped answering their uptime ping. " +
			"Each alert says when it started and what normal is. Start here when something is on fire.",
		"inputSchema": map[string]any{
			"type":       "object",
			"properties": map[string]any{"app_id": map[string]any{"type": "integer", "description": "Restrict to one app, from list_apps."}},
		},
	},
	{
		"name": "list_endpoints",
		"description": "Every endpoint of one app with its requests, errors, error rate, and average duration over a window, most broken first. " +
			"Use it to see which endpoints fail right now before you drill into one with endpoint_timeline.",
		"inputSchema": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"app_id": map[string]any{"type": "integer", "description": "The app, from list_apps. Optional when there is only one app."},
				"since":  map[string]any{"type": "string", "description": "How far back: 1h, 24h, 7d. Default 1h. History goes back 29 days."},
				"limit":  map[string]any{"type": "integer", "description": "Max endpoints, 1 to 200. Default 50."},
			},
		},
	},
	{
		"name": "endpoint_timeline",
		"description": "Requests, errors, and error rate for one endpoint over time, with its normal error rate and volume for this time of day. " +
			"Use it to see when a problem started, then compare that time with deploys and commits. " +
			"Steps are 5 minutes up to 24h back, 1 hour beyond that. History goes back 29 days.",
		"inputSchema": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path":   map[string]any{"type": "string", "description": "Endpoint path, e.g. /api/checkout."},
				"app_id": map[string]any{"type": "integer", "description": "The app, from list_apps. Optional when only one app has this path."},
				"since":  map[string]any{"type": "string", "description": "How far back: 30m, 6h, 7d. Default 6h."},
			},
			"required": []string{"path"},
		},
	},
	{
		"name": "uptime_timeline",
		"description": "Whether an app's URL answers LogNorth's uptime ping, checked every minute: the latest result, response time, uptime over 24 hours, " +
			"and the last 24 hours in 15-minute bars that say why pings failed (an HTTP status or a timeout). " +
			"It also lists the outages of the last 7 days (3 or more failed pings in a row), newest first, each with the issues whose errors " +
			"surged against their own normal rate from 5 minutes before the outage to its end. Those issues are the likely cause; an empty list means no error stood out. " +
			"Use it when an app may be down.",
		"inputSchema": map[string]any{
			"type":       "object",
			"properties": map[string]any{"app_id": map[string]any{"type": "integer", "description": "The app, from list_apps. Optional when there is only one app."}},
		},
	},
	{
		"name": "list_issues",
		"description": "List grouped errors with occurrence counts, trend, and when each was first and last seen. " +
			"Start here when asked what is broken in production. Pass an issue's hash to search_logs to get its events.",
		"inputSchema": map[string]any{
			"type":       "object",
			"properties": map[string]any{"app_id": map[string]any{"type": "integer", "description": "Restrict to one app, from list_apps."}},
		},
	},
	{
		"name": "search_logs",
		"description": "Search production request logs. Use this to find out what actually happened " +
			"before an error: the failing requests, their status codes, durations, and context. " +
			"Narrow with errors_only, issue, and since before raising the limit.",
		"inputSchema": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"search":      map[string]any{"type": "string", "description": "Text to match in the message and context, e.g. a path or an error string."},
				"errors_only": map[string]any{"type": "boolean", "description": "Only requests that failed. Default false."},
				"issue":       map[string]any{"type": "string", "description": "An issue hash from list_issues: only the events of that issue."},
				"since":       map[string]any{"type": "string", "description": "Start of the window: 15m, 2h, 7d ago, or an RFC 3339 time. Default 1h."},
				"until":       map[string]any{"type": "string", "description": "End of the window, in the same forms as since. Default now."},
				"app_id":      map[string]any{"type": "integer", "description": "Restrict to one app, from list_apps."},
				"limit":       map[string]any{"type": "integer", "description": "Max events, 1 to 200. Default 20."},
				"after_id": map[string]any{"type": "integer", "description": "Only events stored after this event id, oldest first. " +
					"Use it to follow the log: pass the highest id you have seen. With after_id, since has no default bound."},
			},
		},
	},
	{
		"name": "get_event",
		"description": "Fetch one log event by id, with its full context and every other event sharing its trace. " +
			"Use it after search_logs to see the whole request that failed. error_file and error_line point at the code.",
		"inputSchema": map[string]any{
			"type":       "object",
			"properties": map[string]any{"id": map[string]any{"type": "integer", "description": "Event id from search_logs."}},
			"required":   []string{"id"},
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
	case "list_alerts":
		return listAlerts(db, args)
	case "list_endpoints":
		return listEndpoints(db, args)
	case "endpoint_timeline":
		return endpointTimeline(db, args)
	case "uptime_timeline":
		return uptimeTimeline(db, args)
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
	// A follower pages by id, so the default one-hour window would hide an
	// event whose timestamp arrived late.
	afterID := uint(max(0, argInt(args, "after_id")))
	window := time.Hour
	if afterID > 0 {
		window = 0
	}
	start, err := parseTime(argString(args, "since"), window)
	if err != nil {
		return nil, err
	}
	end, err := parseTime(argString(args, "until"), 0)
	if err != nil {
		return nil, err
	}

	filters := events.ListFilters{
		Search:    argString(args, "search"),
		ErrorHash: argString(args, "issue"),
		StartTime: start,
		EndTime:   end,
		Limit:     clampLimit(argInt(args, "limit")),
		APIKeyID:  uint(argInt(args, "app_id")),
		AfterID:   afterID,
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

func listAlerts(db *gorm.DB, args map[string]any) (any, error) {
	appID := uint(argInt(args, "app_id"))

	alerts, err := events.ListAlerts(db, appID)
	if err != nil {
		return nil, fmt.Errorf("could not read the alerts: %w", err)
	}

	// Uptime lives on the app: the ping to its URL failed.
	keys, err := apikeys.List(db)
	if err != nil {
		return nil, fmt.Errorf("could not read the apps: %w", err)
	}
	down := []map[string]any{}
	for _, k := range keys {
		if k.IsDown && (appID == 0 || k.ID == appID) {
			down = append(down, map[string]any{
				"app_id":   k.ID,
				"app":      k.Name,
				"url":      k.PingURL,
				"since":    k.DownSince,
				"severity": "critical",
				"summary":  k.PingURL + " is not answering its uptime ping.",
			})
		}
	}

	return map[string]any{"alerts": alerts, "down": down}, nil
}

func endpointTimeline(db *gorm.DB, args map[string]any) (any, error) {
	path := argString(args, "path")
	if path == "" {
		return nil, fmt.Errorf("path is required, e.g. /api/checkout")
	}
	appID, err := resolveApp(db, uint(argInt(args, "app_id")), path)
	if err != nil {
		return nil, err
	}

	since := argString(args, "since")
	if since == "" {
		since = "6h"
	}
	from, err := parseTime(since, 0)
	if err != nil {
		return nil, err
	}
	if oldest := time.Now().Add(-events.StatsRetention); from.Before(oldest) {
		from = oldest
	}
	step := 5 * time.Minute
	if time.Since(from) > 24*time.Hour {
		step = time.Hour
	}

	points, err := events.GetTimeline(db, appID, path, from.UTC(), time.Now().UTC(), step)
	if err != nil {
		return nil, fmt.Errorf("could not read the timeline: %w", err)
	}
	at := events.LastCompleteBucketTime()
	errorRate, _ := events.GetErrorRateBaseline(db, appID, path, at)
	volume, _ := events.GetVolumeBaseline(db, appID, path, at)

	return map[string]any{
		"app_id":       appID,
		"path":         path,
		"step_minutes": int(step.Minutes()),
		"normal": map[string]any{
			"error_rate":       errorRate.Rate,
			"requests_per_5m":  volume.Median,
			"history_is_ready": errorRate.Ready(),
		},
		"points": points,
	}, nil
}

func listEndpoints(db *gorm.DB, args map[string]any) (any, error) {
	appID, err := onlyApp(db, uint(argInt(args, "app_id")))
	if err != nil {
		return nil, err
	}

	since := argString(args, "since")
	if since == "" {
		since = "1h"
	}
	from, err := parseTime(since, 0)
	if err != nil {
		return nil, err
	}
	if oldest := time.Now().Add(-events.StatsRetention); from.Before(oldest) {
		from = oldest
	}
	limit := argInt(args, "limit")
	if limit <= 0 {
		limit = 50
	}

	stats, err := events.GetEndpointStatsSince(db, appID, from, min(limit, 200))
	if err != nil {
		return nil, fmt.Errorf("could not read the endpoints: %w", err)
	}

	return map[string]any{"app_id": appID, "since": from.UTC(), "endpoints": stats}, nil
}

func uptimeTimeline(db *gorm.DB, args map[string]any) (any, error) {
	appID, err := onlyApp(db, uint(argInt(args, "app_id")))
	if err != nil {
		return nil, err
	}
	key, err := apikeys.FindByID(db, appID)
	if err != nil {
		return nil, fmt.Errorf("no app with id %d", appID)
	}

	now := time.Now()
	outages, err := uptime.RecentOutages(db, appID, now.Add(-7*24*time.Hour), 10)
	if err != nil {
		return nil, fmt.Errorf("could not read the outages: %w", err)
	}
	return struct {
		uptime.Report
		Outages []uptime.OutageSurges `json:"outages"`
	}{uptime.ReportFor(db, *key, now, 24*time.Hour, 96), outages}, nil
}

// onlyApp returns the app to read. The app_id may be left out when there is
// only one app.
func onlyApp(db *gorm.DB, appID uint) (uint, error) {
	if appID > 0 {
		return appID, nil
	}

	keys, err := apikeys.List(db)
	if err != nil {
		return 0, fmt.Errorf("could not read the apps: %w", err)
	}
	switch len(keys) {
	case 0:
		return 0, fmt.Errorf("no apps yet")
	case 1:
		return keys[0].ID, nil
	default:
		return 0, fmt.Errorf("%d apps: pass app_id, from list_apps", len(keys))
	}
}

// resolveApp returns the app to read. The app_id may be left out when only
// one app has traffic on the path.
func resolveApp(db *gorm.DB, appID uint, path string) (uint, error) {
	if appID > 0 {
		return appID, nil
	}

	ids, err := events.AppsWithPath(db, path)
	if err != nil {
		return 0, fmt.Errorf("could not read the endpoints: %w", err)
	}
	switch len(ids) {
	case 0:
		return 0, fmt.Errorf("no traffic on %s in the last 29 days", path)
	case 1:
		return ids[0], nil
	default:
		return 0, fmt.Errorf("%d apps have traffic on %s: pass app_id, from list_apps", len(ids), path)
	}
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

// parseTime reads a point in time: a duration ago (Go durations plus a day
// suffix, because an agent asked for last week's errors writes "7d", not
// "168h"), or an RFC 3339 time. Empty means `fallback` ago; a zero fallback
// means no bound.
func parseTime(s string, fallback time.Duration) (time.Time, error) {
	if s == "" {
		if fallback == 0 {
			return time.Time{}, nil
		}
		return time.Now().Add(-fallback), nil
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t, nil
	}

	d, err := parseDuration(s)
	if err != nil {
		return time.Time{}, fmt.Errorf("could not read %q: try 15m, 2h, 7d, or an RFC 3339 time", s)
	}
	return time.Now().Add(-d), nil
}

func parseDuration(s string) (time.Duration, error) {
	if days, ok := strings.CutSuffix(s, "d"); ok {
		n, err := strconv.Atoi(days)
		if err != nil || n <= 0 {
			return 0, fmt.Errorf("bad day count")
		}
		return time.Duration(n) * 24 * time.Hour, nil
	}
	d, err := time.ParseDuration(s)
	if err != nil || d <= 0 {
		return 0, fmt.Errorf("bad duration")
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
