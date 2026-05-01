package jobs

import (
	"testing"

	"lognorth/internal/license"
	"lognorth/internal/polar"
	"lognorth/internal/settings"
)

// setPolarProductionBaseURL points the polar package at the given URL
// for the duration of a test and returns a restore function.
func setPolarProductionBaseURL(url string) func() {
	prev := polar.ProductionBaseURL
	polar.ProductionBaseURL = url
	return func() { polar.ProductionBaseURL = prev }
}

func TestLicenseChecker_NoKey_ClearsFlag(t *testing.T) {
	db := setupProcessorDB(t)
	license.SetConfigured(true) // pretend it was set
	t.Cleanup(func() { license.SetConfigured(false) })

	if err := NewLicenseChecker().ProcessBatch(newJobCtx(db)); err != nil {
		t.Fatalf("ProcessBatch: %v", err)
	}

	if license.IsConfigured() {
		t.Fatalf("expected flag to be false when no license key is stored")
	}

	s, _ := settings.Get(db)
	if s.LicenseStatus != "" {
		t.Fatalf("expected empty LicenseStatus, got %q", s.LicenseStatus)
	}
}

// TestLicenseChecker_PolarUnreachable_KeepsStatus pins the behavior we
// care most about: a Polar outage must NOT flip a paying customer's
// license to invalid. Pointing the polar client at an unreachable URL
// simulates a network failure; status and the cached flag must survive.
func TestLicenseChecker_PolarUnreachable_KeepsStatus(t *testing.T) {
	t.Setenv("LOGNORTH_ENV", "production") // skip dev sandbox path

	db := setupProcessorDB(t)

	s, _ := settings.Get(db)
	s.LicenseKey = "lgn-test-key"
	s.LicenseStatus = "valid"
	s.LicenseEmail = "carlos@example.com"
	if err := settings.Update(db, s); err != nil {
		t.Fatalf("seed settings: %v", err)
	}

	license.SetConfigured(true)
	t.Cleanup(func() { license.SetConfigured(false) })

	// Force every Polar call to fail by pointing the package at an
	// unroutable address. license.Validate creates a client per call
	// but reads BaseURL constants from the polar package.
	prevProd := setPolarProductionBaseURL("http://127.0.0.1:1") // connection-refused
	t.Cleanup(prevProd)

	if err := NewLicenseChecker().ProcessBatch(newJobCtx(db)); err != nil {
		t.Fatalf("ProcessBatch: %v", err)
	}

	got, _ := settings.Get(db)
	if got.LicenseStatus != "valid" {
		t.Fatalf("status flipped on transient failure: got %q, want valid", got.LicenseStatus)
	}
	if !license.IsConfigured() {
		t.Fatalf("flag flipped to false on transient failure")
	}
}
