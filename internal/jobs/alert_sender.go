package jobs

import (
	"time"

	"github.com/karloscodes/cartridge"

	"lognorth/internal/alerts"
)

// AlertSender delivers the alerts the detectors recorded (alerts.Deliver).
type AlertSender struct{}

// NewAlertSender creates a new alert sender.
func NewAlertSender() *AlertSender {
	return &AlertSender{}
}

// ProcessBatch sends the alerts that are due.
func (j *AlertSender) ProcessBatch(ctx *cartridge.JobContext) error {
	return alerts.Deliver(ctx.DB, time.Now())
}
