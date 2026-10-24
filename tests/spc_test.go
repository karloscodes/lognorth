package tests

import (
	"lognorth/internal/alerts"
	"testing"
)

// A baseline with enough history: 1% errors, buckets vary like binomial.
var onePercent = alerts.ErrorRateBaseline{Rate: 1.0, Dispersion: 1, HourDispersion: 1, Buckets: 91}

func TestSPCIsSpike(t *testing.T) {
	// At 1% over 1,000 requests, one binomial stddev is 0.31%.

	t.Run("returns critical at 5 sigma or more", func(t *testing.T) {
		isSpike, severity := alerts.SPCIsSpike(3.0, 1000, onePercent) // z = 6.4

		if !isSpike || severity != "critical" {
			t.Errorf("expected critical, got spike=%v severity=%q", isSpike, severity)
		}
	})

	t.Run("returns warning between 3 and 5 sigma", func(t *testing.T) {
		isSpike, severity := alerts.SPCIsSpike(2.2, 1000, onePercent) // z = 3.8

		if !isSpike || severity != "warning" {
			t.Errorf("expected warning, got spike=%v severity=%q", isSpike, severity)
		}
	})

	t.Run("returns nothing within normal range", func(t *testing.T) {
		isSpike, _ := alerts.SPCIsSpike(1.5, 1000, onePercent) // z = 1.6

		if isSpike {
			t.Error("expected no spike")
		}
	})

	t.Run("widens the limits for an endpoint that varies more than binomial", func(t *testing.T) {
		noisy := onePercent
		noisy.Dispersion = 3

		isSpike, _ := alerts.SPCIsSpike(3.0, 1000, noisy) // z = 6.4 / 3 = 2.1

		if isSpike {
			t.Error("expected no spike on a noisy endpoint")
		}
	})

	t.Run("ignores buckets with too few requests", func(t *testing.T) {
		isSpike, _ := alerts.SPCIsSpike(50.0, 2, onePercent) // 1 error in 2 requests

		if isSpike {
			t.Error("expected no spike below the minimum request count")
		}
	})

	t.Run("waits for enough history", func(t *testing.T) {
		learning := onePercent
		learning.Buckets = 5

		isSpike, _ := alerts.SPCIsSpike(50.0, 1000, learning)

		if isSpike {
			t.Error("expected no spike without enough history")
		}
	})

	t.Run("does not alert on one error for an endpoint that never fails", func(t *testing.T) {
		neverFails := alerts.ErrorRateBaseline{Rate: 0, Dispersion: 1, HourDispersion: 1, Buckets: 91}

		isSpike, _ := alerts.SPCIsSpike(0.1, 1000, neverFails) // 1 error in 1,000 requests

		if isSpike {
			t.Error("expected no spike: one error is within the 1-error-per-bucket floor")
		}
	})

	t.Run("alerts on many errors for an endpoint that never fails", func(t *testing.T) {
		neverFails := alerts.ErrorRateBaseline{Rate: 0, Dispersion: 1, HourDispersion: 1, Buckets: 91}

		_, severity := alerts.SPCIsSpike(5.0, 1000, neverFails) // 50 errors in 1,000 requests

		if severity != "critical" {
			t.Errorf("expected critical, got %q", severity)
		}
	})
}

func TestSPCIsDrift(t *testing.T) {
	t.Run("catches a rise that one bucket cannot show", func(t *testing.T) {
		// 1.5% on 1,000 requests is z = 1.6. Over 12,000 requests it is z = 5.5.
		_, bucketSeverity := alerts.SPCIsSpike(1.5, 1000, onePercent)
		_, hourSeverity := alerts.SPCIsDrift(1.5, 12000, onePercent)

		if bucketSeverity != "" {
			t.Errorf("expected no spike on one bucket, got %q", bucketSeverity)
		}
		if hourSeverity != "critical" {
			t.Errorf("expected critical drift over the hour, got %q", hourSeverity)
		}
	})

	t.Run("uses the hour dispersion", func(t *testing.T) {
		wanders := onePercent
		wanders.HourDispersion = 4

		isDrift, _ := alerts.SPCIsDrift(1.5, 12000, wanders) // z = 5.5 / 4 = 1.4

		if isDrift {
			t.Error("expected no drift on an endpoint whose hourly rate wanders")
		}
	})
}

func TestIsSilence(t *testing.T) {
	steady := alerts.VolumeBaseline{Median: 1000, MAD: 50, Buckets: 52} // stddev ≈ 74

	t.Run("returns critical when traffic stops", func(t *testing.T) {
		_, severity := alerts.IsSilence(0, steady)

		if severity != "critical" {
			t.Errorf("expected critical, got %q", severity)
		}
	})

	t.Run("returns warning on a large drop", func(t *testing.T) {
		_, severity := alerts.IsSilence(700, steady) // z = 4.0

		if severity != "warning" {
			t.Errorf("expected warning, got %q", severity)
		}
	})

	t.Run("returns nothing on a normal dip", func(t *testing.T) {
		isSilent, _ := alerts.IsSilence(900, steady) // z = 1.3

		if isSilent {
			t.Error("expected no silence")
		}
	})

	t.Run("uses the Poisson floor when the history is flat", func(t *testing.T) {
		flat := alerts.VolumeBaseline{Median: 1000, MAD: 0, Buckets: 52} // stddev = √1000 ≈ 32

		isSilent, _ := alerts.IsSilence(950, flat) // z = 1.6

		if isSilent {
			t.Error("expected no silence: a 5% dip is within Poisson noise")
		}
	})

	t.Run("skips low-traffic endpoints", func(t *testing.T) {
		quiet := alerts.VolumeBaseline{Median: 20, MAD: 2, Buckets: 52}

		isSilent, _ := alerts.IsSilence(0, quiet)

		if isSilent {
			t.Error("expected no silence below the minimum volume")
		}
	})

	t.Run("waits for enough history", func(t *testing.T) {
		learning := alerts.VolumeBaseline{Median: 1000, MAD: 50, Buckets: 5}

		isSilent, _ := alerts.IsSilence(0, learning)

		if isSilent {
			t.Error("expected no silence without enough history")
		}
	})
}

func TestMaxSeverity(t *testing.T) {
	tests := []struct {
		a, b, want string
	}{
		{"critical", "warning", "critical"},
		{"warning", "critical", "critical"},
		{"warning", "", "warning"},
		{"", "warning", "warning"},
		{"", "", ""},
	}

	for _, tt := range tests {
		if got := alerts.MaxSeverity(tt.a, tt.b); got != tt.want {
			t.Errorf("MaxSeverity(%q, %q) = %q, want %q", tt.a, tt.b, got, tt.want)
		}
	}
}
