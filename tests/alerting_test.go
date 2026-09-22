package tests

import (
	"testing"
	"time"

	"gorm.io/gorm"

	"lognorth/internal/apikeys"
	"lognorth/internal/events"
	"lognorth/internal/settings"
)

// =============================================================================
// ALERTING INTEGRATION TESTS
// These tests simulate real scenarios and verify the alerts generated.
// =============================================================================

func setupAlertingTestDB(t *testing.T) *testDB {
	t.Helper()
	db := setupTestDB(t)

	// Create default settings (email not configured - we just check state)
	db.Create(&settings.Settings{})

	return &testDB{db: db, t: t}
}

type testDB struct {
	db *gorm.DB
	t  *testing.T
}

// createEndpointStats records traffic in the last complete bucket.
func (tdb *testDB) createEndpointStats(apiKeyID uint, path string, total, errors int) {
	tdb.createEndpointStatsAt(events.LastCompleteBucketTime(), apiKeyID, path, total, errors)
}

// createEndpointStatsAt records traffic in the bucket that starts at `bucket`.
func (tdb *testDB) createEndpointStatsAt(bucket time.Time, apiKeyID uint, path string, total, errors int) {
	stat := events.EndpointStat{
		APIKeyID: apiKeyID,
		Path:     path,
		Bucket:   bucket,
		Total:    int64(total),
		Errors:   int64(errors),
	}
	tdb.db.Create(&stat)
}

// getAlertState returns the current persistent alert state
func (tdb *testDB) getAlertState(apiKeyID uint, path string) *events.SpikeState {
	return events.GetOrCreateSpikeState(tdb.db, apiKeyID, path)
}

// getDigestQueue returns pending warnings for daily digest
func (tdb *testDB) getDigestQueue() []events.DigestItem {
	return events.GetPendingDigestItems(tdb.db)
}

// =============================================================================
// SCENARIO: Daily Digest - Warnings batched for 9am email
// =============================================================================

func TestAlerting_Digest_WarningsQueuedForEmail(t *testing.T) {
	tdb := setupAlertingTestDB(t)

	// Given: An app with warnings
	apiKey, _, _ := apikeys.Create(tdb.db, "DigestApp")

	// When: Warnings are queued
	events.QueueDigestItem(tdb.db, apiKey.ID, "/api/slow", "Error rate elevated to 7%", "warning")
	events.QueueDigestItem(tdb.db, apiKey.ID, "/api/flaky", "Drift detected", "warning")

	// Then: Items appear in digest queue
	queue := tdb.getDigestQueue()

	if len(queue) != 2 {
		t.Fatalf("expected 2 digest items, got %d", len(queue))
	}
	if queue[0].Resolved || queue[1].Resolved {
		t.Error("new items should be unresolved")
	}
}

func TestAlerting_Digest_ClearedAfterSending(t *testing.T) {
	tdb := setupAlertingTestDB(t)

	// Given: Pending warnings
	apiKey, _, _ := apikeys.Create(tdb.db, "DigestApp2")
	events.QueueDigestItem(tdb.db, apiKey.ID, "/path1", "Issue 1", "warning")
	events.QueueDigestItem(tdb.db, apiKey.ID, "/path2", "Issue 2", "warning")

	// When: Digest is sent (items marked resolved)
	items := tdb.getDigestQueue()
	events.MarkDigestItemsResolved(tdb.db, items)

	// Then: Queue is empty
	remaining := tdb.getDigestQueue()

	if len(remaining) != 0 {
		t.Errorf("expected empty queue after digest sent, got %d items", len(remaining))
	}
}
