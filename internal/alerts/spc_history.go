package alerts

import (
	"lognorth/internal/events"
	"math"
	"slices"
	"strings"
	"time"

	"gorm.io/gorm"
)

// Baselines come from the stored buckets in endpoint_stats. Nothing is learned
// or saved, so an anomaly cannot leak into a baseline, and a lasting change
// becomes the new normal once it fills the history.

// ErrorRateBaseline is the normal error rate of an endpoint at a time of day.
type ErrorRateBaseline struct {
	Rate           float64 // Pooled error rate over the history, in %
	Dispersion     float64 // How much more 5-minute buckets vary than binomial (≥ 1)
	HourDispersion float64 // The same for 1-hour blocks, for the drift check
	Buckets        int     // History buckets with at least MinRequestsForErrorRate requests
}

// Ready reports whether there is enough history to check against.
func (b ErrorRateBaseline) Ready() bool {
	return b.Buckets >= MinHistoryBuckets
}

// VolumeBaseline is the normal request count of an endpoint at a time of week.
type VolumeBaseline struct {
	Median  float64
	MAD     float64 // Median absolute deviation
	Buckets int
}

// Ready reports whether there is enough history to check against.
func (b VolumeBaseline) Ready() bool {
	return b.Buckets >= MinHistoryBuckets
}

// GetErrorRateBaseline compares against the same time of day on the last
// ErrorRateHistoryDays days.
func GetErrorRateBaseline(db *gorm.DB, apiKeyID uint, path string, at time.Time) (ErrorRateBaseline, error) {
	stats, err := historyBuckets(db, apiKeyID, path, at, 24*time.Hour, ErrorRateHistoryDays)
	if err != nil {
		return ErrorRateBaseline{}, err
	}

	rate := sumStats(stats).ErrorRate()
	baseline := ErrorRateBaseline{
		Rate:           rate,
		Dispersion:     dispersion(stats, rate),
		HourDispersion: dispersion(hourBlocks(stats), rate),
	}
	for _, s := range stats {
		if s.Total >= MinRequestsForErrorRate {
			baseline.Buckets++
		}
	}
	return baseline, nil
}

// GetVolumeBaseline compares against the same time of week on the last
// VolumeHistoryWeeks weeks. Traffic depends on the day of the week.
func GetVolumeBaseline(db *gorm.DB, apiKeyID uint, path string, at time.Time) (VolumeBaseline, error) {
	const week = 7 * 24 * time.Hour
	stats, err := historyBuckets(db, apiKeyID, path, at, week, VolumeHistoryWeeks)
	if err != nil {
		return VolumeBaseline{}, err
	}

	// A bucket with no requests has no row. Count it as 0 in each week with data.
	rowsPerWeek := map[time.Duration]int{}
	var volumes []float64
	for _, s := range stats {
		rowsPerWeek[at.Sub(s.Bucket).Round(week)]++
		volumes = append(volumes, float64(s.Total))
	}
	bucketsPerWeek := int(2*HistoryWindow/(5*time.Minute)) + 1
	for _, rows := range rowsPerWeek {
		for range bucketsPerWeek - rows {
			volumes = append(volumes, 0)
		}
	}

	if len(volumes) == 0 {
		return VolumeBaseline{}, nil
	}
	middle := median(volumes)
	deviations := make([]float64, len(volumes))
	for i, v := range volumes {
		deviations[i] = math.Abs(v - middle)
	}
	return VolumeBaseline{Median: middle, MAD: median(deviations), Buckets: len(volumes)}, nil
}

// GetLastHour returns the pooled stats of the 12 buckets that end with `at`.
func GetLastHour(db *gorm.DB, apiKeyID uint, path string, at time.Time) (events.EndpointStat, error) {
	var stats []events.EndpointStat
	err := db.Where("api_key_id = ? AND path = ? AND bucket BETWEEN ? AND ?",
		apiKeyID, path, at.Add(-55*time.Minute), at).Find(&stats).Error
	return sumStats(stats), err
}

// historyBuckets returns the buckets within HistoryWindow of `at`, one window
// per step back in time, `count` steps.
func historyBuckets(db *gorm.DB, apiKeyID uint, path string, at time.Time, step time.Duration, count int) ([]events.EndpointStat, error) {
	var ranges []string
	var args []any
	for i := 1; i <= count; i++ {
		center := at.Add(-time.Duration(i) * step)
		ranges = append(ranges, "bucket BETWEEN ? AND ?")
		args = append(args, center.Add(-HistoryWindow), center.Add(HistoryWindow))
	}

	var stats []events.EndpointStat
	err := db.Where("api_key_id = ? AND path = ?", apiKeyID, path).
		Where("("+strings.Join(ranges, " OR ")+")", args...).
		Find(&stats).Error
	return stats, err
}

// dispersion returns how much the samples vary, in binomial stddevs around
// `rate` (Laney's σz). It uses the median absolute deviation of the z-scores,
// so one past outage in the history does not hide the next one. The result is
// at least 1.
func dispersion(samples []events.EndpointStat, rate float64) float64 {
	var zs []float64
	for _, s := range samples {
		if s.Total < MinRequestsForErrorRate {
			continue
		}
		zs = append(zs, (s.ErrorRate()-rate)/binomialStdDev(rate, s.Total))
	}
	if len(zs) == 0 {
		return 1
	}

	middle := median(zs)
	for i, z := range zs {
		zs[i] = math.Abs(z - middle)
	}
	// 1.4826 × MAD estimates the stddev.
	return max(1, 1.4826*median(zs))
}

// hourBlocks pools buckets into clock hours.
func hourBlocks(stats []events.EndpointStat) []events.EndpointStat {
	byHour := map[time.Time]events.EndpointStat{}
	for _, s := range stats {
		hour := s.Bucket.Truncate(time.Hour)
		block := byHour[hour]
		block.Total += s.Total
		block.Errors += s.Errors
		byHour[hour] = block
	}

	blocks := make([]events.EndpointStat, 0, len(byHour))
	for _, block := range byHour {
		blocks = append(blocks, block)
	}
	return blocks
}

func sumStats(stats []events.EndpointStat) events.EndpointStat {
	var sum events.EndpointStat
	for _, s := range stats {
		sum.Total += s.Total
		sum.Errors += s.Errors
	}
	return sum
}

func median(values []float64) float64 {
	values = slices.Clone(values)
	slices.Sort(values)
	mid := len(values) / 2
	if len(values)%2 == 0 {
		return (values[mid-1] + values[mid]) / 2
	}
	return values[mid]
}
