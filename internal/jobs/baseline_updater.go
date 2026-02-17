package jobs

import (
	"time"

	"github.com/karloscodes/cartridge"

	"lognorth/internal/apikeys"
	"lognorth/internal/events"
)

// BaselineUpdater updates SPC baselines for all active endpoints.
type BaselineUpdater struct{}

// NewBaselineUpdater creates a new baseline updater.
func NewBaselineUpdater() *BaselineUpdater {
	return &BaselineUpdater{}
}

// ProcessBatch updates baselines for all active endpoints.
func (j *BaselineUpdater) ProcessBatch(ctx *cartridge.JobContext) error {
	db := ctx.DB

	// Get all active API keys
	var keys []apikeys.APIKey
	if err := db.Where("active = ?", true).Find(&keys).Error; err != nil {
		return err
	}

	hourOfWeek := events.HourOfWeek(time.Now())

	for _, key := range keys {
		endpoints := events.GetActiveEndpoints(db, key.ID)

		for _, path := range endpoints {
			// Get current error rate and volume
			errorRate, total, err := events.GetCurrentErrorRate(db, key.ID, path)
			if err != nil {
				continue // Skip this endpoint
			}

			// Update error rate baseline
			baseline := events.GetOrCreateBaseline(db, key.ID, path, hourOfWeek)
			events.UpdateBaseline(db, baseline, errorRate)

			// Update volume baseline
			volumeBaseline := events.GetOrCreateVolumeBaseline(db, key.ID, path, hourOfWeek)
			events.UpdateVolumeBaseline(db, volumeBaseline, float64(total))
		}
	}

	return nil
}
