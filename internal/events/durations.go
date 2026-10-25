package events

import (
	"math"
	"time"

	"gorm.io/gorm"
)

// EndpointDuration counts an endpoint's requests in one UTC hour whose
// duration falls in one bin. Each bin is 10% wider than the one before, so
// p95 and p99 come out within 10% of the exact value from a few rows per
// endpoint and hour, instead of from every event. The processor adds each
// batch in the transaction that stores its events.
//
// The table is WITHOUT ROWID (DurationTableOptions), keyed in the order the
// percentile query groups: an endpoint's bins are next to each other, and
// the key holds everything but the count, so SQLite sums without a sort.
type EndpointDuration struct {
	APIKeyID uint      `gorm:"primaryKey;autoIncrement:false"`
	Path     string    `gorm:"primaryKey;size:500"`
	Bin      int       `gorm:"primaryKey;autoIncrement:false"`
	Hour     time.Time `gorm:"primaryKey"`
	Count    int64     `gorm:"not null;default:0"`
}

// DurationTableOptions creates endpoint_durations as a clustered table.
const DurationTableOptions = "WITHOUT ROWID"

func (EndpointDuration) TableName() string {
	return "endpoint_durations"
}

// durationGrowth is how much wider each bin is than the one before.
const durationGrowth = 1.1

// durationBin returns the bin of a duration in ms. Bin 0 holds 0 ms; bin b
// holds the durations from growth^(b-1) up to growth^b.
func durationBin(ms int) int {
	if ms < 1 {
		return 0
	}
	return 1 + int(math.Floor(math.Log(float64(ms))/math.Log(durationGrowth)))
}

// binBounds returns the durations a bin holds, from lo up to hi.
func binBounds(bin int) (lo, hi float64) {
	if bin == 0 {
		return 0, 0
	}
	return math.Pow(durationGrowth, float64(bin-1)), math.Pow(durationGrowth, float64(bin))
}

type durationKey struct {
	apiKeyID uint
	path     string
	hour     time.Time
	bin      int
}

// DurationBatch counts a batch of request durations per endpoint, hour, and
// bin, and writes each once. Use it in the transaction that stores the events.
type DurationBatch map[durationKey]int64

// Add counts one request of an endpoint.
func (b DurationBatch) Add(apiKeyID uint, path string, durationMS int, at time.Time) {
	if path == "" {
		return
	}
	b[durationKey{apiKeyID, path, at.UTC().Truncate(time.Hour), durationBin(durationMS)}]++
}

// Flush adds the batch to the stored bins: one write per bin, no read.
func (b DurationBatch) Flush(tx *gorm.DB) error {
	for key, count := range b {
		err := tx.Exec(`INSERT INTO endpoint_durations (api_key_id, path, hour, bin, count) VALUES (?, ?, ?, ?, ?)
			ON CONFLICT(api_key_id, path, bin, hour) DO UPDATE SET count = count + excluded.count`,
			key.apiKeyID, key.path, key.hour, key.bin, count).Error
		if err != nil {
			return err
		}
	}
	return nil
}

type binCount struct {
	Bin   int
	Count int64
}

// percentile returns the duration under which a share p of the requests
// fall, from bins in order. It goes into the bin that holds that request in
// a straight line, as if the bin's requests spread evenly across it.
func percentile(bins []binCount, p float64) int {
	var total int64
	for _, b := range bins {
		total += b.Count
	}
	rank := p * float64(total)
	var below float64
	for _, b := range bins {
		if below+float64(b.Count) >= rank {
			lo, hi := binBounds(b.Bin)
			return int(math.Round(lo + (hi-lo)*(rank-below)/float64(b.Count)))
		}
		below += float64(b.Count)
	}
	return 0
}

// addPercentiles sets p95 and p99 on each endpoint from its bins in the
// hours from `from` on. A bin counts its whole hour, so the window can start
// up to an hour early. An endpoint without bins (traffic from before they
// were counted) keeps none.
func addPercentiles(db *gorm.DB, apiKeyID uint, from time.Time, stats []EndpointStats) error {
	if len(stats) == 0 {
		return nil
	}
	paths := make([]string, len(stats))
	for i, s := range stats {
		paths[i] = s.Path
	}

	query := `SELECT path, bin, SUM(count) AS count FROM endpoint_durations WHERE path IN ? AND hour >= ?`
	args := []any{paths, from.UTC().Truncate(time.Hour)}
	if apiKeyID > 0 {
		query += " AND api_key_id = ?"
		args = append(args, apiKeyID)
	}
	query += " GROUP BY path, bin ORDER BY path, bin"
	var rows []struct {
		Path  string
		Bin   int
		Count int64
	}
	if err := db.Raw(query, args...).Scan(&rows).Error; err != nil {
		return err
	}

	bins := map[string][]binCount{}
	for _, r := range rows {
		bins[r.Path] = append(bins[r.Path], binCount{r.Bin, r.Count})
	}
	for i := range stats {
		if b := bins[stats[i].Path]; len(b) > 0 {
			p95, p99 := percentile(b, 0.95), percentile(b, 0.99)
			stats[i].P95Duration, stats[i].P99Duration = &p95, &p99
		}
	}
	return nil
}
