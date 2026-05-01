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

// SetConfigured updates the cached license status.
func SetConfigured(v bool) {
	configured.Store(v)
}

// IsConfigured returns true when a valid license is in place.
func IsConfigured() bool {
	return configured.Load()
}

// Validate checks a license key against Polar and returns (status, email).
// Status is one of "valid", "invalid", "expired".
func Validate(ctx context.Context, key string) (string, string) {
	if key == "" {
		return "", ""
	}

	client := polar.NewClient()
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	if os.Getenv("LOGNORTH_ENV") == "development" || os.Getenv("LOGNORTH_ENV") == "" {
		client.BaseURL = polar.SandboxBaseURL
		if resp, err := client.ValidateLicense(ctx, polar.SandboxOrgID, key); err == nil && resp != nil && resp.ID != "" {
			return polar.LicenseStatus(resp), resp.Customer.Email
		}
	}

	client.BaseURL = polar.ProductionBaseURL
	if resp, err := client.ValidateLicense(ctx, polar.ProductionOrgID, key); err == nil && resp != nil && resp.ID != "" {
		return polar.LicenseStatus(resp), resp.Customer.Email
	}

	return "invalid", ""
}
