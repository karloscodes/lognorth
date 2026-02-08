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
			// Get current error rate
			errorRate, _, err := events.GetCurrentErrorRate(db, key.ID, path)
			if err != nil {
				continue // Skip this endpoint
			}

			// Get or create baseline for this hour-of-week
			baseline := events.GetOrCreateBaseline(db, key.ID, path, hourOfWeek)

			// Update baseline with new observation
			events.UpdateBaseline(db, baseline, errorRate)
		}
	}

	return nil
}
