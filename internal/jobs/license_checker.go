package jobs

import (
	"github.com/karloscodes/cartridge"

	"lognorth/internal/license"
	"lognorth/internal/settings"
)

// LicenseChecker revalidates the stored license against Polar once per
// run. Detects revoked or expired keys without hammering Polar — runs
// daily by default.
type LicenseChecker struct{}

func NewLicenseChecker() *LicenseChecker { return &LicenseChecker{} }

func (j *LicenseChecker) ProcessBatch(ctx *cartridge.JobContext) error {
	s, err := settings.Get(ctx.DB)
	if err != nil {
		return err
	}

	if s.LicenseKey == "" {
		license.SetConfigured(false)
		return nil
	}

	status, email := license.Validate(ctx.Context, s.LicenseKey)

	if status != s.LicenseStatus || email != s.LicenseEmail {
		s.LicenseStatus = status
		s.LicenseEmail = email
		if err := settings.Update(ctx.DB, s); err != nil {
			ctx.Logger.Error("Failed to persist license status", "error", err)
			return err
		}
		ctx.Logger.Info("License status changed", "status", status)
	}

	license.SetConfigured(status == "valid")
	return nil
}
