package jobs

import (
	"testing"

	"lognorth/internal/license"
	"lognorth/internal/settings"
)

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
