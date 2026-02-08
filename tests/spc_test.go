package tests

import (
	"testing"
	"time"

	"lognorth/internal/events"
)

func TestSPCIsSpike(t *testing.T) {
	t.Run("returns critical when above 3 sigma", func(t *testing.T) {
		isSpike, severity := events.SPCIsSpike(10.0, 2.0, 2.0) // z = (10-2)/2 = 4

		if !isSpike {
			t.Error("expected spike to be detected")
		}
		if severity != "critical" {
			t.Errorf("expected critical, got %s", severity)
		}
	})

	t.Run("returns warning when between 2 and 3 sigma", func(t *testing.T) {
		isSpike, severity := events.SPCIsSpike(6.5, 2.0, 2.0) // z = (6.5-2)/2 = 2.25

		if !isSpike {
			t.Error("expected spike to be detected")
		}
		if severity != "warning" {
			t.Errorf("expected warning, got %s", severity)
		}
	})

	t.Run("returns false when within normal range", func(t *testing.T) {
		isSpike, _ := events.SPCIsSpike(3.0, 2.0, 2.0) // z = (3-2)/2 = 0.5

		if isSpike {
			t.Error("expected no spike")
		}
	})

	t.Run("handles zero stddev", func(t *testing.T) {
		isSpike, severity := events.SPCIsSpike(5.0, 2.0, 0.0) // Should use 1.0

		if !isSpike {
			t.Error("expected spike with zero stddev fallback")
		}
		if severity != "critical" {
			t.Errorf("expected critical, got %s", severity)
		}
	})
}

func TestUpdateCUSUM(t *testing.T) {
	t.Run("accumulates positive deviation", func(t *testing.T) {
		state := &events.CUSUMState{Cumulative: 0}

		events.UpdateCUSUM(state, 3.0, 2.0) // deviation = 3 - 2 - 0.5 = 0.5

		if state.Cumulative != 0.5 {
			t.Errorf("expected 0.5, got %f", state.Cumulative)
		}
	})

	t.Run("does not go below zero", func(t *testing.T) {
		state := &events.CUSUMState{Cumulative: 0}

		events.UpdateCUSUM(state, 1.0, 2.0) // deviation = 1 - 2 - 0.5 = -1.5

		if state.Cumulative != 0 {
			t.Errorf("expected 0, got %f", state.Cumulative)
		}
	})

	t.Run("returns critical when threshold exceeded", func(t *testing.T) {
		state := &events.CUSUMState{Cumulative: 4.6}

		detected, severity := events.UpdateCUSUM(state, 3.0, 2.0) // adds 0.5

		if !detected {
			t.Error("expected drift detected")
		}
		if severity != "critical" {
			t.Errorf("expected critical, got %s", severity)
		}
	})

	t.Run("returns warning when above warning threshold", func(t *testing.T) {
		state := &events.CUSUMState{Cumulative: 2.1}

		detected, severity := events.UpdateCUSUM(state, 3.0, 2.0) // adds 0.5 = 2.6

		if !detected {
			t.Error("expected drift detected")
		}
		if severity != "warning" {
			t.Errorf("expected warning, got %s", severity)
		}
	})
}

func TestHourOfWeek(t *testing.T) {
	t.Run("monday midnight is 0", func(t *testing.T) {
		// 2026-02-09 is a Monday
		monday := time.Date(2026, 2, 9, 0, 0, 0, 0, time.UTC)

		if got := events.HourOfWeek(monday); got != 0 {
			t.Errorf("expected 0, got %d", got)
		}
	})

	t.Run("sunday 23:00 is 167", func(t *testing.T) {
		// 2026-02-15 is a Sunday
		sunday := time.Date(2026, 2, 15, 23, 0, 0, 0, time.UTC)

		if got := events.HourOfWeek(sunday); got != 167 {
			t.Errorf("expected 167, got %d", got)
		}
	})
}

func TestMaxSeverity(t *testing.T) {
	tests := []struct {
		a, b     string
		expected string
	}{
		{"critical", "warning", "critical"},
		{"warning", "critical", "critical"},
		{"warning", "warning", "warning"},
		{"", "warning", "warning"},
		{"warning", "", "warning"},
		{"", "", ""},
	}

	for _, tt := range tests {
		got := events.MaxSeverity(tt.a, tt.b)
		if got != tt.expected {
			t.Errorf("MaxSeverity(%q, %q) = %q, want %q", tt.a, tt.b, got, tt.expected)
		}
	}
}

func TestSPCIsRecovered(t *testing.T) {
	t.Run("returns true when below threshold", func(t *testing.T) {
		// threshold = mean + 1*stddev = 2 + 1*1 = 3
		if !events.SPCIsRecovered(2.5, 2.0, 1.0) {
			t.Error("expected recovered")
		}
	})

	t.Run("returns false when above threshold", func(t *testing.T) {
		// threshold = mean + 1*stddev = 2 + 1*1 = 3
		if events.SPCIsRecovered(4.0, 2.0, 1.0) {
			t.Error("expected not recovered")
		}
	})
}

func TestGetEffectiveBaseline(t *testing.T) {
	t.Run("returns defaults for cold start", func(t *testing.T) {
		baseline := &events.BaselineStat{
			Mean:        5.0,
			StdDev:      1.0,
			SampleCount: 10, // Below MinSamplesForBaseline (20)
		}

		mean, stddev := events.GetEffectiveBaseline(baseline)

		if mean != events.DefaultMean {
			t.Errorf("expected default mean %f, got %f", events.DefaultMean, mean)
		}
		if stddev != events.DefaultStdDev {
			t.Errorf("expected default stddev %f, got %f", events.DefaultStdDev, stddev)
		}
	})

	t.Run("returns learned values after enough samples", func(t *testing.T) {
		baseline := &events.BaselineStat{
			Mean:        5.0,
			StdDev:      1.5,
			SampleCount: 25,
		}

		mean, stddev := events.GetEffectiveBaseline(baseline)

		if mean != 5.0 {
			t.Errorf("expected 5.0, got %f", mean)
		}
		if stddev != 1.5 {
			t.Errorf("expected 1.5, got %f", stddev)
		}
	})
}

// =============================================================================
// SCENARIO TESTS: Small Data (Cold Start) vs Medium Data (Learned Baseline)
// =============================================================================

// TestSmallData_ColdStart tests detection behavior with insufficient samples.
// This simulates a new app with only a few days of data.
func TestSmallData_ColdStart(t *testing.T) {
	// Scenario: New app, only 5 samples collected so far
	// System should use conservative defaults (mean=2%, stddev=2%)

	t.Run("uses conservative defaults with 5 samples", func(t *testing.T) {
		baseline := &events.BaselineStat{
			Mean:        10.0, // App's actual mean is 10%, but we don't trust it yet
			StdDev:      1.0,
			SampleCount: 5,
		}

		mean, stddev := events.GetEffectiveBaseline(baseline)

		// Should use defaults, not the learned values
		if mean != 2.0 {
			t.Errorf("cold start should use default mean 2.0, got %f", mean)
		}
		if stddev != 2.0 {
			t.Errorf("cold start should use default stddev 2.0, got %f", stddev)
		}
	})

	t.Run("8% error rate triggers critical during cold start", func(t *testing.T) {
		// With defaults (mean=2, stddev=2), 8% gives z-score = (8-2)/2 = 3
		// This should be critical (> 3 sigma)
		baseline := &events.BaselineStat{SampleCount: 5}
		mean, stddev := events.GetEffectiveBaseline(baseline)

		isSpike, severity := events.SPCIsSpike(8.0, mean, stddev)

		if !isSpike || severity != "critical" {
			t.Errorf("8%% error rate during cold start should be critical, got spike=%v severity=%s", isSpike, severity)
		}
	})

	t.Run("5% error rate triggers warning during cold start", func(t *testing.T) {
		// With defaults (mean=2, stddev=2), 5% gives z-score = (5-2)/2 = 1.5
		// This is below warning threshold (2 sigma), so no alert
		baseline := &events.BaselineStat{SampleCount: 5}
		mean, stddev := events.GetEffectiveBaseline(baseline)

		isSpike, _ := events.SPCIsSpike(5.0, mean, stddev)

		if isSpike {
			t.Error("5% error rate during cold start should NOT trigger spike (z=1.5 < 2)")
		}
	})

	t.Run("6% error rate triggers warning during cold start", func(t *testing.T) {
		// With defaults (mean=2, stddev=2), 6% gives z-score = (6-2)/2 = 2
		// This is exactly at warning threshold
		baseline := &events.BaselineStat{SampleCount: 5}
		mean, stddev := events.GetEffectiveBaseline(baseline)

		isSpike, severity := events.SPCIsSpike(6.1, mean, stddev)

		if !isSpike || severity != "warning" {
			t.Errorf("6.1%% error rate during cold start should be warning, got spike=%v severity=%s", isSpike, severity)
		}
	})
}

// TestMediumData_LearnedBaseline tests detection with enough samples to trust the baseline.
// This simulates an app with ~1 month of data (20+ samples per hour bucket).
func TestMediumData_LearnedBaseline(t *testing.T) {
	// Scenario: App with 50 samples - enough to trust the learned baseline
	// App normally runs at 5% error rate with stddev of 1%

	t.Run("uses learned values with 50 samples", func(t *testing.T) {
		baseline := &events.BaselineStat{
			Mean:        5.0,
			StdDev:      1.0,
			SampleCount: 50,
		}

		mean, stddev := events.GetEffectiveBaseline(baseline)

		if mean != 5.0 {
			t.Errorf("should use learned mean 5.0, got %f", mean)
		}
		if stddev != 1.0 {
			t.Errorf("should use learned stddev 1.0, got %f", stddev)
		}
	})

	t.Run("8% is critical for low-error app", func(t *testing.T) {
		// App normally at 5% ± 1%
		// 8% gives z-score = (8-5)/1 = 3, exactly at critical threshold
		baseline := &events.BaselineStat{
			Mean:        5.0,
			StdDev:      1.0,
			SampleCount: 50,
		}
		mean, stddev := events.GetEffectiveBaseline(baseline)

		isSpike, severity := events.SPCIsSpike(8.1, mean, stddev)

		if !isSpike || severity != "critical" {
			t.Errorf("8.1%% for 5%%±1%% app should be critical, got spike=%v severity=%s", isSpike, severity)
		}
	})

	t.Run("7% is warning for low-error app", func(t *testing.T) {
		// App normally at 5% ± 1%
		// 7% gives z-score = (7-5)/1 = 2, at warning threshold
		baseline := &events.BaselineStat{
			Mean:        5.0,
			StdDev:      1.0,
			SampleCount: 50,
		}
		mean, stddev := events.GetEffectiveBaseline(baseline)

		isSpike, severity := events.SPCIsSpike(7.1, mean, stddev)

		if !isSpike || severity != "warning" {
			t.Errorf("7.1%% for 5%%±1%% app should be warning, got spike=%v severity=%s", isSpike, severity)
		}
	})

	t.Run("6% is normal for low-error app", func(t *testing.T) {
		// App normally at 5% ± 1%
		// 6% gives z-score = (6-5)/1 = 1, within normal range
		baseline := &events.BaselineStat{
			Mean:        5.0,
			StdDev:      1.0,
			SampleCount: 50,
		}
		mean, stddev := events.GetEffectiveBaseline(baseline)

		isSpike, _ := events.SPCIsSpike(6.0, mean, stddev)

		if isSpike {
			t.Error("6% for 5%±1% app should be normal (z=1)")
		}
	})
}

// TestHighVarianceApp tests an app with naturally high variance.
// Some apps have variable error rates - SPC should adapt.
func TestHighVarianceApp(t *testing.T) {
	// Scenario: App with mean 10% and stddev 3% (high variance)
	// This app naturally fluctuates between 4% and 16%

	t.Run("16% is normal for high-variance app", func(t *testing.T) {
		// 10% ± 3%, so 16% is z-score = (16-10)/3 = 2, at warning threshold
		baseline := &events.BaselineStat{
			Mean:        10.0,
			StdDev:      3.0,
			SampleCount: 50,
		}
		mean, stddev := events.GetEffectiveBaseline(baseline)

		isSpike, _ := events.SPCIsSpike(15.9, mean, stddev)

		if isSpike {
			t.Error("15.9% for 10%±3% app should be normal (z<2)")
		}
	})

	t.Run("20% triggers critical for high-variance app", func(t *testing.T) {
		// 10% ± 3%, so 20% is z-score = (20-10)/3 = 3.33, critical
		baseline := &events.BaselineStat{
			Mean:        10.0,
			StdDev:      3.0,
			SampleCount: 50,
		}
		mean, stddev := events.GetEffectiveBaseline(baseline)

		isSpike, severity := events.SPCIsSpike(20.0, mean, stddev)

		if !isSpike || severity != "critical" {
			t.Errorf("20%% for 10%%±3%% app should be critical, got spike=%v severity=%s", isSpike, severity)
		}
	})
}

// TestCUSUMDriftDetection tests gradual drift that individual checks miss.
func TestCUSUMDriftDetection(t *testing.T) {
	// Scenario: Error rate slowly creeping up over multiple checks
	// Each individual reading looks normal, but the trend is concerning

	t.Run("detects slow drift over multiple readings", func(t *testing.T) {
		state := &events.CUSUMState{Cumulative: 0}
		mean := 5.0 // Baseline mean

		// Simulate 10 readings slightly above normal (6% each)
		// deviation per reading = 6 - 5 - 0.5 = 0.5
		for i := 0; i < 10; i++ {
			events.UpdateCUSUM(state, 6.0, mean)
		}

		// After 10 readings: cumulative = 10 * 0.5 = 5.0
		// This should trigger critical (threshold = 5.0)
		if state.Cumulative < events.CUSUMCriticalThreshold {
			t.Errorf("expected cumulative >= %f after drift, got %f",
				events.CUSUMCriticalThreshold, state.Cumulative)
		}
	})

	t.Run("decreases when readings return to normal", func(t *testing.T) {
		state := &events.CUSUMState{Cumulative: 3.0} // Some accumulated drift
		mean := 5.0

		// Reading below mean reduces the accumulator (but doesn't fully clear in one reading)
		events.UpdateCUSUM(state, 4.0, mean) // deviation = 4 - 5 - 0.5 = -1.5, new = max(0, 3.0-1.5) = 1.5

		if state.Cumulative != 1.5 {
			t.Errorf("cumulative should be 1.5 after partial reduction, got %f", state.Cumulative)
		}

		// Multiple below-mean readings eventually reset to 0
		events.UpdateCUSUM(state, 4.0, mean) // 1.5 - 1.5 = 0
		if state.Cumulative != 0 {
			t.Errorf("cumulative should be 0 after full reduction, got %f", state.Cumulative)
		}
	})
}
