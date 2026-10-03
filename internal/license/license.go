// Package license tracks whether a valid license is configured.
//
// The flag is set at boot from settings.LicenseStatus, refreshed when the
// user saves settings, and revalidated daily against Polar to detect
// revocation/expiry. LogNorth is free for one app; the license adds
// unlimited apps, so app creation reads it.
package license

import (
	"context"
	"sync/atomic"
	"time"

	"lognorth/internal/polar"
)

// trySandbox is set only by the polarsandbox build tag (sandbox.go).
var trySandbox bool

var configured atomic.Bool

// SetConfigured updates the cached license status.
func SetConfigured(v bool) {
	configured.Store(v)
}

// IsConfigured returns true when a valid license is in place.
func IsConfigured() bool {
	return configured.Load()
}

// Validate checks a license key against Polar.
//
// Returns ("valid"|"invalid"|"expired", email, nil) on a definitive answer
// (Polar replied), or ("", "", err) on transient failures (network, 5xx
// after retries). Callers should treat err != nil as "no opinion" and
// keep whatever they have stored — flipping a paying customer to invalid
// because Polar had a bad day is not OK.
//
// Only a build with the polarsandbox tag (make dev) tries the sandbox first,
// so devs can use sandbox keys. The tag, not LOGNORTH_ENV, decides: a
// self-hoster sets the environment, and a sandbox "purchase" is free.
func Validate(ctx context.Context, key string) (status, email string, err error) {
	if key == "" {
		return "", "", nil
	}

	client := polar.NewClient()
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	if trySandbox {
		client.BaseURL = polar.SandboxBaseURL
		if resp, sandboxErr := client.ValidateLicense(ctx, polar.SandboxOrgID, key); sandboxErr == nil && resp != nil && resp.ID != "" {
			return polar.LicenseStatus(resp), resp.Customer.Email, nil
		}
	}

	client.BaseURL = polar.ProductionBaseURL
	resp, prodErr := client.ValidateLicense(ctx, polar.ProductionOrgID, key)
	if prodErr != nil {
		return "", "", prodErr
	}
	if resp == nil || resp.ID == "" {
		return "invalid", "", nil
	}
	return polar.LicenseStatus(resp), resp.Customer.Email, nil
}
