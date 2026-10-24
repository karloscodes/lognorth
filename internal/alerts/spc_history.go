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
	Rate       float64 // Pooled error rate over the history, without its outliers, in %
	Dispersion float64 // How much more 5-minute buckets vary than binomial (≥ 1)
	Buckets    int     // History buckets with at least MinRequestsForErrorRate requests
	Requests   int64   // Requests in the history

	// The drift check compares the last hour with the same hour (the 12
	// buckets that end at the same time of day) on each of the last
	// ErrorRateHistoryDays days.
	HourRate       float64 // Pooled error rate of those hours, without outliers, in %
	HourDispersion float64 // How much more those hours vary than binomial (≥ 1)
	HourBlocks     int     // Those hours with at least MinRequestsForErrorRate requests
}

// Ready reports whether there is enough history for the spike check.
func (b ErrorRateBaseline) Ready() bool {
	return b.Buckets >= MinHistoryBuckets
}

// DriftReady reports whether there is enough history for the drift check,
// which pools an hour, so it runs on endpoints too small for the spike check.
func (b ErrorRateBaseline) DriftReady() bool {
	return b.HourBlocks >= MinHistoryHourBlocks
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

	rate := robustRate(stats)
	baseline := ErrorRateBaseline{
		Rate:       rate,
		Dispersion: dispersion(stats, rate),
		Requests:   sumStats(stats).Total,
	}
	for _, s := range stats {
		if s.Total >= MinRequestsForErrorRate {
			baseline.Buckets++
		}
	}

	hours, err := sameHourOnEarlierDays(db, apiKeyID, path, at)
	if err != nil {
		return ErrorRateBaseline{}, err
	}
	baseline.HourRate = robustRate(hours)
	baseline.HourDispersion = dispersion(hours, baseline.HourRate)
	for _, h := range hours {
		if h.Total >= MinRequestsForErrorRate {
			baseline.HourBlocks++
		}
	}
	return baseline, nil
}

// sameHourOnEarlierDays returns, for each of the last ErrorRateHistoryDays
// days, the pooled stats of the 12 buckets that end at the time of day of
// `at`: the hour the drift check compares the last hour with. A window
// centered on `at` would hold half of the hour after it instead, and an
// endpoint that always fails more before 05:00 would drift every morning.
func sameHourOnEarlierDays(db *gorm.DB, apiKeyID uint, path string, at time.Time) ([]events.EndpointStat, error) {
	const day = 24 * time.Hour
	var ranges []string
	var args []any
	for i := 1; i <= ErrorRateHistoryDays; i++ {
		end := at.Add(-time.Duration(i) * day)
		ranges = append(ranges, "bucket BETWEEN ? AND ?")
		args = append(args, end.Add(-55*time.Minute), end)
	}
	var stats []events.EndpointStat
	err := db.Where("api_key_id = ? AND path = ?", apiKeyID, path).
		Where("("+strings.Join(ranges, " OR ")+")", args...).
		Find(&stats).Error
	if err != nil {
		return nil, err
	}

	byDay := map[int]events.EndpointStat{}
	for _, s := range stats {
		i := int(at.Sub(s.Bucket) / day)
		h := byDay[i]
		h.Total += s.Total
		h.Errors += s.Errors
		byDay[i] = h
	}
	hours := make([]events.EndpointStat, 0, len(byDay))
	for _, h := range byDay {
		hours = append(hours, h)
	}
	return hours, nil
}

// robustRate is the pooled error rate of the history without its outliers.
// One outage yesterday at this hour would otherwise raise the normal rate
// for a week, and its repeat would only reach the digest. The dispersion is
// already robust (it uses the median), so it judges which buckets are out.
func robustRate(stats []events.EndpointStat) float64 {
	rate := sumStats(stats).ErrorRate()
	spread := dispersion(stats, rate)
	var kept []events.EndpointStat
	for _, s := range stats {
		if s.Total >= MinRequestsForErrorRate && (s.ErrorRate()-rate)/(binomialStdDev(rate, s.Total)*spread) >= CriticalSigma {
			continue
		}
		kept = append(kept, s)
	}
	return sumStats(kept).ErrorRate()
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

// lastBuckets returns the pooled stats of the n buckets that end with the
// bucket at `at`: 12 is the last hour, 3 the last 15 minutes.
func lastBuckets(db *gorm.DB, apiKeyID uint, path string, at time.Time, n int) (events.EndpointStat, error) {
	var stats []events.EndpointStat
	err := db.Where("api_key_id = ? AND path = ? AND bucket BETWEEN ? AND ?",
		apiKeyID, path, at.Add(-time.Duration(n-1)*5*time.Minute), at).Find(&stats).Error
	return sumStats(stats), err
}

// lastHourWithoutSpikes returns the pooled stats of the last hour without
// the buckets that alone are critical. The drift check reads it: a spike
// already alerts on its own, and once it is over, it would keep the last
// hour high for up to an hour while the endpoint is fine again.
func lastHourWithoutSpikes(db *gorm.DB, apiKeyID uint, path string, at time.Time, baseline ErrorRateBaseline) (events.EndpointStat, error) {
	var stats []events.EndpointStat
	err := db.Where("api_key_id = ? AND path = ? AND bucket BETWEEN ? AND ?",
		apiKeyID, path, at.Add(-55*time.Minute), at).Find(&stats).Error
	if err != nil {
		return events.EndpointStat{}, err
	}
	var kept []events.EndpointStat
	for _, s := range stats {
		if !aloneCritical(s, baseline) {
			kept = append(kept, s)
		}
	}
	return sumStats(kept), nil
}

// aloneCritical reports whether one bucket by itself is far past the normal
// rate: 5σ, and past the floor a critical rate needs. Unlike the spike check
// it judges a small bucket too, with the binomial floor of one error.
func aloneCritical(s events.EndpointStat, baseline ErrorRateBaseline) bool {
	if s.Total == 0 {
		return false
	}
	rate := s.ErrorRate()
	z := (rate - baseline.Rate) / (binomialStdDev(baseline.Rate, s.Total) * max(1, baseline.Dispersion))
	return z >= CriticalSigma && rate >= max(CriticalMinRatio*baseline.Rate, baseline.Rate+CriticalMinPoints)
}

// GetLastHour returns the pooled stats of the 12 buckets that end with `at`.
func GetLastHour(db *gorm.DB, apiKeyID uint, path string, at time.Time) (events.EndpointStat, error) {
	return lastBuckets(db, apiKeyID, path, at, 12)
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
