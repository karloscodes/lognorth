package events

import "strings"

// ShouldIgnorePath filters paths that are never legitimate application endpoints.
// This is a conservative server-side safety net — the SDK is the primary filter
// (it knows which routes actually exist in the app).
//
// Only dotfile paths are filtered here. No app serves /.env, /.git, /.aws, etc.
// Anything else (file extensions, CMS paths) could be a real endpoint in some app.
var ignoredPrefixes = []string{
	"/.", // /.env, /.git, /.aws, /.svn, /.docker, /.vscode, etc.
}

// ShouldIgnorePath returns true if the path is never a real application endpoint.
func ShouldIgnorePath(p string) bool {
	if p == "" {
		return false
	}

	p = strings.ToLower(p)

	for _, prefix := range ignoredPrefixes {
		if strings.HasPrefix(p, prefix) {
			return true
		}
	}

	return false
}
