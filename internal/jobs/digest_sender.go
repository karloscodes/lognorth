package jobs

import (
	"github.com/karloscodes/cartridge"

	"lognorth/internal/alerts"
)

// DigestSender runs the daily digest of internal/alerts.
type DigestSender struct {
	digest alerts.Digest
}

// NewDigestSender creates a new digest sender.
func NewDigestSender() *DigestSender {
	return &DigestSender{}
}

// ProcessBatch sends the digest when it is due.
func (j *DigestSender) ProcessBatch(ctx *cartridge.JobContext) error {
	return j.digest.Run(ctx.DB, ctx.Logger)
}
