// Package license tracks whether a valid license is configured.
//
// The flag is set at boot from settings.LicenseStatus, refreshed when the
// user saves settings, and revalidated daily against Polar to detect
// revocation/expiry. Domain code (e.g. the event processor) reads it to
// decide whether to do work.
package license

import (
	"context"
	"os"
	"sync/atomic"
	"time"

	"lognorth/internal/polar"
)

var configured atomic.Bool

// In e2e/test runs the binary boots against a fresh DB with no license
// key. Forcing IsConfigured()=true in that env keeps the event processor
// running so ingestion tests can verify the actual flow. Production and
// development still respect the stored status.
var forceConfigured = os.Getenv("LOGNORTH_ENV") == "test"

// SetConfigured updates the cached license status.
func SetConfigured(v bool) {
	configured.Store(v)
}

// IsConfigured returns true when a valid license is in place.
func IsConfigured() bool {
	return forceConfigured || configured.Load()
}

// Validate checks a license key against Polar.
//
// Returns ("valid"|"invalid"|"expired", email, nil) on a definitive answer
// (Polar replied), or ("", "", err) on transient failures (network, 5xx
// after retries). Callers should treat err != nil as "no opinion" and
// keep whatever they have stored — flipping a paying customer to invalid
// because Polar had a bad day is not OK.
//
// Sandbox is tried first in development so devs can use sandbox keys;
// production is the fallback there and the only call in production.
func Validate(ctx context.Context, key string) (status, email string, err error) {
	if key == "" {
		return "", "", nil
	}

	client := polar.NewClient()
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	if os.Getenv("LOGNORTH_ENV") == "development" || os.Getenv("LOGNORTH_ENV") == "" {
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
