package jobs

import (
	"github.com/karloscodes/cartridge"

	"lognorth/internal/events"
)

// SpikeChecker checks for endpoint error rate spikes and recoveries
type SpikeChecker struct{}

// NewSpikeChecker creates a new spike checker
func NewSpikeChecker() *SpikeChecker {
	return &SpikeChecker{}
}

// ProcessBatch checks for spikes and recoveries, sends grouped alerts
func (p *SpikeChecker) ProcessBatch(ctx *cartridge.JobContext) error {
	engine, err := events.NewAlertEngine(ctx.DB)
	if err != nil {
		ctx.Logger.Error("Failed to create alert engine", "error", err)
		return err
	}

	engine.CheckSpikes()
	return nil
}
