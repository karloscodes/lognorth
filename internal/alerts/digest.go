package alerts

import (
	"fmt"
	"log/slog"
	"lognorth/internal/apikeys"
	"lognorth/internal/events"
	"lognorth/internal/settings"
	"strings"
	"time"

	"gorm.io/gorm"
)

// digestHour is the earliest hour the digest goes out, in the owner's zone
// (Location), the clock quiet hours use too.
const digestHour = 9

// Digest sends the daily digest of warnings, once a day.
type Digest struct {
	lastSentDate string
}

// Run sends the digest when it is due: once a day, at the first hour from
// digestHour that is outside quiet hours.
func (j *Digest) Run(db *gorm.DB, logger *slog.Logger, now time.Time) error {
	s, err := settings.Get(db)
	if err != nil || !ChannelConfigured(s) || IsQuietHours(s, now) {
		return nil
	}

	local := now.In(Location(s))
	today := local.Format("2006-01-02")
	if j.lastSentDate == today || local.Hour() < digestHour {
		return nil
	}

	// Get pending digest items
	items := GetPendingDigestItems(db)
	if len(items) == 0 {
		j.lastSentDate = today
		return nil
	}

	// Build digest email
	subject := fmt.Sprintf("Daily digest, %s", local.Format("Jan 2"))
	body := j.buildDigestBody(db, items, s, now)

	if err := Send(s, subject, body); err != nil {
		logger.Error("Failed to send digest", "error", err)
		return err
	}

	// Mark items as resolved
	MarkDigestItemsResolved(db, items)
	j.lastSentDate = today

	logger.Info("Sent daily digest", "items", len(items))
	return nil
}

func (j *Digest) buildDigestBody(db *gorm.DB, items []DigestItem, s *settings.Settings, now time.Time) string {
	var sb strings.Builder

	sb.WriteString("Yesterday across your apps:\n\n")

	var keys []apikeys.APIKey
	db.Where("active = ?", true).Find(&keys)

	// The last 24 whole hours and the hour that runs now.
	since := now.UTC().Truncate(time.Hour).Add(-24 * time.Hour)
	wroteStats := false
	for _, key := range keys {
		var day struct{ Total, Errors int64 }
		db.Model(&events.AppHour{}).
			Select("COALESCE(SUM(total), 0) AS total, COALESCE(SUM(errors), 0) AS errors").
			Where("api_key_id = ? AND hour >= ?", key.ID, since).
			Scan(&day)
		totalEvents, totalErrors := day.Total, day.Errors

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
