package alerts

import (
	"encoding/json"
	"fmt"
	"math"
	"math/rand/v2"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"gorm.io/gorm"

	"lognorth/internal/apikeys"
	"lognorth/internal/events"
	"lognorth/internal/settings"
	"lognorth/internal/uptime"
)

// A world replays an app's life through the real detectors and the real
// sender, on a simulated clock: endpoint traffic in endpoint_stats, error
// events, uptime pings, and leaked secrets. Every email lands in a real
// webhook, stamped with the simulated time it went out. Nothing is mocked:
// only the clock is the test's.
//
// Each minute of a run: the uptime pings of the minute, the endpoint check
// every 5 minutes, then the sender.
type world struct {
	t      *testing.T
	db     *gorm.DB
	start  time.Time // the start of the scenario: history comes before it
	now    time.Time
	rng    *rand.Rand
	apps   map[string]*apikeys.APIKey
	pings  map[uint]func(time.Time) bool
	errors []plannedError
	inbox  *inbox
}

// An email the owner got: the subject, the whole text, and when it went out.
type email struct {
	At      time.Time
	Subject string
	Text    string
}

type inbox struct {
	mu   sync.Mutex
	got  []email
	seen int
}

type plannedError struct {
	at      time.Time
	app     string
	class   string
	message string
	path    string
	line    int
}

// scenarioStart is a Wednesday at noon UTC: far from a weekend and from
// midnight, so a scenario's history and its run fall on plain weekdays.
var scenarioStart = time.Date(2026, 10, 14, 12, 0, 0, 0, time.UTC)

func newWorld(t *testing.T) *world {
	t.Helper()
	w := &world{
		t:     t,
		db:    newTestDB(t),
		start: scenarioStart,
		now:   scenarioStart,
		rng:   rand.New(rand.NewPCG(1, 2)),
		apps:  map[string]*apikeys.APIKey{},
		pings: map[uint]func(time.Time) bool{},
		inbox: &inbox{},
	}
	srv := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		var alert struct{ Title, Text string }
		json.NewDecoder(r.Body).Decode(&alert)
		w.inbox.mu.Lock()
		w.inbox.got = append(w.inbox.got, email{Subject: alert.Title, Text: alert.Text})
		w.inbox.mu.Unlock()
	}))
	t.Cleanup(srv.Close)
	w.settings(func(s *settings.Settings) {
		s.NotifyChannel = "webhook"
		s.WebhookURL = srv.URL
		s.BaseURL = "https://logs.example.com"
		s.TimeZone = "UTC" // hq's own zone must not leak into a test
	})
	return w
}

// settings changes the saved settings.
func (w *world) settings(change func(*settings.Settings)) {
	w.t.Helper()
	s, err := settings.Get(w.db)
	if err != nil {
		w.t.Fatal(err)
	}
	change(s)
	if err := settings.Update(w.db, s); err != nil {
		w.t.Fatal(err)
	}
}

// app creates an app, old enough to be past any first-minutes grace.
func (w *world) app(name string) *apikeys.APIKey {
	w.t.Helper()
	if key, ok := w.apps[name]; ok {
		return key
	}
	key, _, err := apikeys.Create(w.db, name)
	if err != nil {
		w.t.Fatal(err)
	}
	w.db.Model(key).Updates(map[string]any{"created_at": w.start.AddDate(0, -2, 0), "ping_url": "https://" + strings.ToLower(name) + ".example.com/up"})
	key, _ = apikeys.FindByID(w.db, key.ID)
	w.apps[name] = key
	return key
}

// traffic describes an endpoint's normal: requests per 5-minute bucket at the
// daily mean, and the % that fail.
type traffic struct {
	requests  float64
	errorRate float64
	diurnal   float64 // 0..1: how much the day's wave moves the requests (busiest at 15:00 UTC)
	weekend   float64 // the share of the requests on Saturday and Sunday; 0 means 1
}

// A change overrides the normal from one time on: it returns the requests and
// the error rate of the bucket at t, given the normal ones.
type change func(t time.Time, requests, errorRate float64) (float64, float64)

// endpoint writes an endpoint's buckets from `from` to `to`: the normal with
// binomial noise, then each change in order.
func (w *world) endpoint(app, path string, normal traffic, from, to time.Time, changes ...change) {
	w.t.Helper()
	key := w.app(app)
	var rows []events.EndpointStat
	for b := events.TruncateTo5Min(from.UTC()); b.Before(to); b = b.Add(5 * time.Minute) {
		requests, rate := normal.at(b)
		for _, c := range changes {
			requests, rate = c(b, requests, rate)
		}
		total := w.poisson(requests)
		if total == 0 {
			continue // a bucket with no requests has no row
		}
		errors := w.binomial(total, rate/100)
		rows = append(rows, events.EndpointStat{APIKeyID: key.ID, Path: path, Bucket: b, Total: total, Errors: errors, AvgDuration: 50})
	}
	if err := w.db.CreateInBatches(rows, 500).Error; err != nil {
		w.t.Fatal(err)
	}
}

func (n traffic) at(t time.Time) (float64, float64) {
	requests := n.requests
	if n.diurnal > 0 {
		hour := float64(t.Hour()) + float64(t.Minute())/60
		requests *= 1 + n.diurnal*math.Cos((hour-15)/24*2*math.Pi)
	}
	if n.weekend > 0 && (t.Weekday() == time.Saturday || t.Weekday() == time.Sunday) {
		requests *= n.weekend
	}
	return requests, n.errorRate
}

// between applies a change from `from` to before `to` only.
func between(from, to time.Time, c change) change {
	return func(t time.Time, requests, rate float64) (float64, float64) {
		if t.Before(from) || !t.Before(to) {
			return requests, rate
		}
		return c(t, requests, rate)
	}
}

func failing(rate float64) change {
	return func(t time.Time, requests, _ float64) (float64, float64) { return requests, rate }
}

func silent() change {
	return func(t time.Time, _, rate float64) (float64, float64) { return 0, rate }
}

// ramp moves the error rate from `from` to `to` % over the window.
func ramp(start, end time.Time, from, to float64) change {
	return func(t time.Time, requests, rate float64) (float64, float64) {
		if t.Before(start) {
			return requests, rate
		}
		if !t.Before(end) {
			return requests, to
		}
		share := float64(t.Sub(start)) / float64(end.Sub(start))
		return requests, from + (to-from)*share
	}
}

func (w *world) poisson(mean float64) int64 {
	if mean <= 0 {
		return 0
	}
	if mean > 50 {
		return int64(math.Max(0, math.Round(mean+w.rng.NormFloat64()*math.Sqrt(mean))))
	}
	// Knuth's method for a small mean.
	limit, product, n := math.Exp(-mean), 1.0, int64(0)
	for {
		product *= w.rng.Float64()
		if product <= limit {
			return n
		}
		n++
	}
}

func (w *world) binomial(n int64, p float64) int64 {
	if p <= 0 {
		return 0
	}
	if p >= 1 {
		return n
	}
	if n > 100 {
		mean, sd := float64(n)*p, math.Sqrt(float64(n)*p*(1-p))
		return int64(math.Min(float64(n), math.Max(0, math.Round(mean+w.rng.NormFloat64()*sd))))
	}
	var k int64
	for range n {
		if w.rng.Float64() < p {
			k++
		}
	}
	return k
}

// ping sets an app's uptime: up(t) says whether the ping at t answers.
func (w *world) ping(app string, up func(t time.Time) bool) {
	w.pings[w.app(app).ID] = up
}

func upExcept(from, to time.Time) func(time.Time) bool {
	return func(t time.Time) bool { return t.Before(from) || !t.Before(to) }
}

// failError plans one error event of an app at t.
func (w *world) failError(at time.Time, app, class, message string) {
	w.app(app)
	w.errors = append(w.errors, plannedError{at: at, app: app, class: class, message: message, path: "/checkout", line: 42})
}

// run steps the clock one minute at a time for d.
func (w *world) run(d time.Duration) {
	w.t.Helper()
	end := w.now.Add(d)
	for w.now.Before(end) {
		w.now = w.now.Add(time.Minute)
		w.tick()
	}
}

// runUntil runs to the given time.
func (w *world) runUntil(at time.Time) {
	w.run(at.Sub(w.now))
}

func (w *world) tick() {
	w.t.Helper()
	now := w.now
	s, _ := settings.Get(w.db)

	// The errors due this minute go through the processor's path.
	var later []plannedError
	for _, e := range w.errors {
		if e.at.After(now) {
			later = append(later, e)
			continue
		}
		w.storeError(e, s.BaseURL)
	}
	w.errors = later

	for appID, up := range w.pings {
		key, err := apikeys.FindByID(w.db, appID)
		if err != nil {
			w.t.Fatal(err)
		}
		check := uptime.Check{Up: up(now), Status: 200, LatencyMS: 40}
		if !check.Up {
			check.Status, check.Error = 502, "502 Bad Gateway"
		}
		if err := ApplyPing(w.db, key, check, s.BaseURL, now); err != nil {
			w.t.Fatal(err)
		}
	}

	if now.Minute()%5 == 2 { // the checker runs every 5 minutes, at its own phase
		if err := CheckEndpoints(w.db, now); err != nil {
			w.t.Fatal(err)
		}
	}

	if err := Deliver(w.db, now); err != nil {
		w.t.Fatal(err)
	}
	w.inbox.mu.Lock()
	for i := w.inbox.seen; i < len(w.inbox.got); i++ {
		w.inbox.got[i].At = now
	}
	w.inbox.seen = len(w.inbox.got)
	w.inbox.mu.Unlock()
}

func (w *world) storeError(e plannedError, baseURL string) {
	w.t.Helper()
	ctx, _ := json.Marshal(map[string]any{"method": "POST", "path": e.path, "status": 500, "error": e.message, "error_class": e.class, "error_file": "app/checkout.rb", "error_line": e.line})
	event := &events.Event{APIKeyID: w.apps[e.app].ID, Message: "POST " + e.path + " → 500", IsError: true, Context: string(ctx), Timestamp: e.at}
	if err := events.Create(w.db, event); err != nil {
		w.t.Fatal(err)
	}
	alert, err := CheckError(w.db, event, baseURL, e.at)
	if err != nil {
		w.t.Fatal(err)
	}
	if alert != nil {
		if err := Record(w.db, e.at, alert); err != nil {
			w.t.Fatal(err)
		}
	}
}

// emails returns every email so far.
func (w *world) emails() []email {
	w.inbox.mu.Lock()
	defer w.inbox.mu.Unlock()
	return append([]email(nil), w.inbox.got...)
}

// emailsWith returns the emails whose subject holds s.
func (w *world) emailsWith(s string) []email {
	var found []email
	for _, e := range w.emails() {
		if strings.Contains(e.Subject, s) {
			found = append(found, e)
		}
	}
	return found
}

// expectEmails fails unless exactly n emails went out.
func (w *world) expectEmails(n int) []email {
	w.t.Helper()
	got := w.emails()
	if len(got) != n {
		w.t.Fatalf("got %d emails, want %d:\n%s", len(got), n, describe(got))
	}
	return got
}

// expectAtMost fails when more than n emails went out.
func (w *world) expectAtMost(n int) []email {
	w.t.Helper()
	got := w.emails()
	if len(got) > n {
		w.t.Fatalf("got %d emails, want at most %d:\n%s", len(got), n, describe(got))
	}
	return got
}

// within fails unless the email went out at most d after `from`.
func (w *world) within(e email, from time.Time, d time.Duration) {
	w.t.Helper()
	if e.At.Before(from) || e.At.Sub(from) > d {
		w.t.Errorf("%q went out at %s, %s after the start; want within %s", e.Subject, e.At.Format("Mon 15:04"), e.At.Sub(from), d)
	}
}

func describe(got []email) string {
	var b strings.Builder
	for _, e := range got {
		fmt.Fprintf(&b, "  %s  %s\n", e.At.Format("Mon 15:04"), e.Subject)
	}
	return b.String()
}

// The history most scenarios need: four weeks of an endpoint's normal before
// the start, and the normal through the scenario.
func (w *world) normal(app, path string, n traffic, through time.Duration, changes ...change) {
	w.endpoint(app, path, n, w.start.AddDate(0, 0, -28), w.start.Add(through), changes...)
}

// bucket sets an endpoint's bucket at `at` to exact counts.
func (w *world) bucket(app, path string, at time.Time, total, errors int64) {
	w.t.Helper()
	key := w.app(app)
	b := events.TruncateTo5Min(at.UTC())
	w.db.Where("api_key_id = ? AND path = ? AND bucket = ?", key.ID, path, b).Delete(&events.EndpointStat{})
	if err := w.db.Create(&events.EndpointStat{APIKeyID: key.ID, Path: path, Bucket: b, Total: total, Errors: errors, AvgDuration: 50}).Error; err != nil {
		w.t.Fatal(err)
	}
}
