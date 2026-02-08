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
