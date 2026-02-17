package events

// SPCIsSpike checks if current error rate is a statistical spike using z-score.
// Returns (isSpike, severity) where severity is "warning", "critical", or "".
func SPCIsSpike(current, mean, stddev float64) (bool, string) {
	if stddev == 0 {
		stddev = 1.0 // Prevent division by zero
	}

	zScore := (current - mean) / stddev

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
func SPCIsRecovered(current, mean, stddev float64) bool {
	if stddev == 0 {
		stddev = 1.0
	}
	threshold := mean + RecoverySigma*stddev
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
