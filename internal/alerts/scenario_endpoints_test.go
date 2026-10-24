package alerts

import (
	"strings"
	"testing"
	"time"
)

// Spikes, drift, silence, and small endpoints, replayed with four weeks of
// history. Each case says what an on-call developer wants from it.

func TestScenarioErrorRate(t *testing.T) {
	busy := traffic{requests: 200, errorRate: 2, diurnal: 0.3}

	t.Run("a deploy that takes an endpoint from 2% to 40% errors emails within 10 minutes", func(t *testing.T) {
		t.Parallel()
		w := newWorld(t)
		deploy := w.start.Add(time.Hour)
		w.normal("shop", "/checkout", busy, 3*time.Hour, between(deploy, deploy.Add(time.Hour), failing(40)))

		w.run(3 * time.Hour)

		got := w.expectEmails(1)
		w.within(got[0], deploy, 15*time.Minute)
		if !strings.Contains(got[0].Subject, "shop — spike: /checkout") {
			t.Errorf("subject = %q, want a spike on /checkout", got[0].Subject)
		}
	})

	t.Run("two days of normal noise send no email", func(t *testing.T) {
		t.Parallel()
		w := newWorld(t)
		w.normal("shop", "/checkout", busy, 48*time.Hour)
		w.normal("shop", "/search", traffic{requests: 600, errorRate: 0.5, diurnal: 0.5}, 48*time.Hour)

		w.run(48 * time.Hour)

		w.expectEmails(0)
	})

	t.Run("one 500 on a quiet endpoint sends nothing", func(t *testing.T) {
		t.Parallel()
		w := newWorld(t)
		w.normal("shop", "/admin/export", traffic{requests: 4, errorRate: 0}, 2*time.Hour)
		w.bucket("shop", "/admin/export", w.start.Add(30*time.Minute), 4, 1)

		w.run(2 * time.Hour)

		w.expectEmails(0)
	})

	t.Run("every request of 5 minutes failing on a small endpoint emails", func(t *testing.T) {
		t.Parallel()
		w := newWorld(t)
		broken := w.start.Add(30 * time.Minute)
		w.normal("shop", "/admin/export", traffic{requests: 4, errorRate: 0}, 2*time.Hour)
		w.bucket("shop", "/admin/export", broken, 6, 6)

		w.run(2 * time.Hour)

		got := w.expectEmails(1)
		w.within(got[0], broken, 15*time.Minute)
	})

	t.Run("a 5-minute blip alerts once and clears", func(t *testing.T) {
		t.Parallel()
		w := newWorld(t)
		blip := w.start.Add(time.Hour)
		w.normal("shop", "/checkout", busy, 4*time.Hour, between(blip, blip.Add(5*time.Minute), failing(60)))

		w.run(4 * time.Hour)

		w.expectEmails(1)
		state := GetOrCreateSpikeState(w.db, w.app("shop").ID, "/checkout")
		if state.Severity != "" {
			t.Errorf("severity %q after the blip, want it cleared", state.Severity)
		}
	})

	t.Run("an endpoint that flaps between failing and fine emails at most once an hour", func(t *testing.T) {
		t.Parallel()
		w := newWorld(t)
		start := w.start.Add(30 * time.Minute)
		flap := func(t time.Time, requests, rate float64) (float64, float64) {
			if t.Before(start) || t.After(start.Add(3*time.Hour)) || (t.Minute()/15)%2 == 1 {
				return requests, rate
			}
			return requests, 50
		}
		w.normal("shop", "/checkout", busy, 4*time.Hour, flap)

		w.run(4 * time.Hour)

		w.expectAtMost(4)
	})

	t.Run("a repeat of yesterday's outage at the same hour still emails", func(t *testing.T) {
		t.Parallel()
		w := newWorld(t)
		today := w.start.Add(time.Hour)
		yesterday := today.Add(-24 * time.Hour)
		w.normal("shop", "/checkout", busy, 3*time.Hour,
			between(yesterday, yesterday.Add(30*time.Minute), failing(50)),
			between(today, today.Add(30*time.Minute), failing(12)))

		w.run(3 * time.Hour)

		got := w.expectEmails(1)
		w.within(got[0], today, 15*time.Minute)
	})

	t.Run("an endpoint that always fails more at night stays quiet at night", func(t *testing.T) {
		t.Parallel()
		w := newWorld(t)
		nightly := func(t time.Time, requests, rate float64) (float64, float64) {
			if h := t.Hour(); h >= 1 && h < 5 {
				return requests, 8 // a nightly batch job that times out, every night
			}
			return requests, rate
		}
		w.normal("shop", "/sync", traffic{requests: 100, errorRate: 1}, 24*time.Hour, nightly)

		w.run(24 * time.Hour)

		w.expectEmails(0)
	})

	t.Run("a small rise from 2% to 3% stays quiet", func(t *testing.T) {
		t.Parallel()
		w := newWorld(t)
		w.normal("shop", "/checkout", busy, 4*time.Hour, between(w.start, w.start.Add(4*time.Hour), failing(3)))

		w.run(4 * time.Hour)

		w.expectEmails(0)
	})

	t.Run("a new endpoint with no history sends no spike", func(t *testing.T) {
		t.Parallel()
		w := newWorld(t)
		w.endpoint("shop", "/beta", traffic{requests: 100, errorRate: 30}, w.start.Add(-time.Hour), w.start.Add(2*time.Hour))

		w.run(2 * time.Hour)

		w.expectEmails(0)
	})

	t.Run("fifteen endpoints that fail at once send one email", func(t *testing.T) {
		t.Parallel()
		w := newWorld(t)
		outage := w.start.Add(time.Hour)
		for _, path := range fifteenPaths() {
			w.normal("shop", path, busy, 3*time.Hour, between(outage, outage.Add(20*time.Minute), failing(60)))
		}

		w.run(3 * time.Hour)

		got := w.emails()
		if len(got) == 0 || len(got) > 2 {
			t.Fatalf("got %d emails, want 1 or 2:\n%s", len(got), describe(got))
		}
		if !strings.Contains(got[0].Subject, "endpoints failing") && !strings.Contains(got[0].Subject, "spike") {
			t.Errorf("first subject = %q, want the failing endpoints", got[0].Subject)
		}
		listed := strings.Count(strings.Join([]string{got[0].Text, func() string {
			if len(got) > 1 {
				return got[1].Text
			}
			return ""
		}()}, "\n"), "spike: ")
		if listed < 10 {
			t.Errorf("the emails list %d spikes, want the endpoints listed (10 or more)", listed)
		}
	})

	t.Run("two apps that fail at once get one email each", func(t *testing.T) {
		t.Parallel()
		w := newWorld(t)
		outage := w.start.Add(time.Hour)
		w.normal("shop", "/checkout", busy, 3*time.Hour, between(outage, outage.Add(time.Hour), failing(50)))
		w.normal("blog", "/posts", busy, 3*time.Hour, between(outage, outage.Add(time.Hour), failing(50)))

		w.run(3 * time.Hour)

		if len(w.emailsWith("shop — ")) != 1 || len(w.emailsWith("blog — ")) != 1 {
			t.Errorf("want one email for each app:\n%s", describe(w.emails()))
		}
	})
}

func TestScenarioDrift(t *testing.T) {
	t.Run("a slow climb from 1% to 8% over 6 hours emails before it reaches 4%", func(t *testing.T) {
		t.Parallel()
		w := newWorld(t)
		climb := w.start.Add(time.Hour)
		w.normal("shop", "/checkout", traffic{requests: 200, errorRate: 1}, 8*time.Hour, ramp(climb, climb.Add(6*time.Hour), 1, 8))

		w.run(8 * time.Hour)

		got := w.expectEmails(1)
		rateAtEmail := 1 + 7*float64(got[0].At.Sub(climb))/float64(6*time.Hour)
		if rateAtEmail > 4 {
			t.Errorf("emailed at %s, when the rate was %.1f%%; want before 4%%", got[0].At.Format("15:04"), rateAtEmail)
		}
	})

	t.Run("a steady 3% for a day sends nothing", func(t *testing.T) {
		t.Parallel()
		w := newWorld(t)
		w.normal("shop", "/checkout", traffic{requests: 200, errorRate: 3, diurnal: 0.3}, 24*time.Hour)

		w.run(24 * time.Hour)

		w.expectEmails(0)
	})
}

func TestScenarioSilence(t *testing.T) {
	steady := traffic{requests: 300, errorRate: 1, diurnal: 0.3}

	t.Run("an endpoint that stops emails a silence within 15 minutes", func(t *testing.T) {
		t.Parallel()
		w := newWorld(t)
		stop := w.start.Add(time.Hour)
		w.normal("shop", "/checkout", steady, 3*time.Hour, between(stop, stop.Add(time.Hour), silent()))

		w.run(3 * time.Hour)

		got := w.expectEmails(1)
		w.within(got[0], stop, 15*time.Minute)
		if !strings.Contains(got[0].Subject, "silent: /checkout") {
			t.Errorf("subject = %q, want a silence on /checkout", got[0].Subject)
		}
	})

	t.Run("a weekend with a third of the traffic stays quiet", func(t *testing.T) {
		t.Parallel()
		w := newWorld(t)
		w.start = time.Date(2026, 10, 17, 0, 0, 0, 0, time.UTC) // a Saturday
		w.now = w.start
		w.normal("shop", "/checkout", traffic{requests: 300, errorRate: 1, diurnal: 0.6, weekend: 0.33}, 57*time.Hour)

		w.run(57 * time.Hour) // to Monday 09:00

		w.expectEmails(0)
	})

	t.Run("a low-traffic endpoint that stops sends nothing", func(t *testing.T) {
		t.Parallel()
		w := newWorld(t)
		stop := w.start.Add(time.Hour)
		w.normal("shop", "/webhooks/stripe", traffic{requests: 20, errorRate: 0}, 3*time.Hour, between(stop, stop.Add(2*time.Hour), silent()))

		w.run(3 * time.Hour)

		w.expectEmails(0)
	})

	t.Run("an endpoint that stops twice, hours apart, emails twice", func(t *testing.T) {
		t.Parallel()
		w := newWorld(t)
		first, second := w.start.Add(time.Hour), w.start.Add(5*time.Hour)
		w.normal("shop", "/checkout", steady, 7*time.Hour,
			between(first, first.Add(30*time.Minute), silent()),
			between(second, second.Add(30*time.Minute), silent()))

		w.run(7 * time.Hour)

		got := w.expectEmails(2)
		w.within(got[0], first, 15*time.Minute)
		w.within(got[1], second, 15*time.Minute)
	})
}

func TestScenarioSmallEndpoints(t *testing.T) {
	small := traffic{requests: 10, errorRate: 1}

	t.Run("an endpoint at 10 requests per 5 minutes that fails completely emails within 20 minutes", func(t *testing.T) {
		t.Parallel()
		w := newWorld(t)
		outage := w.start.Add(time.Hour)
		w.normal("shop", "/account", small, 3*time.Hour, between(outage, outage.Add(time.Hour), failing(100)))

		w.run(3 * time.Hour)

		got := w.expectEmails(1)
		w.within(got[0], outage, 20*time.Minute)
	})

	t.Run("a small endpoint with two errors stays quiet", func(t *testing.T) {
		t.Parallel()
		w := newWorld(t)
		blip := w.start.Add(time.Hour)
		w.normal("shop", "/account", small, 3*time.Hour, between(blip, blip.Add(10*time.Minute), failing(10)))

		w.run(3 * time.Hour)

		w.expectEmails(0)
	})

	t.Run("a small endpoint that drifts from 1% to 30% is caught within the hour", func(t *testing.T) {
		t.Parallel()
		w := newWorld(t)
		drift := w.start.Add(time.Hour)
		w.normal("shop", "/account", small, 4*time.Hour, between(drift, drift.Add(3*time.Hour), failing(30)))

		w.run(4 * time.Hour)

		got := w.expectEmails(1)
		w.within(got[0], drift, 70*time.Minute)
	})

	t.Run("a small endpoint that always fails 20% is not caught by the plain rule at 50%", func(t *testing.T) {
		t.Parallel()
		w := newWorld(t)
		worse := w.start.Add(time.Hour)
		w.normal("shop", "/legacy", traffic{requests: 10, errorRate: 20}, 3*time.Hour, between(worse, worse.Add(20*time.Minute), failing(50)))

		w.run(3 * time.Hour)

		w.expectEmails(0)
	})
}

func fifteenPaths() []string {
	var paths []string
	for _, p := range []string{"/checkout", "/cart", "/search", "/account", "/orders", "/products", "/login", "/logout",
		"/wishlist", "/reviews", "/shipping", "/payment", "/coupons", "/returns", "/api/v1/items"} {
		paths = append(paths, p)
	}
	return paths
}
