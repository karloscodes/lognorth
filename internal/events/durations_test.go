package events

import (
	"math"
	"testing"
	"time"

	"gorm.io/gorm"
)

// requests stores one batch of requests to an endpoint, as the processor
// counts the events it moves: the 5-minute stats and the duration bins.
func requests(t *testing.T, db *gorm.DB, path string, at time.Time, durations ...int) {
	t.Helper()
	err := db.Transaction(func(tx *gorm.DB) error {
		stats, bins := NewStatBatch(), DurationBatch{}
		for _, ms := range durations {
			if err := stats.Add(tx, 1, path, false, ms, at); err != nil {
				return err
			}
			bins.Add(1, path, ms, at)
		}
		if err := stats.Flush(tx); err != nil {
			return err
		}
		return bins.Flush(tx)
	})
	if err != nil {
		t.Fatal(err)
	}
}

func repeat(ms, times int) []int {
	all := make([]int, times)
	for i := range all {
		all[i] = ms
	}
	return all
}

func within10Percent(t *testing.T, name string, got *int, want int) {
	t.Helper()
	if got == nil {
		t.Fatalf("%s = none, want about %dms", name, want)
	}
	if math.Abs(float64(*got-want)) > 0.1*float64(want) {
		t.Errorf("%s = %dms, want within 10%% of %dms", name, *got, want)
	}
}

func TestEndpointPercentiles(t *testing.T) {
	now := time.Now().UTC()

	t.Run("p95 and p99 come within 10% of the exact durations", func(t *testing.T) {
		db := setupTestDB(t)
		var durations []int
		for ms := 1; ms <= 1000; ms++ {
			durations = append(durations, ms)
		}
		requests(t, db, "/search", now, durations...)

		stats, err := GetEndpointStats(db, 1, 10)

		if err != nil || len(stats) != 1 {
			t.Fatalf("stats = %+v, err = %v", stats, err)
		}
		within10Percent(t, "p95", stats[0].P95Duration, 950)
		within10Percent(t, "p99", stats[0].P99Duration, 990)
	})

	t.Run("a slow tail shows in p99, though the average hides it", func(t *testing.T) {
		db := setupTestDB(t)
		requests(t, db, "/checkout", now, append(repeat(50, 980), repeat(3000, 20)...)...)

		stats, _ := GetEndpointStats(db, 1, 10)

		within10Percent(t, "p95", stats[0].P95Duration, 50)
		within10Percent(t, "p99", stats[0].P99Duration, 3000)
		if stats[0].AvgDuration > 120 {
			t.Errorf("avg = %dms, want about 109ms", stats[0].AvgDuration)
		}
	})

	t.Run("batches of the same hour add up", func(t *testing.T) {
		db := setupTestDB(t)
		requests(t, db, "/checkout", now, repeat(10, 950)...)
		requests(t, db, "/checkout", now, repeat(1000, 50)...)

		stats, _ := GetEndpointStats(db, 1, 10)

		within10Percent(t, "p95", stats[0].P95Duration, 10)
		within10Percent(t, "p99", stats[0].P99Duration, 1000)
	})

	t.Run("requests under 1 ms have a p95 of 0", func(t *testing.T) {
		db := setupTestDB(t)
		requests(t, db, "/health", now, repeat(0, 100)...)

		stats, _ := GetEndpointStats(db, 1, 10)

		if p := stats[0].P95Duration; p == nil || *p != 0 {
			t.Errorf("p95 = %v, want 0", p)
		}
	})

	t.Run("hours before the window do not count", func(t *testing.T) {
		db := setupTestDB(t)
		requests(t, db, "/report", now.Add(-3*time.Hour), repeat(5000, 100)...)
		requests(t, db, "/report", now, repeat(20, 100)...)

		stats, _ := GetEndpointStatsSince(db, 1, now.Add(-time.Hour), 10)

		within10Percent(t, "p99", stats[0].P99Duration, 20)
	})

	t.Run("an endpoint counted before the bins existed has none", func(t *testing.T) {
		db := setupTestDB(t)
		db.Create(&EndpointStat{APIKeyID: 1, Path: "/old", Bucket: now.Truncate(5 * time.Minute), Total: 10, AvgDuration: 30})

		stats, _ := GetEndpointStats(db, 1, 10)

		if stats[0].P95Duration != nil || stats[0].P99Duration != nil {
			t.Errorf("p95 = %v, p99 = %v; want none", stats[0].P95Duration, stats[0].P99Duration)
		}
	})
}
