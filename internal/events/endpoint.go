package events

import (
	"regexp"
	"slices"
	"strings"

	"gorm.io/gorm"
)

// An endpoint is what endpoint_stats, the Endpoints page, and the alerts key
// on: a route, not a URL. /users/123 and /users/124 are one endpoint, so
// they share one history and one baseline. A key reads like a path, so the
// pages, the MCP tools, and north show it as one.

// EndpointKey returns the endpoint of a request: the route the SDK sent when
// it covers the path, else the path with its ids replaced by :id. Express
// sends the route relative to its router (/:id under /api/users), so a route
// counts only when it matches the path segment by segment.
func EndpointKey(route, path string) string {
	if path == "" {
		return ""
	}
	if key := routeKey(route); key != "" && covers(key, path) {
		return key
	}
	return normalizePath(path)
}

var uuidSegment = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// normalizePath replaces each id segment of a path with :id: all digits, a
// UUID, or hex of 16 characters or more (a database or object id, a hash).
func normalizePath(path string) string {
	segments := strings.Split(path, "/")
	for i, s := range segments {
		if isID(s) {
			segments[i] = ":id"
		}
	}
	return strings.Join(segments, "/")
}

func isID(s string) bool {
	if s == "" {
		return false
	}
	if strings.Trim(s, "0123456789") == "" || uuidSegment.MatchString(s) {
		return true
	}
	return len(s) >= 16 && strings.Trim(s, "0123456789abcdefABCDEF") == ""
}

// routeKey turns a route as an SDK sends it into a key. A Go pattern loses
// its method and host ("GET example.com/users/{id}"); {id} reads :id as in
// Express, {rest...} reads *rest, and {$} (Go's exact match) goes.
func routeKey(route string) string {
	route = strings.TrimSpace(route)
	if i := strings.LastIndex(route, " "); i >= 0 {
		route = route[i+1:]
	}
	if !strings.HasPrefix(route, "/") {
		i := strings.Index(route, "/")
		if i < 0 {
			return ""
		}
		route = route[i:]
	}
	segments := strings.Split(route, "/")
	for i, s := range segments {
		if !strings.HasPrefix(s, "{") || !strings.HasSuffix(s, "}") {
			continue
		}
		switch name := s[1 : len(s)-1]; {
		case name == "$":
			segments[i] = ""
		case strings.HasSuffix(name, "..."):
			segments[i] = "*" + strings.TrimSuffix(name, "...")
		default:
			segments[i] = ":" + name
		}
	}
	return strings.Join(segments, "/")
}

// covers reports whether a key matches a path segment by segment: :x matches
// one segment, *x the rest, and a key that ends in / (a Go subtree) every
// path under it.
func covers(key, path string) bool {
	if key == path {
		return true
	}
	if len(key) > 1 && strings.HasSuffix(key, "/") {
		return strings.HasPrefix(path, key)
	}
	keySegments, pathSegments := strings.Split(key, "/"), strings.Split(path, "/")
	for i, k := range keySegments {
		if strings.HasPrefix(k, "*") {
			return i < len(pathSegments)
		}
		if i >= len(pathSegments) {
			return false
		}
		if strings.HasPrefix(k, ":") {
			if pathSegments[i] == "" {
				return false
			}
			continue
		}
		if k != pathSegments[i] {
			return false
		}
	}
	return len(keySegments) == len(pathSegments)
}

// EndpointOf returns the endpoint of an app that a URL belongs to, for a
// lookup by URL (/users/123): the URL when it is a key, the URL with its ids
// replaced, or a route that covers it. It returns "" when the app has no
// traffic on it.
func EndpointOf(db *gorm.DB, appID uint, path string) (string, error) {
	matches, err := endpointsOf(db, appID, path)
	if err != nil || len(matches) == 0 {
		return "", err
	}
	return matches[0].Path, nil
}

// AppsWithPath returns the apps that sent traffic on the endpoint of a URL in
// the stored history.
func AppsWithPath(db *gorm.DB, path string) ([]uint, error) {
	matches, err := endpointsOf(db, 0, path)
	if err != nil {
		return nil, err
	}
	var ids []uint
	for _, m := range matches {
		if !slices.Contains(ids, m.APIKeyID) {
			ids = append(ids, m.APIKeyID)
		}
	}
	return ids, nil
}

type appEndpoint struct {
	APIKeyID uint
	Path     string
}

// endpointsOf returns the stored endpoints a URL belongs to, best first: the
// URL itself, then the URL with its ids replaced, then the routes that
// cover it. An appID of 0 means every app.
func endpointsOf(db *gorm.DB, appID uint, path string) ([]appEndpoint, error) {
	normalized := normalizePath(path)
	query := db.Model(&EndpointStat{}).Distinct("api_key_id", "path").
		Where("path IN ? OR path LIKE '%:%' OR path LIKE '%*%' OR path LIKE '%/'", []string{path, normalized})
	if appID > 0 {
		query = query.Where("api_key_id = ?", appID)
	}
	var rows []appEndpoint
	if err := query.Scan(&rows).Error; err != nil {
		return nil, err
	}

	rank := func(key string) int {
		switch {
		case key == path:
			return 0
		case key == normalized:
			return 1
		case covers(key, path):
			return 2
		}
		return -1
	}
	var matches []appEndpoint
	for _, r := range rows {
		if rank(r.Path) >= 0 {
			matches = append(matches, r)
		}
	}
	slices.SortStableFunc(matches, func(a, b appEndpoint) int { return rank(a.Path) - rank(b.Path) })
	return matches, nil
}

// IsEndpointPattern reports whether s is an endpoint key with a parameter,
// such as /users/:id or /files/*path.
func IsEndpointPattern(s string) bool {
	return strings.HasPrefix(s, "/") && (strings.Contains(s, "/:") || strings.Contains(s, "/*"))
}

// endpointCondition matches the events whose path belongs to an endpoint:
// its literal segments as written, one segment for each :x, and any rest
// for a *x or a trailing /. It reads the path out of the context of each
// event, so it narrows a list that the app and the time already narrowed.
func endpointCondition(key string) (string, []any) {
	const eventPath = "json_extract(context, '$.path')"
	if !strings.ContainsAny(key, ":*") && !(len(key) > 1 && strings.HasSuffix(key, "/")) {
		return eventPath + " = ?", []any{key}
	}

	segments := strings.Split(key, "/")
	exactCount := true
	for i, s := range segments {
		switch {
		case strings.HasPrefix(s, "*"):
			segments[i] = "*"
			exactCount = false
		case strings.HasPrefix(s, ":"):
			segments[i] = "?*"
		default:
			segments[i] = globEscaper.Replace(s)
		}
	}
	glob := strings.Join(segments, "/")
	if len(key) > 1 && strings.HasSuffix(key, "/") {
		glob += "*"
		exactCount = false
	}
	if !exactCount {
		return eventPath + " GLOB ?", []any{glob}
	}
	return eventPath + " GLOB ? AND length(" + eventPath + ") - length(replace(" + eventPath + ", '/', '')) = ?",
		[]any{glob, strings.Count(key, "/")}
}

// globEscaper makes a literal segment match itself in GLOB.
var globEscaper = strings.NewReplacer("[", "[[]", "*", "[*]", "?", "[?]")
