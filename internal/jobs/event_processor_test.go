package jobs

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/karloscodes/cartridge"
	"github.com/karloscodes/cartridge/sqlite"
	"gorm.io/gorm"

	"lognorth/internal/alerts"
	"lognorth/internal/apikeys"
	"lognorth/internal/database"
	"lognorth/internal/events"
	"lognorth/internal/settings"
)

func setupProcessorDB(t *testing.T) *gorm.DB {
	t.Helper()

	manager := sqlite.NewManager(sqlite.Config{Path: filepath.Join(t.TempDir(), "test.db"), EnableWAL: true})
	db, err := manager.Connect()
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	if err := database.Migrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

func seedIngested(t *testing.T, db *gorm.DB) uint {
	t.Helper()
	key, _, err := apikeys.Create(db, "Test App")
	if err != nil {
		t.Fatalf("create api key: %v", err)
	}
	if err := events.CreateIngested(db, []*events.IngestedEvent{{
		APIKeyID: key.ID,
		Message:  "GET /healthz",
		Context:  `{"method":"GET","path":"/healthz","status":200}`,
	}}); err != nil {
		t.Fatalf("create ingested: %v", err)
	}
	return key.ID
}

func newJobCtx(db *gorm.DB) *cartridge.JobContext {
	return &cartridge.JobContext{
		Context: context.Background(),
		Logger:  slog.New(slog.NewTextHandler(io.Discard, nil)),
		DB:      db,
	}
}

func TestEventProcessor_MovesIngestedEvents(t *testing.T) {
	db := setupProcessorDB(t)
	seedIngested(t, db)

	if err := NewEventProcessor().ProcessBatch(newJobCtx(db)); err != nil {
		t.Fatalf("ProcessBatch: %v", err)
	}

	var unprocessed int64
	if err := db.Model(&events.IngestedEvent{}).Count(&unprocessed).Error; err != nil {
		t.Fatalf("count unprocessed: %v", err)
	}
	if unprocessed != 0 {
		t.Fatalf("expected 0 unprocessed events, got %d", unprocessed)
	}

	var processed int64
	if err := db.Model(&events.Event{}).Count(&processed).Error; err != nil {
		t.Fatalf("count events: %v", err)
	}
	if processed != 1 {
		t.Fatalf("expected 1 processed event, got %d", processed)
	}
}

func TestEventProcessor_BatchWithFailingEvent(t *testing.T) {
	db := setupProcessorDB(t)
	key, _, err := apikeys.Create(db, "Test App")
	if err != nil {
		t.Fatalf("create api key: %v", err)
	}
	seed := []*events.IngestedEvent{
		{APIKeyID: key.ID, Message: "GET /a", Context: `{"path":"/a","status":200}`},
		{APIKeyID: key.ID, Message: "poison", Context: `{"path":"/poison","status":200}`},
		{APIKeyID: key.ID, Message: "GET /b", Context: `{"path":"/b","status":200}`},
	}
	if err := events.CreateIngested(db, seed); err != nil {
		t.Fatalf("create ingested: %v", err)
	}
	// The last step of a move deletes the staging row. Failing there means
	// the event and its endpoint stat were already written, so only a
	// savepoint rollback can undo them.
	if err := db.Exec(`CREATE TRIGGER poison BEFORE DELETE ON ingested_events
		WHEN OLD.message = 'poison' BEGIN SELECT RAISE(ABORT, 'poison'); END`).Error; err != nil {
		t.Fatalf("create trigger: %v", err)
	}

	err = NewEventProcessor().ProcessBatch(newJobCtx(db))

	if err != nil {
		t.Fatalf("ProcessBatch: %v", err)
	}
	var messages []string
	db.Model(&events.Event{}).Order("id").Pluck("message", &messages)
	if len(messages) != 2 || messages[0] != "GET /a" || messages[1] != "GET /b" {
		t.Errorf("events = %v, want [GET /a GET /b]", messages)
	}
	var waiting []string
	db.Model(&events.IngestedEvent{}).Pluck("message", &waiting)
	if len(waiting) != 1 || waiting[0] != "poison" {
		t.Errorf("waiting = %v, want [poison]", waiting)
	}
	var poisonStats int64
	db.Model(&events.EndpointStat{}).Where("path = ?", "/poison").Count(&poisonStats)
	if poisonStats != 0 {
		t.Errorf("endpoint stats for /poison = %d, want 0 (rolled back)", poisonStats)
	}
}

// During a deploy the old and the new container both run a processor. Both
// can read the same staging rows before either moves them.
func TestEventProcessor_TwoProcessorsMoveABatchOnce(t *testing.T) {
	db := setupProcessorDB(t)
	key, _, err := apikeys.Create(db, "Test App")
	if err != nil {
		t.Fatalf("create api key: %v", err)
	}
	if err := events.CreateIngested(db, []*events.IngestedEvent{
		{APIKeyID: key.ID, Message: "GET /a", Context: `{"path":"/a","status":200}`},
		{APIKeyID: key.ID, Message: "GET /b", Context: `{"path":"/b","status":200}`},
	}); err != nil {
		t.Fatalf("create ingested: %v", err)
	}
	batch, err := events.NextIngested(db, 0, processBatchSize) // both processors read this
	if err != nil {
		t.Fatalf("read batch: %v", err)
	}

	first, _, err := moveEvents(newJobCtx(db), batch)
	if err != nil {
		t.Fatalf("first processor: %v", err)
	}
	second, _, err := moveEvents(newJobCtx(db), batch)
	if err != nil {
		t.Fatalf("second processor: %v", err)
	}

	if len(first) != 2 || len(second) != 0 {
		t.Errorf("moved %d then %d, want 2 then 0", len(first), len(second))
	}
	var stored int64
	db.Model(&events.Event{}).Count(&stored)
	if stored != 2 {
		t.Errorf("events = %d, want 2 (no copies)", stored)
	}
	var total int64
	db.Model(&events.EndpointStat{}).Select("COALESCE(SUM(total), 0)").Scan(&total)
	if total != 2 {
		t.Errorf("endpoint stat total = %d, want 2", total)
	}
}

func TestEventProcessor_NotesReleases(t *testing.T) {
	db := setupProcessorDB(t)
	key, _, err := apikeys.Create(db, "Test App")
	if err != nil {
		t.Fatalf("create api key: %v", err)
	}
	started := time.Now().UTC().Add(-time.Hour).Truncate(time.Second)
	if err := events.CreateIngested(db, []*events.IngestedEvent{
		{APIKeyID: key.ID, Message: "Release a1b2c3d started", Context: `{"release":"a1b2c3d"}`, Timestamp: started},
		{APIKeyID: key.ID, Message: "boom", IsError: true, Context: `{"error":"boom","release":"a1b2c3d"}`, Timestamp: started.Add(time.Minute)},
		{APIKeyID: key.ID, Message: "GET / → 200", Context: `{"path":"/","status":200}`, Timestamp: started},
	}); err != nil {
		t.Fatalf("create ingested: %v", err)
	}

	if err := NewEventProcessor().ProcessBatch(newJobCtx(db)); err != nil {
		t.Fatalf("ProcessBatch: %v", err)
	}

	releases := events.ReleasesBetween(db, key.ID, started.Add(-time.Hour), time.Now())
	if len(releases) != 1 || releases[0].Version != "a1b2c3d" || !releases[0].FirstSeen.Equal(started) {
		t.Fatalf("expected a1b2c3d deployed at its start, got %+v", releases)
	}
}

func TestEventProcessor_Secrets(t *testing.T) {
	stripeKey := "sk_" + "live_" + strings.Repeat("x", 24) // joined, so gitleaks has nothing to report
	leaking := func(appID uint) *events.IngestedEvent {
		return &events.IngestedEvent{
			APIKeyID: appID,
			Message:  "POST /checkout → 200",
			Context:  `{"method":"POST","path":"/checkout","status":200,"stripe":"` + stripeKey + `"}`,
		}
	}
	setup := func(t *testing.T) (*gorm.DB, uint) {
		db := setupProcessorDB(t)
		key, _, err := apikeys.Create(db, "Shop")
		if err != nil {
			t.Fatalf("create api key: %v", err)
		}
		return db, key.ID
	}
	clock := time.Now()
	process := func(t *testing.T, db *gorm.DB, seed ...*events.IngestedEvent) {
		t.Helper()
		if err := events.CreateIngested(db, seed); err != nil {
			t.Fatalf("create ingested: %v", err)
		}
		if err := NewEventProcessor().ProcessBatch(newJobCtx(db)); err != nil {
			t.Fatalf("ProcessBatch: %v", err)
		}
		clock = clock.Add(time.Hour)
		if err := alerts.Deliver(db, clock); err != nil {
			t.Fatalf("Deliver: %v", err)
		}
	}

	t.Run("stores the event without the secret", func(t *testing.T) {
		db, appID := setup(t)

		process(t, db, leaking(appID))

		var stored events.Event
		db.First(&stored)
		if strings.Contains(stored.Context, stripeKey) || !strings.Contains(stored.Context, "[redacted: Stripe secret key]") {
			t.Errorf("stored context = %s, want the key redacted", stored.Context)
		}
	})

	t.Run("alerts once per app and kind", func(t *testing.T) {
		db, appID := setup(t)
		alerts := webhookAlerts(t, db)

		process(t, db, leaking(appID), leaking(appID))
		process(t, db, leaking(appID))

		got := alerts()
		if len(got) != 1 {
			t.Fatalf("alerts = %d, want 1: %v", len(got), got)
		}
		for _, want := range []string{"Shop — logged a secret: Stripe secret key", "at POST /checkout", "Rotate the key"} {
			if !strings.Contains(got[0], want) {
				t.Errorf("alert lacks %q:\n%s", want, got[0])
			}
		}
	})

	t.Run("alerts on the next leak once a channel exists", func(t *testing.T) {
		db, appID := setup(t)
		process(t, db, leaking(appID))
		alerts := webhookAlerts(t, db)

		process(t, db, leaking(appID))

		if got := alerts(); len(got) != 1 {
			t.Errorf("alerts = %d, want 1: %v", len(got), got)
		}
	})
}

// webhookAlerts points the alerts at a test webhook and returns the texts it
// has received.
func webhookAlerts(t *testing.T, db *gorm.DB) func() []string {
	t.Helper()
	var mu sync.Mutex
	var texts []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var alert struct{ Text string }
		json.NewDecoder(r.Body).Decode(&alert)
		mu.Lock()
		texts = append(texts, alert.Text)
		mu.Unlock()
	}))
	t.Cleanup(srv.Close)

	s, err := settings.Get(db)
	if err != nil {
		t.Fatalf("settings: %v", err)
	}
	s.NotifyChannel = "webhook"
	s.WebhookURL = srv.URL
	if err := settings.Update(db, s); err != nil {
		t.Fatalf("save settings: %v", err)
	}
	return func() []string {
		mu.Lock()
		defer mu.Unlock()
		return slices.Clone(texts)
	}
}

func TestEventProcessor_ErrorAlerts(t *testing.T) {
	failing := func(appID uint, class string, line int) *events.IngestedEvent {
		return &events.IngestedEvent{
			APIKeyID: appID, Message: "POST /pay → 500", IsError: true,
			Context: fmt.Sprintf(`{"method":"POST","path":"/pay","status":500,"error":"boom","error_class":%q,"error_file":"pay.rb","error_line":%d}`, class, line),
		}
	}
	setup := func(t *testing.T) (*gorm.DB, uint, func() []string) {
		db := setupProcessorDB(t)
		key, _, _ := apikeys.Create(db, "Shop")
		return db, key.ID, webhookAlerts(t, db)
	}
	clock := time.Now()
	process := func(t *testing.T, db *gorm.DB, seed ...*events.IngestedEvent) {
		t.Helper()
		events.CreateIngested(db, seed)
		if err := NewEventProcessor().ProcessBatch(newJobCtx(db)); err != nil {
			t.Fatalf("ProcessBatch: %v", err)
		}
		clock = clock.Add(time.Hour)
		if err := alerts.Deliver(db, clock); err != nil {
			t.Fatalf("Deliver: %v", err)
		}
	}

	t.Run("one new issue sends the alert it always sent", func(t *testing.T) {
		db, appID, alerts := setup(t)

		process(t, db, failing(appID, "PaymentError", 41))

		got := alerts()
		if len(got) != 1 || !strings.HasPrefix(got[0], "Shop — new: PaymentError: boom\n\nPaymentError: boom\nat pay.rb:41") {
			t.Errorf("alerts = %q, want one alert for the new issue", got)
		}
	})

	t.Run("three new issues in one run send one alert", func(t *testing.T) {
		db, appID, alerts := setup(t)

		process(t, db, failing(appID, "PaymentError", 41), failing(appID, "TimeoutError", 7), failing(appID, "PaymentError", 41), failing(appID, "ParseError", 3))

		got := alerts()
		if len(got) != 1 {
			t.Fatalf("alerts = %d, want 1: %q", len(got), got)
		}
		for _, want := range []string{"Shop — 3 new errors", "new: PaymentError: boom\n  at pay.rb:41", "new: TimeoutError: boom", "new: ParseError: boom", "/lognorth:investigate "} {
			if !strings.Contains(got[0], want) {
				t.Errorf("alert lacks %q:\n%s", want, got[0])
			}
		}
	})

	t.Run("a new issue and one back after a quiet day share one alert", func(t *testing.T) {
		db, appID, alerts := setup(t)
		process(t, db, failing(appID, "PaymentError", 41))
		db.Model(&events.IssueSeen{}).Where("1 = 1").Update("last_occurrence_at", time.Now().Add(-48*time.Hour))

		process(t, db, failing(appID, "PaymentError", 41), failing(appID, "TimeoutError", 7))

		got := alerts()
		if len(got) != 2 || !strings.HasPrefix(got[1], "Shop — 1 new error, 1 back") || !strings.Contains(got[1], "back after 2d: PaymentError: boom") {
			t.Errorf("alerts = %q, want the first alert, then one with the new and the returning issue", got)
		}
	})

	t.Run("each app gets its own alert", func(t *testing.T) {
		db, shop, alerts := setup(t)
		blog, _, _ := apikeys.Create(db, "Blog")

		process(t, db, failing(shop, "PaymentError", 41), failing(blog.ID, "TimeoutError", 7), failing(shop, "ParseError", 3))

		got := alerts()
		slices.Sort(got)
		if len(got) != 2 || !strings.HasPrefix(got[0], "Blog — new: TimeoutError") || !strings.HasPrefix(got[1], "Shop — 2 new errors") {
			t.Errorf("alerts = %q, want one for Blog and one for Shop", got)
		}
	})

	t.Run("lists ten issues and links the rest", func(t *testing.T) {
		db, appID, alerts := setup(t)
		var seed []*events.IngestedEvent
		for line := range 12 {
			seed = append(seed, failing(appID, "PaymentError", line+1))
		}

		process(t, db, seed...)

		got := alerts()
		if len(got) != 1 || strings.Count(got[0], "new: ") != 10 || !strings.Contains(got[0], "And 2 more") {
			t.Errorf("alerts = %q, want one alert that lists 10 and counts 2 more", got)
		}
	})
}

func TestEventProcessor_CountsHours(t *testing.T) {
	// countsMatch checks the home page counts and the 30-day histogram
	// against the stored events.
	countsMatch := func(t *testing.T, db *gorm.DB, appID uint) {
		t.Helper()
		var total, errors int64
		db.Model(&events.Event{}).Where("api_key_id = ?", appID).Count(&total)
		db.Model(&events.Event{}).Where("api_key_id = ? AND is_error = 1", appID).Count(&errors)
		stats, _ := events.GetStats(db, appID)
		if stats.Total != total || stats.Errors != errors {
			t.Errorf("stats count %d events, %d errors; the table has %d and %d", stats.Total, stats.Errors, total, errors)
		}
		var drawn int64
		for _, b := range events.GetHistogram(db, appID, time.Now().AddDate(0, 0, -30), time.Time{}) {
			drawn += b.Total
		}
		if drawn != total {
			t.Errorf("the histogram draws %d events; the table has %d", drawn, total)
		}
	}
	request := func(appID uint, at time.Time, failed bool) *events.IngestedEvent {
		status := 200
		if failed {
			status = 500
		}
		return &events.IngestedEvent{
			APIKeyID: appID, Message: "GET /", IsError: failed, Timestamp: at,
			Context: fmt.Sprintf(`{"method":"GET","path":"/","status":%d}`, status),
		}
	}

	t.Run("counts every moved event in its hour, a late one too", func(t *testing.T) {
		db := setupProcessorDB(t)
		key, _, _ := apikeys.Create(db, "Shop")
		now := time.Now()
		events.CreateIngested(db, []*events.IngestedEvent{request(key.ID, now, false), request(key.ID, now, true)})
		NewEventProcessor().ProcessBatch(newJobCtx(db))
		events.CreateIngested(db, []*events.IngestedEvent{request(key.ID, now, false), request(key.ID, now.Add(-72*time.Hour), true)})

		err := NewEventProcessor().ProcessBatch(newJobCtx(db))

		if err != nil {
			t.Fatalf("ProcessBatch: %v", err)
		}
		countsMatch(t, db, key.ID)
		var old events.AppHour
		db.Where("api_key_id = ? AND hour = ?", key.ID, now.Add(-72*time.Hour).UTC().Truncate(time.Hour)).First(&old)
		if old.Total != 1 || old.Errors != 1 {
			t.Errorf("the hour 3 days ago counts %d (%d errors), want the late error", old.Total, old.Errors)
		}
	})
}

func TestEventProcessor_KeepsIssues(t *testing.T) {
	db := setupProcessorDB(t)
	shop, _, _ := apikeys.Create(db, "Shop")
	failing := func(class, path string, ago time.Duration) *events.IngestedEvent {
		return &events.IngestedEvent{
			APIKeyID: shop.ID, Message: "failed", IsError: true, Timestamp: time.Now().Add(-ago),
			Context: fmt.Sprintf(`{"path":%q,"status":500,"error":"boom","error_class":%q,"error_file":"a.rb","error_line":1}`, path, class),
		}
	}
	for run, seed := range [][]*events.IngestedEvent{
		{failing("A", "/a", 3*time.Hour), failing("B", "/b", 2*time.Hour)},
		{failing("A", "/a2", time.Hour), failing("A", "/a", 72*time.Hour), failing("C", "", time.Minute)},
		{failing("B", "/b", 10*time.Second)},
	} {
		events.CreateIngested(db, seed)
		if err := NewEventProcessor().ProcessBatch(newJobCtx(db)); err != nil {
			t.Fatalf("run %d: %v", run, err)
		}
	}

	kept, _ := events.ListIssuesWithState(db, shop.ID, 100)
	events.RecountAllIssues(db)
	recounted, _ := events.ListIssuesWithState(db, shop.ID, 100)

	keptJSON, _ := json.Marshal(kept)
	recountedJSON, _ := json.Marshal(recounted)
	if len(kept) != 3 || string(keptJSON) != string(recountedJSON) {
		t.Errorf("kept by the processor:\n%s\nrecounted from the events:\n%s", keptJSON, recountedJSON)
	}
}
