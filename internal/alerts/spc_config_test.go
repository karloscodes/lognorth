package alerts

import (
	"testing"
	"time"

	"lognorth/internal/events"
)

// endpoint_stats must keep the longest history a check reads: 4 weeks of
// volume, and a day for the window around each point.
func TestStatsAreKeptForTheLongestHistory(t *testing.T) {
	need := (VolumeHistoryWeeks*7 + 1) * 24 * time.Hour

	if events.StatsRetention < need {
		t.Errorf("events.StatsRetention is %s, want at least %s", events.StatsRetention, need)
	}
}
