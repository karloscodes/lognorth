package tests

import (
	"testing"
	"time"

	"lognorth/internal/events"
)

// =============================================================================
// ALERTING DETECTION RULES TESTS
// Test the pure detection functions in isolation
// =============================================================================

func TestIsSpike(t *testing.T) {
	threshold := 10 // Default threshold when no baseline

	t.Run("no baseline, above threshold is spike", func(t *testing.T) {
		if !events.IsSpike(15.0, 0, threshold) {
			t.Error("15% with no baseline should be a spike (threshold 10%)")
		}
	})

	t.Run("no baseline, below threshold is not spike", func(t *testing.T) {
		if events.IsSpike(5.0, 0, threshold) {
			t.Error("5% with no baseline should not be a spike (threshold 10%)")
		}
	})

	t.Run("with baseline, 3x multiplier triggers spike", func(t *testing.T) {
		// Baseline 5%, current 15% = 3x baseline
		if !events.IsSpike(15.0, 5.0, threshold) {
			t.Error("15% on 5% baseline should be a spike (3x)")
		}
	})

	t.Run("with baseline, absolute jump triggers spike", func(t *testing.T) {
		// Baseline 5%, current 21% = baseline + 16% (>15% jump)
		if !events.IsSpike(21.0, 5.0, threshold) {
			t.Error("21% on 5% baseline should be a spike (+16% absolute)")
		}
	})

	t.Run("with baseline, normal fluctuation is not spike", func(t *testing.T) {
		// Baseline 5%, current 10% = 2x (below 3x), +5% (below 15% jump)
		if events.IsSpike(10.0, 5.0, threshold) {
			t.Error("10% on 5% baseline should not be a spike")
		}
	})

	t.Run("exact 3x multiplier is spike", func(t *testing.T) {
		// Baseline 5%, current 15% = exactly 3x
		if !events.IsSpike(15.0, 5.0, threshold) {
			t.Error("exact 3x baseline should be a spike")
		}
	})
}

func TestIsRecovered(t *testing.T) {
	t.Run("no baseline, below 5% is recovered", func(t *testing.T) {
		if !events.IsRecovered(4.0, 0) {
			t.Error("4% with no baseline should be recovered")
		}
	})

	t.Run("no baseline, at 5% is not recovered", func(t *testing.T) {
		if events.IsRecovered(5.0, 0) {
			t.Error("5% with no baseline should not be recovered (at threshold)")
		}
	})

	t.Run("with baseline, within margin is recovered", func(t *testing.T) {
		// Baseline 5%, recovery margin 5%, so 10% or below is recovered
		if !events.IsRecovered(10.0, 5.0) {
			t.Error("10% on 5% baseline should be recovered (within 5% margin)")
		}
	})

	t.Run("with baseline, above margin is not recovered", func(t *testing.T) {
		// Baseline 5%, recovery margin 5%, so above 10% is not recovered
		if events.IsRecovered(11.0, 5.0) {
			t.Error("11% on 5% baseline should not be recovered")
		}
	})

	t.Run("exactly at baseline is recovered", func(t *testing.T) {
		if !events.IsRecovered(5.0, 5.0) {
			t.Error("exactly at baseline should be recovered")
		}
	})
}

func TestIsRegression(t *testing.T) {
	t.Run("error quiet for 25 hours is regression", func(t *testing.T) {
		lastOccurrence := time.Now().Add(-25 * time.Hour)

		if !events.IsRegression(lastOccurrence) {
			t.Error("error quiet for 25h should be a regression")
		}
	})

	t.Run("error quiet for 23 hours is not regression", func(t *testing.T) {
		lastOccurrence := time.Now().Add(-23 * time.Hour)

		if events.IsRegression(lastOccurrence) {
			t.Error("error quiet for 23h should not be a regression (need 24h)")
		}
	})

	t.Run("error quiet for exactly 24 hours is regression", func(t *testing.T) {
		lastOccurrence := time.Now().Add(-24 * time.Hour)

		if !events.IsRegression(lastOccurrence) {
			t.Error("error quiet for exactly 24h should be a regression")
		}
	})

	t.Run("recent error is not regression", func(t *testing.T) {
		lastOccurrence := time.Now().Add(-1 * time.Hour)

		if events.IsRegression(lastOccurrence) {
			t.Error("recent error (1h ago) should not be a regression")
		}
	})
}

// =============================================================================
// SILENCE DETECTION TESTS
// Test the absence-of-logs detection functions
// =============================================================================

func TestIsSilence(t *testing.T) {
	t.Run("detects critical silence at -3 sigma", func(t *testing.T) {
		// Mean 100, stddev 10, current 65 → z = (65-100)/10 = -3.5
		silent, severity := events.IsSilence(65, 100, 10)

		if !silent {
			t.Error("expected silence to be detected")
		}
		if severity != "critical" {
			t.Errorf("expected critical, got %s", severity)
		}
	})

	t.Run("detects warning at -2 sigma", func(t *testing.T) {
		// Mean 100, stddev 10, current 78 → z = (78-100)/10 = -2.2
		silent, severity := events.IsSilence(78, 100, 10)

		if !silent {
			t.Error("expected silence to be detected")
		}
		if severity != "warning" {
			t.Errorf("expected warning, got %s", severity)
		}
	})

	t.Run("ignores low-traffic endpoints", func(t *testing.T) {
		// Mean 30 (below MinVolumeForSilenceAlert=50), should not alert
		silent, _ := events.IsSilence(0, 30, 10)

		if silent {
			t.Error("should not alert on low-traffic endpoints")
		}
	})

	t.Run("normal traffic is not silence", func(t *testing.T) {
		// Mean 100, stddev 10, current 95 → z = -0.5
		silent, _ := events.IsSilence(95, 100, 10)

		if silent {
			t.Error("normal traffic should not be detected as silence")
		}
	})

	t.Run("handles zero stddev with default coefficient of variation", func(t *testing.T) {
		// Mean 100, stddev 0 → uses mean*0.3=30 as stddev
		// Current 30 → z = (30-100)/30 = -2.33 → warning
		silent, severity := events.IsSilence(30, 100, 0)

		if !silent {
			t.Error("expected silence with zero stddev fallback")
		}
		if severity != "warning" {
			t.Errorf("expected warning, got %s", severity)
		}
	})
}

func TestIsSilenceRecovered(t *testing.T) {
	t.Run("returns true when volume back to normal", func(t *testing.T) {
		// Mean 100, stddev 10, recovery threshold = 100 - 1*10 = 90
		// Current 95 is above 90
		if !events.IsSilenceRecovered(95, 100, 10) {
			t.Error("expected recovery when volume is back to normal")
		}
	})

	t.Run("returns false when volume still low", func(t *testing.T) {
		// Mean 100, stddev 10, recovery threshold = 90
		// Current 85 is below 90
		if events.IsSilenceRecovered(85, 100, 10) {
			t.Error("expected no recovery when volume still low")
		}
	})

	t.Run("handles zero stddev", func(t *testing.T) {
		// Mean 100, stddev 0 → uses mean*0.3=30
		// Recovery threshold = 100 - 1*30 = 70
		if !events.IsSilenceRecovered(75, 100, 0) {
			t.Error("expected recovery with zero stddev fallback")
		}
	})
}
