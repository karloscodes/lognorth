package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/karloscodes/cartridge/sqlite"
	"gorm.io/gorm"

	"lognorth/internal/apikeys"
	"lognorth/internal/database"
	"lognorth/internal/events"
	"lognorth/internal/mcp"
	"lognorth/internal/uptime"
)

const testKey = "lgn-agent-test"

// testServer runs LogNorth's MCP handler over HTTP on a fresh database, with
// the agent key check the real route does.
func testServer(t *testing.T) (*gorm.DB, *client) {
	t.Helper()

	manager := sqlite.NewManager(sqlite.Config{Path: filepath.Join(t.TempDir(), "test.db"), EnableWAL: true})
	db, err := manager.Connect()
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	if err := database.Migrate(db); err != nil {
		t.Fatalf("Failed to migrate: %v", err)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/mcp" || r.Header.Get("Authorization") != "Bearer "+testKey {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		var req mcp.Request
		json.NewDecoder(r.Body).Decode(&req)
		res, _ := mcp.Handle(db, req)
		json.NewEncoder(w).Encode(res)
	}))
	t.Cleanup(srv.Close)

	return db, newClient(remote{URL: srv.URL, Key: testKey})
}

func createApp(t *testing.T, db *gorm.DB, name string) uint {
	t.Helper()
	key, _, err := apikeys.Create(db, name)
	if err != nil {
		t.Fatalf("Failed to create app: %v", err)
	}
	return key.ID
}

func logEvent(t *testing.T, db *gorm.DB, e *events.Event, ctx *events.EventContext) {
	t.Helper()
	if e.Timestamp.IsZero() {
		e.Timestamp = time.Now()
	}
	if ctx != nil {
		e.SetContext(ctx)
	}
	if err := events.Create(db, e); err != nil {
		t.Fatalf("Failed to create event: %v", err)
	}
}

func TestConnect(t *testing.T) {
	t.Run("saves the server and key when the server accepts them", func(t *testing.T) {
		t.Setenv("XDG_CONFIG_HOME", t.TempDir())
		t.Setenv("LOGNORTH_URL", "")
		db, c := testServer(t)
		createApp(t, db, "checkout-prod")

		err := connect([]string{c.URL + "/mcp/", testKey})

		if err != nil {
			t.Fatalf("connect failed: %v", err)
		}
		saved, err := loadRemote()
		if err != nil || saved.URL != c.URL || saved.Key != testKey {
			t.Errorf("saved = %+v (%v), want %s with the key", saved, err, c.URL)
		}
		path, _ := remotePath()
		if info, _ := os.Stat(path); info.Mode().Perm() != 0o600 {
			t.Errorf("file mode = %v, want 0600", info.Mode().Perm())
		}
	})

	t.Run("refuses an app key", func(t *testing.T) {
		t.Setenv("XDG_CONFIG_HOME", t.TempDir())
		_, c := testServer(t)

		err := connect([]string{c.URL, "lgn-0123abcd"})

		if err == nil || !strings.Contains(err.Error(), "app key") {
			t.Errorf("err = %v, want it to say this is an app key", err)
		}
	})

	t.Run("refuses a key the server rejects, and saves nothing", func(t *testing.T) {
		t.Setenv("XDG_CONFIG_HOME", t.TempDir())
		_, c := testServer(t)

		err := connect([]string{c.URL, "lgn-agent-wrong"})

		if err == nil || !strings.Contains(err.Error(), "rejected") {
			t.Errorf("err = %v, want the rejected key", err)
		}
		path, _ := remotePath()
		if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
			t.Errorf("a rejected key was saved to %s", path)
		}
	})

	t.Run("the plugin's environment variables win over the file", func(t *testing.T) {
		t.Setenv("XDG_CONFIG_HOME", t.TempDir())
		t.Setenv("LOGNORTH_URL", "logs.example.com/")
		t.Setenv("LOGNORTH_AGENT_KEY", "lgn-agent-env")

		r, err := loadRemote()

		if err != nil || r.URL != "https://logs.example.com" || r.Key != "lgn-agent-env" {
			t.Errorf("remote = %+v (%v), want the environment", r, err)
		}
	})
}

func TestTail(t *testing.T) {
	t.Run("prints the recent events oldest first, then each new one once", func(t *testing.T) {
		db, c := testServer(t)
		appID := createApp(t, db, "checkout-prod")
		logEvent(t, db, &events.Event{APIKeyID: appID, Message: "first", Timestamp: time.Now().Add(-2 * time.Minute)}, nil)
		logEvent(t, db, &events.Event{APIKeyID: appID, Message: "second", Timestamp: time.Now().Add(-time.Minute)}, nil)
		tl := &tailer{c: c, filter: map[string]any{}}
		var out bytes.Buffer

		if err := tl.backlog(&out, 20); err != nil {
			t.Fatalf("backlog: %v", err)
		}
		logEvent(t, db, &events.Event{APIKeyID: appID, Message: "third"}, nil)
		if err := tl.poll(&out); err != nil {
			t.Fatalf("poll: %v", err)
		}
		if err := tl.poll(&out); err != nil {
			t.Fatalf("second poll: %v", err)
		}

		got := out.String()
		first, second, third := strings.Index(got, "first"), strings.Index(got, "second"), strings.Index(got, "third")
		if first < 0 || !(first < second && second < third) {
			t.Errorf("output = %q, want first, second, third in order", got)
		}
		if strings.Count(got, "third") != 1 {
			t.Errorf("output = %q, want the new event once", got)
		}
	})

	t.Run("-n 0 prints nothing old, only what arrives next", func(t *testing.T) {
		db, c := testServer(t)
		appID := createApp(t, db, "checkout-prod")
		logEvent(t, db, &events.Event{APIKeyID: appID, Message: "old"}, nil)
		tl := &tailer{c: c, filter: map[string]any{}}
		var out bytes.Buffer

		tl.backlog(&out, 0)
		logEvent(t, db, &events.Event{APIKeyID: appID, Message: "new"}, nil)
		tl.poll(&out)

		if got := out.String(); strings.Contains(got, "old") || !strings.Contains(got, "new") {
			t.Errorf("output = %q, want only the new event", got)
		}
	})

	t.Run("--errors and a search keep only what matches", func(t *testing.T) {
		db, c := testServer(t)
		appID := createApp(t, db, "checkout-prod")
		logEvent(t, db, &events.Event{APIKeyID: appID, Message: "POST /pay 500", IsError: true, ErrorMessage: "card declined"},
			&events.EventContext{Method: "POST", Path: "/pay", Status: 500})
		logEvent(t, db, &events.Event{APIKeyID: appID, Message: "GET /health 200"}, &events.EventContext{Method: "GET", Path: "/health", Status: 200})
		tl := &tailer{c: c, filter: map[string]any{"errors_only": true, "search": "/pay"}}
		var out bytes.Buffer

		tl.backlog(&out, 20)

		if got := out.String(); !strings.Contains(got, "/pay") || strings.Contains(got, "/health") {
			t.Errorf("output = %q, want only the failed /pay", got)
		}
	})

	t.Run("a failed request shows its status, error, and code location", func(t *testing.T) {
		tl := &tailer{}
		e := event{Message: "POST /checkout 500", DurationMS: 1204, IsError: true, ErrorClass: "Stripe::CardError",
			ErrorMessage: "Your card was declined.", ErrorFile: "app/services/payment_service.rb", ErrorLine: 67,
			Context: `{"method":"POST","path":"/checkout","status":500}`}

		got := tl.format(e)

		for _, want := range []string{"POST", "/checkout", "500 · 1204ms", "Stripe::CardError: Your card was declined.", "payment_service.rb:67"} {
			if !strings.Contains(got, want) {
				t.Errorf("line = %q, want %q", got, want)
			}
		}
	})

	t.Run("indents the events of the request above", func(t *testing.T) {
		tl := &tailer{}
		tl.format(event{TraceID: "t1", Context: `{"method":"POST","path":"/checkout","status":201}`})

		got := tl.format(event{TraceID: "t1", Message: "Charging card", DurationMS: 847})

		if !strings.Contains(got, "    Charging card") {
			t.Errorf("line = %q, want it indented under its request", got)
		}
	})

	t.Run("names the app when several share the stream", func(t *testing.T) {
		tl := &tailer{names: map[uint]string{1: "checkout-prod"}}

		got := tl.format(event{APIKeyID: 1, Message: "hello"})

		if !strings.Contains(got, "checkout-prod") {
			t.Errorf("line = %q, want the app name", got)
		}
	})
}

func TestTop(t *testing.T) {
	t.Run("reads the endpoints, alerts, and uptime of one app", func(t *testing.T) {
		db, c := testServer(t)
		appID := createApp(t, db, "checkout-prod")
		at := events.LastCompleteBucketTime()
		db.Create(&events.EndpointStat{APIKeyID: appID, Path: "/checkout", Bucket: at, Total: 100, Errors: 20, AvgDuration: 850})
		db.Create(&events.EndpointStat{APIKeyID: appID, Path: "/health", Bucket: at, Total: 300, AvgDuration: 3})
		db.Model(&apikeys.APIKey{}).Where("id = ?", appID).Update("ping_url", "https://shop.example.com/up")
		uptime.Record(db, &uptime.Check{APIKeyID: appID, CheckedAt: time.Now().Add(-time.Minute), Up: true, Status: 200, LatencyMS: 84})
		st := topState{apps: []app{{ID: appID, Name: "checkout-prod"}}}

		s := read(c, st, true, nil)

		if s.err != nil {
			t.Fatalf("read: %v", s.err)
		}
		if len(s.endpoints) != 2 || s.endpoints[0].Path != "/checkout" {
			t.Errorf("endpoints = %+v, want /checkout first", s.endpoints)
		}
		if s.uptime == nil || s.uptime.Status != "up" || s.uptime.LatencyMS != 84 {
			t.Errorf("uptime = %+v, want up 84ms", s.uptime)
		}
	})

	fixture := func() (topState, snapshot) {
		pct := 99.5
		u := &uptimeReport{Status: "up", LatencyMS: 84, Uptime24h: &pct}
		u.Bars = make([]struct {
			Checks int `json:"checks"`
			Fails  int `json:"fails"`
		}, 96)
		for i := range u.Bars {
			u.Bars[i].Checks = 15
		}
		u.Bars[40].Fails = 2
		st := topState{apps: []app{{ID: 1, Name: "checkout-prod"}, {ID: 2, Name: "api-prod"}}}
		s := snapshot{
			appID: 1, window: "1h", at: time.Now(), uptime: u,
			endpoints: []endpointRow{
				{Path: "/checkout", Total: 1204, Errors: 219, ErrorRate: 18.2, AvgDuration: 850},
				{Path: "/health", Total: 3000, AvgDuration: 3},
			},
			alerts: []alertRow{{Path: "/checkout", Kind: "error_rate", Since: time.Now(), Summary: "18.2% errors in the last 5 min, normally 0.9%."}},
		}
		return st, s
	}

	t.Run("draws the alert, the flagged endpoint, the uptime strip, and the keys", func(t *testing.T) {
		st, s := fixture()

		lines := renderTop(st, s, false, time.Now(), 100, 30, palette{})

		screen := strings.Join(lines, "\n")
		for _, want := range []string{"top · checkout-prod (1/2)", "■ up 84ms", "99.50%", "/checkout SPIKE", "1,204", "18.2%", "a app", "q quit"} {
			if !strings.Contains(screen, want) {
				t.Errorf("screen is missing %q:\n%s", want, screen)
			}
		}
		if !strings.Contains(screen, "> /checkout") {
			t.Errorf("want the first endpoint selected:\n%s", screen)
		}
	})

	t.Run("fills the window exactly and never wider", func(t *testing.T) {
		st, s := fixture()
		st.open, st.path = true, "/checkout"
		s.errors = []event{{Timestamp: time.Now(), ErrorClass: "Stripe::CardError", ErrorMessage: strings.Repeat("declined ", 30)}}

		for _, size := range [][2]int{{60, 12}, {100, 30}, {200, 50}} {
			lines := renderTop(st, s, false, time.Now(), size[0], size[1], palette{})

			if len(lines) != size[1] {
				t.Errorf("%dx%d: %d lines, want %d", size[0], size[1], len(lines), size[1])
			}
			for _, l := range lines {
				if n := utf8.RuneCountInString(l); n > size[0] {
					t.Errorf("%dx%d: a line is %d wide: %q", size[0], size[1], n, l)
				}
			}
		}
	})

	t.Run("asks for a bigger window below 60x12", func(t *testing.T) {
		st, s := fixture()

		lines := renderTop(st, s, false, time.Now(), 50, 10, palette{})

		if len(lines) != 1 || !strings.Contains(lines[0], "60×12") {
			t.Errorf("lines = %q, want the size hint", lines)
		}
	})
}
