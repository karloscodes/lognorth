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

// DigestSender sends daily digest emails.
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

	// Get timezone
	loc, err := time.LoadLocation(s.DigestTimezone)
	if err != nil {
		loc = time.UTC
	}

	now := time.Now().In(loc)
	today := now.Format("2006-01-02")

	// Only send once per day at the configured hour
	if j.lastSentDate == today {
		return nil
	}

	if now.Hour() != s.DigestHour {
		return nil
	}

	// Get pending digest items
	items := events.GetPendingDigestItems(db)
	if len(items) == 0 {
		j.lastSentDate = today
		return nil
	}

	// Build digest email
	subject := fmt.Sprintf("LogNorth Daily Digest - %s", now.Format("Jan 2"))
	body := j.buildDigestBody(db, items, s)

	if err := s.SendEmail(subject, body); err != nil {
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

	sb.WriteString("Yesterday's summary across your apps:\n\n")

	// Group warnings
	var warnings []string
	for _, item := range items {
		warnings = append(warnings, fmt.Sprintf("- %s", item.Message))
	}

	if len(warnings) > 0 {
		sb.WriteString("WARNINGS (resolved automatically)\n")
		sb.WriteString(strings.Join(warnings, "\n"))
		sb.WriteString("\n\n")
	}

	// Add stats summary
	sb.WriteString("STATS\n")

	var keys []apikeys.APIKey
	db.Where("active = ?", true).Find(&keys)

	yesterday := time.Now().AddDate(0, 0, -1)

	for _, key := range keys {
		var totalEvents, totalErrors int64

		db.Model(&events.Event{}).
			Where("api_key_id = ? AND timestamp > ?", key.ID, yesterday).
			Count(&totalEvents)

		db.Model(&events.Event{}).
			Where("api_key_id = ? AND timestamp > ? AND is_error = ?", key.ID, yesterday, true).
			Count(&totalErrors)

		if totalEvents > 0 {
			errorRate := float64(totalErrors) / float64(totalEvents) * 100
			sb.WriteString(fmt.Sprintf("- %s: %d events, %d errors (%.2f%%)\n",
				key.Name, totalEvents, totalErrors, errorRate))
		}
	}

	sb.WriteString("\nNo action needed - just keeping you informed.\n")
	sb.WriteString(fmt.Sprintf("\nView all: %s\n", s.BaseURL))

	return sb.String()
}
