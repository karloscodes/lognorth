//go:build polarsandbox

package license

// Built only by make dev and make dev-server: sandbox keys work there.
func init() { trySandbox = true }
