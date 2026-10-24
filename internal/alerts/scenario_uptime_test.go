package alerts

import (
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"lognorth/internal/apikeys"
	"lognorth/internal/events"
	"lognorth/internal/settings"
)

// An app's uptime ping, every minute; and incidents, where several detectors
// fire for one cause.

func TestScenarioUptime(t *testing.T) {
	t.Run("an app that goes down emails once, after 3 failed pings", func(t *testing.T) {
		t.Parallel()
		w := newWorld(t)
		down := w.start.Add(10 * time.Minute)
		w.ping("shop", upExcept(down, down.Add(2*time.Hour)))

		w.run(time.Hour)

		got := w.expectEmails(1)
		w.within(got[0], down, 5*time.Minute)
		if got[0].Subject != "shop — down: https://shop.example.com/up" {
			t.Errorf("subject = %q", got[0].Subject)
		}
	})

	t.Run("recovery emails once, after 5 good pings", func(t *testing.T) {
		t.Parallel()
		w := newWorld(t)
		down, back := w.start.Add(10*time.Minute), w.start.Add(40*time.Minute)
		w.ping("shop", upExcept(down, back))

		w.run(2 * time.Hour)

		got := w.expectEmails(2)
		w.within(got[1], back, 7*time.Minute)
		if got[1].Subject != "shop — recovered after 30m down" {
			t.Errorf("subject = %q, want the down time measured to the first good ping", got[1].Subject)
		}
	})

	t.Run("one or two failed pings send nothing", func(t *testing.T) {
		t.Parallel()
		w := newWorld(t)
		at := w.start.Add(10 * time.Minute)
		w.ping("shop", upExcept(at, at.Add(2*time.Minute)))

		w.run(time.Hour)

		w.expectEmails(0)
	})

	t.Run("an app that flaps for half an hour sends one down email", func(t *testing.T) {
		t.Parallel()
		w := newWorld(t)
		start := w.start.Add(10 * time.Minute)
		flap := func(t time.Time) bool {
			if t.Before(start) || !t.Before(start.Add(30*time.Minute)) {
				return true
			}
			return int(t.Sub(start)/time.Minute)%4 == 3 // 3 fails, 1 answer, again
		}
		w.ping("shop", flap)

		w.run(time.Hour)

		got := w.emails()
		if len(w.emailsWith("— down: ")) != 1 || len(got) > 2 {
			t.Errorf("want one down email (and one recovery):\n%s", describe(got))
		}
	})

	t.Run("an outage inside quiet hours goes out in the morning as one email", func(t *testing.T) {
		t.Parallel()
		w := newWorld(t)
		w.settings(func(s *settings.Settings) { s.QuietHoursEnabled, s.QuietStart, s.QuietEnd = true, 23, 7 })
		down := time.Date(2026, 10, 15, 2, 0, 0, 0, time.UTC)
		w.ping("shop", upExcept(down, down.Add(40*time.Minute)))

		w.runUntil(time.Date(2026, 10, 15, 9, 0, 0, 0, time.UTC))

		got := w.expectEmails(1)
		if got[0].Subject != "shop — while you were off: down and recovered" {
			t.Errorf("subject = %q", got[0].Subject)
		}
	})

	t.Run("an app still down when quiet hours end says so at once", func(t *testing.T) {
		t.Parallel()
		w := newWorld(t)
		w.settings(func(s *settings.Settings) { s.QuietHoursEnabled, s.QuietStart, s.QuietEnd = true, 23, 7 })
		down := time.Date(2026, 10, 15, 6, 0, 0, 0, time.UTC)
		w.ping("shop", upExcept(down, down.Add(3*time.Hour)))

		w.runUntil(time.Date(2026, 10, 15, 8, 0, 0, 0, time.UTC))

		got := w.expectEmails(1)
		w.within(got[0], time.Date(2026, 10, 15, 7, 0, 0, 0, time.UTC), 2*time.Minute)
		if !strings.Contains(got[0].Subject, "down: ") {
			t.Errorf("subject = %q, want the down", got[0].Subject)
		}
	})

	t.Run("the down email names the errors that surged", func(t *testing.T) {
		t.Parallel()
		w := newWorld(t)
		down := w.start.Add(30 * time.Minute)
		w.ping("shop", upExcept(down, down.Add(time.Hour)))
		for i := range 10 {
			w.failError(down.Add(time.Duration(i)*10*time.Second), "shop", "PG::ConnectionBad", "could not connect to server")
		}

		w.run(time.Hour)

		down1 := w.emailsWith("— down: ")
		if len(down1) != 1 || !strings.Contains(down1[0].Text, "PG::ConnectionBad") {
			t.Errorf("want the down email to name the surge:\n%s\n%v", describe(w.emails()), texts(down1))
		}
	})

	t.Run("two outages an hour apart send two down emails", func(t *testing.T) {
		t.Parallel()
		w := newWorld(t)
		first, second := w.start.Add(10*time.Minute), w.start.Add(90*time.Minute)
		w.ping("shop", func(t time.Time) bool {
			return upExcept(first, first.Add(15*time.Minute))(t) && upExcept(second, second.Add(15*time.Minute))(t)
		})

		w.run(3 * time.Hour)

		if got := w.emailsWith("down: "); len(got) != 2 {
			t.Errorf("want two down emails:\n%s", describe(w.emails()))
		}
	})
}

func TestScenarioIncidents(t *testing.T) {
	busy := traffic{requests: 200, errorRate: 1, diurnal: 0.3}

	t.Run("a database outage sends at most 3 emails, not one per endpoint", func(t *testing.T) {
		t.Parallel()
		w := newWorld(t)
		outage := w.start.Add(time.Hour)
		for _, path := range fifteenPaths() {
			w.normal("shop", path, busy, 3*time.Hour, between(outage, outage.Add(30*time.Minute), failing(80)))
		}
		w.ping("shop", upExcept(outage, outage.Add(30*time.Minute)))
		for i := range 30 {
			w.failError(outage.Add(time.Duration(i)*20*time.Second), "shop", "PG::ConnectionBad", "could not connect to server")
		}

		w.run(3 * time.Hour)

		got := w.expectAtMost(3)
		if len(w.emailsWith("— down: ")) == 0 {
			t.Errorf("want the down told:\n%s", describe(got))
		}
		if strings.Contains(describe(got), "endpoints failing") {
			t.Errorf("the spikes went out while the app was down:\n%s", describe(got))
		}
	})

	t.Run("an endpoint that still fails after the app is back alerts then", func(t *testing.T) {
		t.Parallel()
		w := newWorld(t)
		outage := w.start.Add(time.Hour)
		w.normal("shop", "/checkout", busy, 4*time.Hour, between(outage, outage.Add(2*time.Hour), failing(60)))
		w.ping("shop", upExcept(outage, outage.Add(20*time.Minute)))

		w.run(4 * time.Hour)

		spikes := w.emailsWith("spike: /checkout")
		if len(spikes) == 0 {
			spikes = w.emailsWith("endpoint")
		}
		if len(spikes) == 0 {
			t.Fatalf("want the failing endpoint told once the app is back:\n%s", describe(w.emails()))
		}
		if spikes[0].At.Before(outage.Add(20 * time.Minute)) {
			t.Errorf("the spike went out at %s, while the app was down", spikes[0].At.Format("15:04"))
		}
	})

	t.Run("an incident that keeps producing alerts sends at most one email per 10 minutes", func(t *testing.T) {
		t.Parallel()
		w := newWorld(t)
		start := w.start.Add(10 * time.Minute)
		for i := range 60 {
			w.failError(start.Add(time.Duration(i)*time.Minute), "shop", "Error"+string(rune('A'+i%26))+string(rune('a'+i/26)), "boom")
		}

		w.run(90 * time.Minute)

		got := w.emails()
		for i := 1; i < len(got); i++ {
			if gap := got[i].At.Sub(got[i-1].At); gap < followUpGap {
				t.Errorf("two emails %s apart:\n%s", gap, describe(got))
			}
		}
	})
}

func TestScenarioDelivery(t *testing.T) {
	t.Run("without a channel, alerts are dropped, and a channel set up later sends nothing old", func(t *testing.T) {
		t.Parallel()
		w := newWorld(t)
		var url string
		w.settings(func(s *settings.Settings) { url, s.NotifyChannel = s.WebhookURL, "" })
		w.failError(w.start.Add(time.Minute), "shop", "PaymentError", "card declined")
		w.run(time.Hour)
		w.settings(func(s *settings.Settings) { s.NotifyChannel, s.WebhookURL = "webhook", url })

		w.run(time.Hour)

		w.expectEmails(0)
		var dropped int64
		w.db.Model(&Alert{}).Where("result = ?", "dropped: no channel").Count(&dropped)
		if dropped != 1 {
			t.Errorf("dropped %d alerts, want 1", dropped)
		}
	})

	t.Run("a webhook that fails is tried again until it answers", func(t *testing.T) {
		t.Parallel()
		w := newWorld(t)
		var url string
		w.settings(func(s *settings.Settings) { url, s.WebhookURL = s.WebhookURL, "http://127.0.0.1:1/down" })
		w.failError(w.start.Add(time.Minute), "shop", "PaymentError", "card declined")
		w.run(30 * time.Minute)
		var waiting Alert
		w.db.First(&waiting)
		if waiting.SentAt != nil || !strings.HasPrefix(waiting.Result, "failed: ") {
			t.Errorf("the alert is %+v, want it waiting with the failure", waiting)
		}
		w.settings(func(s *settings.Settings) { s.WebhookURL = url })

		w.run(5 * time.Minute)

		w.expectEmails(1)
	})

	t.Run("an alert that cannot go out for a day is dropped", func(t *testing.T) {
		t.Parallel()
		w := newWorld(t)
		var url string
		w.settings(func(s *settings.Settings) { url, s.WebhookURL = s.WebhookURL, "http://127.0.0.1:1/down" })
		w.failError(w.start.Add(time.Minute), "shop", "PaymentError", "card declined")
		w.run(25 * time.Hour)
		w.settings(func(s *settings.Settings) { s.WebhookURL = url })

		w.run(time.Hour)

		w.expectEmails(0)
	})

	t.Run("a deleted app's alert still goes out, named as unknown", func(t *testing.T) {
		t.Parallel()
		w := newWorld(t)
		w.failError(w.start.Add(time.Minute), "gone", "PaymentError", "card declined")
		w.run(10 * time.Second)
		apikeys.Delete(w.db, w.apps["gone"].ID)

		w.run(10 * time.Minute)

		got := w.expectEmails(1)
		if !strings.HasPrefix(got[0].Subject, "Unknown app — new: ") {
			t.Errorf("subject = %q", got[0].Subject)
		}
	})
}

func TestScenarioLeaks(t *testing.T) {
	leak := func(w *world, at time.Time, kind string) {
		w.t.Helper()
		event := &events.Event{APIKeyID: w.app("shop").ID, Message: "POST /checkout → 200", Context: `{"method":"POST","path":"/checkout"}`, Timestamp: at}
		if err := events.Create(w.db, event); err != nil {
			w.t.Fatal(err)
		}
		s, _ := settings.Get(w.db)
		alert, err := CheckLeak(w.db, Leak{Event: event, Kind: kind}, s, at)
		if err != nil {
			w.t.Fatal(err)
		}
		if alert != nil {
			Record(w.db, at, alert)
		}
	}

	t.Run("a leaked secret alerts once per app and kind", func(t *testing.T) {
		t.Parallel()
		w := newWorld(t)
		leak(w, w.start, "Stripe secret key")
		leak(w, w.start, "Stripe secret key")
		leak(w, w.start.Add(time.Hour), "Stripe secret key")

		w.run(2 * time.Hour)

		got := w.expectEmails(1)
		if got[0].Subject != "shop — logged a secret: Stripe secret key" || !strings.Contains(got[0].Text, "Rotate the key") {
			t.Errorf("email = %q:\n%s", got[0].Subject, got[0].Text)
		}
	})

	t.Run("a leak before a channel exists alerts on the next leak once one does", func(t *testing.T) {
		t.Parallel()
		w := newWorld(t)
		var url string
		w.settings(func(s *settings.Settings) { url, s.NotifyChannel = s.WebhookURL, "" })
		leak(w, w.start, "AWS access key")
		w.run(time.Hour)
		w.settings(func(s *settings.Settings) { s.NotifyChannel, s.WebhookURL = "webhook", url })

		leak(w, w.now, "AWS access key")
		w.run(time.Hour)

		w.expectEmails(1)
	})
}

func TestScenarioDigest(t *testing.T) {
	t.Run("a rise too small to email reaches the morning digest, at 9 in the owner's zone", func(t *testing.T) {
		t.Parallel()
		w := newWorld(t)
		w.settings(func(s *settings.Settings) { s.TimeZone = "Asia/Tokyo" }) // UTC+9
		rise := w.start.Add(time.Hour)
		w.normal("shop", "/checkout", traffic{requests: 400, errorRate: 2}, 23*time.Hour, between(rise, rise.Add(3*time.Hour), failing(3.2)))
		digest := &Digest{}
		logger := slog.New(slog.NewTextHandler(io.Discard, nil))

		var digestAt time.Time
		for w.now.Before(w.start.Add(23 * time.Hour)) {
			w.run(time.Minute)
			before := len(w.emails())
			digest.Run(w.db, logger, w.now)
			w.inbox.mu.Lock()
			for i := w.inbox.seen; i < len(w.inbox.got); i++ {
				w.inbox.got[i].At = w.now
			}
			w.inbox.seen = len(w.inbox.got)
			w.inbox.mu.Unlock()
			if len(w.emails()) > before && digestAt.IsZero() {
				digestAt = w.now
			}
		}

		got := w.expectEmails(1)
		if !strings.HasPrefix(got[0].Subject, "Daily digest") || !strings.Contains(got[0].Text, "/checkout error rate elevated") {
			t.Fatalf("want the warning in the digest:\n%s\n%s", describe(got), got[0].Text)
		}
		if local := digestAt.In(time.FixedZone("JST", 9*3600)); local.Hour() != 9 {
			t.Errorf("the digest went out at %s in Tokyo, want 09:00", local.Format("15:04"))
		}
	})
}

func TestScenarioUrgentDown(t *testing.T) {
	t.Run("a down goes out at once, even minutes after another email", func(t *testing.T) {
		t.Parallel()
		w := newWorld(t)
		w.failError(w.start.Add(time.Minute), "shop", "PaymentError", "card declined")
		down := w.start.Add(3 * time.Minute)
		w.ping("shop", upExcept(down, down.Add(time.Hour)))

		w.run(30 * time.Minute)

		got := w.emailsWith("— down: ")
		if len(got) != 1 {
			t.Fatalf("want one down email:\n%s", describe(w.emails()))
		}
		w.within(got[0], down, 5*time.Minute)
	})
}
