package jobs

import (
	"github.com/karloscodes/cartridge"

	"lognorth/internal/alerts"
)

// AnomalyChecker runs the endpoint checks of internal/alerts.
type AnomalyChecker struct{}

// NewAnomalyChecker creates a new anomaly checker.
func NewAnomalyChecker() *AnomalyChecker {
	return &AnomalyChecker{}
}

// ProcessBatch checks every endpoint against its history.
func (j *AnomalyChecker) ProcessBatch(ctx *cartridge.JobContext) error {
	return alerts.CheckEndpoints(ctx.DB)
}
