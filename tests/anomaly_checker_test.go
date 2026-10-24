package tests

import (
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/karloscodes/cartridge"

	"lognorth/internal/alerts"
	"lognorth/internal/apikeys"
	"lognorth/internal/events"
	"lognorth/internal/jobs"
)

// runAnomalyChecker runs the real job once against the test database.
func (tdb *testDB) runAnomalyChecker() {
	tdb.t.Helper()

	ctx := &cartridge.JobContext{
		Context: tdb.t.Context(),
		Logger:  slog.New(slog.NewTextHandler(io.Discard, nil)),
		DB:      tdb.db,
	}
	if err := jobs.NewAnomalyChecker().ProcessBatch(ctx); err != nil {
		tdb.t.Fatalf("anomaly checker failed: %v", err)
	}
}

// seedHistory records the same traffic in every history bucket, `count`
// windows `step` apart, like a normal past for the endpoint.
func (tdb *testDB) seedHistory(apiKeyID uint, path string, step time.Duration, count, total, errors int) {
	at := events.LastCompleteBucketTime()
	for i := 1; i <= count; i++ {
		center := at.Add(-time.Duration(i) * step)
		for b := center.Add(-alerts.HistoryWindow); !b.After(center.Add(alerts.HistoryWindow)); b = b.Add(5 * time.Minute) {
			tdb.createEndpointStatsAt(b, apiKeyID, path, total, errors)
		}
	}
}

// seedDays gives an endpoint 7 normal days at this time of day.
func (tdb *testDB) seedDays(apiKeyID uint, path string, total, errors int) {
	tdb.seedHistory(apiKeyID, path, 24*time.Hour, alerts.ErrorRateHistoryDays, total, errors)
}

// setLastBucket changes the traffic of the last complete bucket.
func (tdb *testDB) setLastBucket(path string, total, errors int) {
	tdb.db.Model(&events.EndpointStat{}).
		Where("path = ? AND bucket = ?", path, events.LastCompleteBucketTime()).
		Updates(map[string]any{"total": total, "errors": errors})
}

func TestAnomalyChecker_ErrorRate(t *testing.T) {
	t.Run("with a spike", func(t *testing.T) {
		tdb := setupAlertingTestDB(t)
		apiKey, _, _ := apikeys.Create(tdb.db, "Shop")
		tdb.seedDays(apiKey.ID, "/api/orders", 1000, 10)
		tdb.createEndpointStats(apiKey.ID, "/api/orders", 1000, 80)

		tdb.runAnomalyChecker()

		state := tdb.getAlertState(apiKey.ID, "/api/orders")
		if state.Severity != "critical" {
			t.Errorf("8%% on a 1%% history → expected critical, got %q", state.Severity)
		}
		if state.ErrorCount != 80 {
			t.Errorf("expected the real error count 80, got %d", state.ErrorCount)
		}
	})

	t.Run("with normal traffic", func(t *testing.T) {
		tdb := setupAlertingTestDB(t)
		apiKey, _, _ := apikeys.Create(tdb.db, "Shop")
		tdb.seedDays(apiKey.ID, "/api/orders", 1000, 10)
		tdb.createEndpointStats(apiKey.ID, "/api/orders", 1000, 12)

		tdb.runAnomalyChecker()

		if state := tdb.getAlertState(apiKey.ID, "/api/orders"); state.Severity != "" {
			t.Errorf("1.2%% on a 1%% history → expected no alert, got %q", state.Severity)
		}
	})

	t.Run("with too few requests", func(t *testing.T) {
		tdb := setupAlertingTestDB(t)
		apiKey, _, _ := apikeys.Create(tdb.db, "Shop")
		tdb.seedDays(apiKey.ID, "/api/admin", 1000, 0)
		tdb.createEndpointStats(apiKey.ID, "/api/admin", 2, 1)

		tdb.runAnomalyChecker()

		if state := tdb.getAlertState(apiKey.ID, "/api/admin"); state.Severity != "" {
			t.Errorf("1 error in 2 requests → expected no alert, got %q", state.Severity)
		}
	})

	t.Run("with one error on an endpoint that never fails", func(t *testing.T) {
		tdb := setupAlertingTestDB(t)
		apiKey, _, _ := apikeys.Create(tdb.db, "Shop")
		tdb.seedDays(apiKey.ID, "/api/health", 1000, 0)
		tdb.createEndpointStats(apiKey.ID, "/api/health", 1000, 1)

		tdb.runAnomalyChecker()

		if state := tdb.getAlertState(apiKey.ID, "/api/health"); state.Severity != "" {
			t.Errorf("1 error in 1,000 requests → expected no alert, got %q", state.Severity)
		}
	})

	t.Run("with a spike in the bucket that is still filling", func(t *testing.T) {
		tdb := setupAlertingTestDB(t)
		apiKey, _, _ := apikeys.Create(tdb.db, "Shop")
		tdb.seedDays(apiKey.ID, "/api/orders", 1000, 10)
		tdb.createEndpointStatsAt(events.TruncateTo5Min(time.Now().UTC()), apiKey.ID, "/api/orders", 100, 50)

		tdb.runAnomalyChecker()

		if state := tdb.getAlertState(apiKey.ID, "/api/orders"); state.Severity != "" {
			t.Errorf("expected the partial bucket to be ignored, got %q", state.Severity)
		}
	})

	t.Run("without history", func(t *testing.T) {
		tdb := setupAlertingTestDB(t)
		apiKey, _, _ := apikeys.Create(tdb.db, "Shop")
		tdb.createEndpointStats(apiKey.ID, "/api/new", 1000, 500)

		tdb.runAnomalyChecker()

		if state := tdb.getAlertState(apiKey.ID, "/api/new"); state.Severity != "" {
			t.Errorf("a new endpoint has no normal yet → expected no alert, got %q", state.Severity)
		}
	})

	t.Run("with a slow rise over the last hour", func(t *testing.T) {
		tdb := setupAlertingTestDB(t)
		apiKey, _, _ := apikeys.Create(tdb.db, "Shop")
		tdb.seedDays(apiKey.ID, "/api/checkout", 1000, 10)
		at := events.LastCompleteBucketTime()
		for b := at.Add(-55 * time.Minute); !b.After(at); b = b.Add(5 * time.Minute) {
			tdb.createEndpointStatsAt(b, apiKey.ID, "/api/checkout", 1000, 15)
		}

		tdb.runAnomalyChecker()

		if state := tdb.getAlertState(apiKey.ID, "/api/checkout"); state.Severity != "critical" {
			t.Errorf("1.5%% for an hour on a 1%% history → expected critical drift, got %q", state.Severity)
		}
	})

	t.Run("with an outage in the history", func(t *testing.T) {
		tdb := setupAlertingTestDB(t)
		apiKey, _, _ := apikeys.Create(tdb.db, "Shop")
		tdb.seedDays(apiKey.ID, "/api/orders", 1000, 10)
		yesterday := events.LastCompleteBucketTime().Add(-24 * time.Hour)
		tdb.db.Model(&events.EndpointStat{}).Where("bucket = ?", yesterday).Update("errors", 500)
		tdb.createEndpointStats(apiKey.ID, "/api/orders", 1000, 40)

		tdb.runAnomalyChecker()

		if state := tdb.getAlertState(apiKey.ID, "/api/orders"); state.Severity != "critical" {
			t.Errorf("one past outage must not hide a 4%% spike → expected critical, got %q", state.Severity)
		}
	})

	t.Run("recovers after two clean checks", func(t *testing.T) {
		tdb := setupAlertingTestDB(t)
		apiKey, _, _ := apikeys.Create(tdb.db, "Shop")
		tdb.seedDays(apiKey.ID, "/api/orders", 1000, 10)
		tdb.createEndpointStats(apiKey.ID, "/api/orders", 1000, 80)
		tdb.runAnomalyChecker()
		tdb.setLastBucket("/api/orders", 1000, 10)

		tdb.runAnomalyChecker()
		afterOne := tdb.getAlertState(apiKey.ID, "/api/orders").Severity
		tdb.runAnomalyChecker()
		afterTwo := tdb.getAlertState(apiKey.ID, "/api/orders").Severity

		if afterOne != "critical" {
			t.Errorf("after one clean check → expected still critical, got %q", afterOne)
		}
		if afterTwo != "" {
			t.Errorf("after two clean checks → expected cleared, got %q", afterTwo)
		}
	})
}

func TestAnomalyChecker_Silence(t *testing.T) {
	t.Run("with a traffic drop", func(t *testing.T) {
		tdb := setupAlertingTestDB(t)
		apiKey, _, _ := apikeys.Create(tdb.db, "Shop")
		tdb.seedHistory(apiKey.ID, "/api/webhooks", 7*24*time.Hour, alerts.VolumeHistoryWeeks, 1000, 0)
		tdb.createEndpointStats(apiKey.ID, "/api/webhooks", 10, 0)

		tdb.runAnomalyChecker()

		silence := alerts.GetOrCreateSilenceState(tdb.db, apiKey.ID, "/api/webhooks")
		if silence.Severity != "critical" {
			t.Errorf("10 requests on a 1,000 history → expected critical, got %q", silence.Severity)
		}
	})

	t.Run("with no requests at all", func(t *testing.T) {
		tdb := setupAlertingTestDB(t)
		apiKey, _, _ := apikeys.Create(tdb.db, "Shop")
		tdb.seedHistory(apiKey.ID, "/api/webhooks", 7*24*time.Hour, alerts.VolumeHistoryWeeks, 1000, 0)
		tdb.createEndpointStatsAt(events.LastCompleteBucketTime().Add(-time.Hour), apiKey.ID, "/api/webhooks", 1000, 0)

		tdb.runAnomalyChecker()

		silence := alerts.GetOrCreateSilenceState(tdb.db, apiKey.ID, "/api/webhooks")
		if silence.Severity != "critical" {
			t.Errorf("an empty bucket on a 1,000 history → expected critical, got %q", silence.Severity)
		}
	})

	t.Run("with normal traffic", func(t *testing.T) {
		tdb := setupAlertingTestDB(t)
		apiKey, _, _ := apikeys.Create(tdb.db, "Shop")
		tdb.seedHistory(apiKey.ID, "/api/webhooks", 7*24*time.Hour, alerts.VolumeHistoryWeeks, 1000, 0)
		tdb.createEndpointStats(apiKey.ID, "/api/webhooks", 980, 0)

		tdb.runAnomalyChecker()

		silence := alerts.GetOrCreateSilenceState(tdb.db, apiKey.ID, "/api/webhooks")
		if silence.Severity != "" {
			t.Errorf("980 requests on a 1,000 history → expected no alert, got %q", silence.Severity)
		}
	})
}

func TestGetUnnotifiedCriticalSpikes(t *testing.T) {
	t.Run("returns a new critical spike", func(t *testing.T) {
		tdb := setupAlertingTestDB(t)
		tdb.db.Create(&alerts.SpikeState{Path: "/api/orders", Severity: "critical", SpikeStartAt: time.Now()})

		spikes := alerts.GetUnnotifiedCriticalSpikes(tdb.db)

		if len(spikes) != 1 {
			t.Errorf("expected 1 spike to email, got %d", len(spikes))
		}
	})

	t.Run("waits for the cooldown after the last email", func(t *testing.T) {
		tdb := setupAlertingTestDB(t)
		tdb.db.Create(&alerts.SpikeState{
			Path:         "/api/orders",
			Severity:     "critical",
			SpikeStartAt: time.Now(),
			LastAlertAt:  time.Now().Add(-10 * time.Minute),
		})

		spikes := alerts.GetUnnotifiedCriticalSpikes(tdb.db)

		if len(spikes) != 0 {
			t.Errorf("an email 10 min ago → expected no new email, got %d", len(spikes))
		}
	})

	t.Run("does not email the same spike twice", func(t *testing.T) {
		tdb := setupAlertingTestDB(t)
		start := time.Now().Add(-3 * time.Hour)
		tdb.db.Create(&alerts.SpikeState{
			Path:         "/api/orders",
			Severity:     "critical",
			SpikeStartAt: start,
			LastAlertAt:  start.Add(time.Minute),
		})

		spikes := alerts.GetUnnotifiedCriticalSpikes(tdb.db)

		if len(spikes) != 0 {
			t.Errorf("expected no second email for the same spike, got %d", len(spikes))
		}
	})
}
