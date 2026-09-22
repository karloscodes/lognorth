package events

import "math"

// binomialStdDev returns the stddev of an error rate (in %) over `total`
// requests: the p-chart limit. The rate is at least 1 error per bucket, so an
// endpoint that never fails does not treat one error as a 50σ spike.
func binomialStdDev(rate float64, total int64) float64 {
	n := float64(total)
	p := max(rate/100, 1/n)
	return math.Sqrt(p*(1-p)/n) * 100
}

// SPCIsSpike checks the last 5-minute bucket against the baseline.
// Returns (isSpike, severity) where severity is "warning", "critical", or "".
func SPCIsSpike(current float64, total int64, baseline ErrorRateBaseline) (bool, string) {
	return errorRateSeverity(current, total, baseline.Rate, baseline.Dispersion, baseline.Ready())
}

// SPCIsDrift checks the pooled last hour against the baseline. With 12 times
// the requests of one bucket, it catches a slow rise the spike check misses.
func SPCIsDrift(current float64, total int64, baseline ErrorRateBaseline) (bool, string) {
	return errorRateSeverity(current, total, baseline.Rate, baseline.HourDispersion, baseline.Ready())
}

func errorRateSeverity(current float64, total int64, rate, dispersion float64, ready bool) (bool, string) {
	if !ready || total < MinRequestsForErrorRate {
		return false, ""
	}

	zScore := (current - rate) / (binomialStdDev(rate, total) * dispersion)
	return severityAbove(zScore)
}

// IsSilence checks if the current volume is abnormally low.
// Returns (isSilent, severity) where severity is "warning", "critical", or "".
func IsSilence(currentVolume float64, baseline VolumeBaseline) (bool, string) {
	if !baseline.Ready() || baseline.Median < MinVolumeForSilenceAlert {
		return false, "" // Low-traffic endpoint, skip
	}

	// 1.4826 × MAD estimates the stddev. Counts vary at least as much as
	// Poisson (√median), even when the history is flat.
	stddev := max(1.4826*baseline.MAD, math.Sqrt(baseline.Median))
	zScore := (baseline.Median - currentVolume) / stddev
	return severityAbove(zScore)
}

func severityAbove(zScore float64) (bool, string) {
	if zScore >= CriticalSigma {
		return true, "critical"
	}
	if zScore >= WarningSigma {
		return true, "warning"
	}
	return false, ""
}

// MaxSeverity returns the higher severity level.
func MaxSeverity(a, b string) string {
	if a == "critical" || b == "critical" {
		return "critical"
	}
	if a == "warning" || b == "warning" {
		return "warning"
	}
	return ""
}
