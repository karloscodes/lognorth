package tests

import (
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/karloscodes/cartridge"

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

func (tdb *testDB) baselineSamples(apiKeyID uint, path string) int {
	hourOfWeek := events.HourOfWeek(events.LastCompleteBucketTime())
	return events.GetOrCreateBaseline(tdb.db, apiKeyID, path, hourOfWeek).SampleCount
}

func TestAnomalyChecker(t *testing.T) {
	t.Run("with a spike on a learned baseline", func(t *testing.T) {
		tdb := setupAlertingTestDB(t)
		apiKey, _, _ := apikeys.Create(tdb.db, "Shop")
		tdb.createBaseline(apiKey.ID, "/api/orders", events.HourOfWeek(events.LastCompleteBucketTime()), 1.0, 0.5, 50)
		tdb.createEndpointStats(apiKey.ID, "/api/orders", 1000, 50)

		tdb.runAnomalyChecker()

		state := tdb.getAlertState(apiKey.ID, "/api/orders")
		if state.Severity != "critical" {
			t.Errorf("expected critical, got %q", state.Severity)
		}
		if state.ErrorCount != 50 {
			t.Errorf("expected the real error count 50, got %d", state.ErrorCount)
		}
		if got := tdb.baselineSamples(apiKey.ID, "/api/orders"); got != 50 {
			t.Errorf("expected the baseline to skip the spike (50 samples), got %d", got)
		}
	})

	t.Run("with too few requests", func(t *testing.T) {
		tdb := setupAlertingTestDB(t)
		apiKey, _, _ := apikeys.Create(tdb.db, "Shop")
		tdb.createBaseline(apiKey.ID, "/api/admin", events.HourOfWeek(events.LastCompleteBucketTime()), 0.0, 0.0, 50)
		tdb.createEndpointStats(apiKey.ID, "/api/admin", 2, 1)

		tdb.runAnomalyChecker()

		if state := tdb.getAlertState(apiKey.ID, "/api/admin"); state.Severity != "" {
			t.Errorf("1 error in 2 requests → expected no alert, got %q", state.Severity)
		}
		if got := tdb.baselineSamples(apiKey.ID, "/api/admin"); got != 50 {
			t.Errorf("expected no learning from 2 requests (50 samples), got %d", got)
		}
	})

	t.Run("with one error on an endpoint that never fails", func(t *testing.T) {
		tdb := setupAlertingTestDB(t)
		apiKey, _, _ := apikeys.Create(tdb.db, "Shop")
		tdb.createBaseline(apiKey.ID, "/api/health", events.HourOfWeek(events.LastCompleteBucketTime()), 0.0, 0.0001, 50)
		tdb.createEndpointStats(apiKey.ID, "/api/health", 1000, 1)

		tdb.runAnomalyChecker()

		if state := tdb.getAlertState(apiKey.ID, "/api/health"); state.Severity != "" {
			t.Errorf("1 error in 1,000 requests → expected no alert, got %q", state.Severity)
		}
		if got := tdb.baselineSamples(apiKey.ID, "/api/health"); got != 51 {
			t.Errorf("expected the baseline to learn from a normal bucket (51 samples), got %d", got)
		}
	})

	t.Run("with a spike in the bucket that is still filling", func(t *testing.T) {
		tdb := setupAlertingTestDB(t)
		apiKey, _, _ := apikeys.Create(tdb.db, "Shop")
		tdb.db.Create(&events.EndpointStat{
			APIKeyID: apiKey.ID,
			Path:     "/api/orders",
			Bucket:   events.TruncateTo5Min(time.Now()),
			Total:    100,
			Errors:   50,
		})

		tdb.runAnomalyChecker()

		if state := tdb.getAlertState(apiKey.ID, "/api/orders"); state.Severity != "" {
			t.Errorf("expected the partial bucket to be ignored, got %q", state.Severity)
		}
	})

	t.Run("with a new endpoint during cold start", func(t *testing.T) {
		tdb := setupAlertingTestDB(t)
		apiKey, _, _ := apikeys.Create(tdb.db, "Shop")
		tdb.createEndpointStats(apiKey.ID, "/api/search", 100, 10)

		tdb.runAnomalyChecker()

		if state := tdb.getAlertState(apiKey.ID, "/api/search"); state.Severity != "critical" {
			t.Errorf("10%% on cold-start defaults (2%%±2%%) → expected critical, got %q", state.Severity)
		}
		if got := tdb.baselineSamples(apiKey.ID, "/api/search"); got != 1 {
			t.Errorf("expected the baseline to keep learning during cold start (1 sample), got %d", got)
		}
	})

	t.Run("with a traffic drop", func(t *testing.T) {
		tdb := setupAlertingTestDB(t)
		apiKey, _, _ := apikeys.Create(tdb.db, "Shop")
		hourOfWeek := events.HourOfWeek(events.LastCompleteBucketTime())
		tdb.db.Create(&events.VolumeBaselineStat{
			APIKeyID:    apiKey.ID,
			Path:        "/api/webhooks",
			HourOfWeek:  hourOfWeek,
			Mean:        1000,
			StdDev:      100,
			SampleCount: 50,
		})
		tdb.createEndpointStats(apiKey.ID, "/api/webhooks", 10, 0)

		tdb.runAnomalyChecker()

		silence := events.GetOrCreateSilenceState(tdb.db, apiKey.ID, "/api/webhooks")
		if silence.Severity != "critical" {
			t.Errorf("10 requests on a 1000±100 baseline → expected critical, got %q", silence.Severity)
		}
		volume := events.GetOrCreateVolumeBaseline(tdb.db, apiKey.ID, "/api/webhooks", hourOfWeek)
		if volume.SampleCount != 50 || volume.Mean != 1000 {
			t.Errorf("expected the outage to stay out of the baseline, got mean=%.0f samples=%d", volume.Mean, volume.SampleCount)
		}
	})
}
