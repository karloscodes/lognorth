package jobs

import (
	"fmt"
	"strings"
	"time"

	"github.com/karloscodes/cartridge"
	"gorm.io/gorm"

	"lognorth/internal/apikeys"
	"lognorth/internal/events"
	"lognorth/internal/settings"
)

// digestHour is the earliest hour the digest goes out, in server time: UTC in
// the Docker image, or the zone in TZ, the same clock quiet hours use.
const digestHour = 9

// DigestSender sends the daily digest of warnings.
type DigestSender struct {
	lastSentDate string
}

// NewDigestSender creates a new digest sender.
func NewDigestSender() *DigestSender {
	return &DigestSender{}
}

// ProcessBatch checks if it's time to send the digest and sends it.
func (j *DigestSender) ProcessBatch(ctx *cartridge.JobContext) error {
	db := ctx.DB
	s, err := settings.Get(db)
	if err != nil || !s.CanNotify() {
		return nil
	}

	// Once a day, at the first hour from digestHour that is outside quiet
	// hours (CanNotify above already returned while they are on).
	now := time.Now()
	today := now.Format("2006-01-02")
	if j.lastSentDate == today || now.Hour() < digestHour {
		return nil
	}

	// Get pending digest items
	items := events.GetPendingDigestItems(db)
	if len(items) == 0 {
		j.lastSentDate = today
		return nil
	}

	// Build digest email
	subject := fmt.Sprintf("Daily digest, %s", now.Format("Jan 2"))
	body := j.buildDigestBody(db, items, s)

	if err := s.SendNotification(subject, body); err != nil {
		ctx.Logger.Error("Failed to send digest", "error", err)
		return err
	}

	// Mark items as resolved
	events.MarkDigestItemsResolved(db, items)
	j.lastSentDate = today

	ctx.Logger.Info("Sent daily digest", "items", len(items))
	return nil
}

func (j *DigestSender) buildDigestBody(db *gorm.DB, items []events.DigestItem, s *settings.Settings) string {
	var sb strings.Builder

	sb.WriteString("Yesterday across your apps:\n\n")

	var keys []apikeys.APIKey
	db.Where("active = ?", true).Find(&keys)

	yesterday := time.Now().UTC().AddDate(0, 0, -1)
	wroteStats := false
	for _, key := range keys {
		var totalEvents, totalErrors int64

		db.Model(&events.Event{}).
			Where("api_key_id = ? AND timestamp > ?", key.ID, yesterday).
			Count(&totalEvents)

		db.Model(&events.Event{}).
			Where("api_key_id = ? AND timestamp > ? AND is_error = 1", key.ID, yesterday).
			Count(&totalErrors)

		if totalEvents > 0 {
			errorRate := float64(totalErrors) / float64(totalEvents) * 100
			sb.WriteString(fmt.Sprintf("- %s: %d events, %d errors (%.1f%%)\n",
				key.Name, totalEvents, totalErrors, errorRate))
			wroteStats = true
		}
	}

	if len(items) > 0 {
		if wroteStats {
			sb.WriteString("\n")
		}
		sb.WriteString("Transient warnings that resolved on their own:\n")
		for _, item := range items {
			sb.WriteString(fmt.Sprintf("- %s\n", item.Message))
		}
	}

	if s.BaseURL != "" {
		sb.WriteString(fmt.Sprintf("\n%s\n", s.BaseURL))
	}

	return sb.String()
}
