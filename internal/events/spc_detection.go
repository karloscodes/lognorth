package events

import "math"

// errorRateStdDev returns the stddev to compare an error rate against.
// The learned stddev shrinks toward 0 on an endpoint that never fails. The
// binomial stddev for `total` requests is the floor (p-chart), so one error in
// 1,000 requests is not a 50σ spike. The expected rate is at least 1 error per bucket.
func errorRateStdDev(mean, stddev float64, total int64) float64 {
	n := float64(total)
	p := max(mean/100, 1/n)
	binomial := math.Sqrt(p*(1-p)/n) * 100
	return max(stddev, binomial)
}

// SPCIsSpike checks if current error rate is a statistical spike using z-score.
// Returns (isSpike, severity) where severity is "warning", "critical", or "".
func SPCIsSpike(current, mean, stddev float64, total int64) (bool, string) {
	if total < MinRequestsForErrorRate {
		return false, ""
	}

	zScore := (current - mean) / errorRateStdDev(mean, stddev, total)

	if zScore >= CriticalSigma {
		return true, "critical"
	}
	if zScore >= WarningSigma {
		return true, "warning"
	}
	return false, ""
}

// UpdateCUSUM updates the cumulative sum and checks for drift.
// Returns (detected, severity) where severity is "warning", "critical", or "".
func UpdateCUSUM(state *CUSUMState, current, mean float64) (bool, string) {
	deviation := current - mean - CUSUMSlack
	state.Cumulative = max(0, state.Cumulative+deviation)

	if state.Cumulative >= CUSUMCriticalThreshold {
		return true, "critical"
	}
	if state.Cumulative >= CUSUMWarningThreshold {
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

// SPCIsRecovered checks if error rate is back to normal using statistical threshold.
func SPCIsRecovered(current, mean, stddev float64, total int64) bool {
	if total < MinRequestsForErrorRate {
		return false
	}
	threshold := mean + RecoverySigma*errorRateStdDev(mean, stddev, total)
	return current <= threshold
}

// IsSilence checks if current volume is abnormally low.
// Returns (isSilent, severity) where severity is "warning", "critical", or "".
func IsSilence(currentVolume, mean, stddev float64) (bool, string) {
	if mean < MinVolumeForSilenceAlert {
		return false, "" // Low-traffic endpoint, skip
	}
	if stddev == 0 {
		stddev = mean * 0.3 // Default to 30% coefficient of variation
	}

	zScore := (currentVolume - mean) / stddev

	if zScore <= SilenceCriticalSigma {
		return true, "critical"
	}
	if zScore <= SilenceWarningSigma {
		return true, "warning"
	}
	return false, ""
}

// IsSilenceRecovered checks if volume is back to normal.
func IsSilenceRecovered(currentVolume, mean, stddev float64) bool {
	if stddev == 0 {
		stddev = mean * 0.3
	}
	threshold := mean - RecoverySigma*stddev
	return currentVolume >= threshold
}
