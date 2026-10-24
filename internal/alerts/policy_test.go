package alerts

import (
	"strings"
	"testing"
	"time"

	"lognorth/internal/events"
	"lognorth/internal/settings"
)

// The pieces of the policy, one table each: when an app's alerts go out, how
// an email names them, and the rules the endpoint checks apply.

func TestDue(t *testing.T) {
	now := scenarioStart
	alertAt := func(kind string, ago time.Duration) []Alert {
		return []Alert{{Kind: kind, CreatedAt: now.Add(-ago)}}
	}
	never := time.Time{}

	for _, c := range []struct {
		name                string
		pending             []Alert
		lastEmail, lastDown time.Time
		want                bool
	}{
		{"the first alert after a quiet day goes out once it settled", alertAt(KindNewError, time.Minute), never, never, true},
		{"an alert waits for others of its burst", alertAt(KindNewError, 10*time.Second), never, never, false},
		{"an alert just past settle goes out", alertAt(KindNewError, settle), never, never, true},
		{"an alert waits within 10 minutes of the last email", alertAt(KindNewError, time.Minute), now.Add(-5 * time.Minute), never, false},
		{"an alert goes out 10 minutes after the last email", alertAt(KindNewError, time.Minute), now.Add(-followUpGap), never, true},
		{"the first down of an incident does not wait", alertAt(KindDown, time.Minute), now.Add(-2 * time.Minute), never, true},
		{"a down waits when a down went out 20 minutes ago", alertAt(KindDown, time.Minute), now.Add(-5 * time.Minute), now.Add(-20 * time.Minute), false},
		{"a down goes at once when the last down was an hour ago", alertAt(KindDown, time.Minute), now.Add(-5 * time.Minute), now.Add(-time.Hour), true},
		{"a recovery waits for the gap like any alert", alertAt(KindRecovered, time.Minute), now.Add(-3 * time.Minute), now.Add(-3 * time.Minute), false},
		{"a spike waits for the gap", alertAt(KindSpike, time.Minute), now.Add(-3 * time.Minute), never, false},
		{"a down among other alerts takes them with it", append(alertAt(KindNewError, 2*time.Minute), alertAt(KindDown, time.Minute)...), now.Add(-3 * time.Minute), never, true},
	} {
		t.Run(c.name, func(t *testing.T) {
			if got := due(c.pending, c.lastEmail, c.lastDown, now); got != c.want {
				t.Errorf("due = %v, want %v", got, c.want)
			}
		})
	}
}

func TestCountKinds(t *testing.T) {
	kinds := func(ks ...string) []Alert {
		var alerts []Alert
		for _, k := range ks {
			alerts = append(alerts, Alert{Kind: k})
		}
		return alerts
	}
	for _, c := range []struct {
		alerts []Alert
		want   string
	}{
		{kinds(KindNewError, KindNewError, KindNewError), "3 new errors"},
		{kinds(KindNewError), "1 new error"},
		{kinds(KindRegression, KindRegression), "2 errors back"},
		{kinds(KindNewError, KindRegression), "1 new error, 1 back"},
		{kinds(KindNewError, KindNewError, KindRegression), "2 new errors, 1 back"},
		{kinds(KindDown, KindRecovered), "down and recovered"},
		{kinds(KindDown, KindDown, KindRecovered), "down 2 times and recovered"},
		{kinds(KindSpike, KindSpike, KindDown), "down, 2 endpoints failing"},
		{kinds(KindSilence), "1 endpoint silent"},
		{kinds(KindLeak, KindLeak), "logged 2 secrets"},
		{kinds(KindRecovered, KindNewError), "1 new error, recovered"},
		{kinds(KindLeak, KindSpike, KindNewError, KindDown, KindSilence), "down, 1 endpoint failing, 1 endpoint silent, 1 new error, logged a secret"},
	} {
		t.Run(c.want, func(t *testing.T) {
			if got := countKinds(c.alerts); got != c.want {
				t.Errorf("countKinds = %q, want %q", got, c.want)
			}
		})
	}
}

func TestCompose(t *testing.T) {
	s := &settings.Settings{BaseURL: "https://logs.example.com", TimeZone: "UTC"}
	at := scenarioStart

	t.Run("one alert keeps its own subject and body", func(t *testing.T) {
		subject, body := compose(s, "shop", 1, []Alert{{Kind: KindNewError, Title: "new: TypeError: boom", Body: "TypeError: boom\nat app.ts:7", CreatedAt: at}}, at)

		if subject != "shop — new: TypeError: boom" || body != "TypeError: boom\nat app.ts:7" {
			t.Errorf("got %q / %q", subject, body)
		}
	})

	t.Run("several are counted and listed with their time, without repeating a headline", func(t *testing.T) {
		subject, body := compose(s, "shop", 1, []Alert{
			{Kind: KindNewError, Title: "new: TypeError: boom", Body: "TypeError: boom\nat app.ts:7", CreatedAt: at},
			{Kind: KindDown, Title: "down: https://shop/up", Body: "shop isn't responding.", CreatedAt: at.Add(3 * time.Minute)},
		}, at.Add(4*time.Minute))

		if subject != "shop — down, 1 new error" {
			t.Errorf("subject = %q", subject)
		}
		for _, want := range []string{"12:00  new: TypeError: boom\n  at app.ts:7", "12:03  down: https://shop/up\n  shop isn't responding."} {
			if !strings.Contains(body, want) {
				t.Errorf("body lacks %q:\n%s", want, body)
			}
		}
		if strings.Count(body, "TypeError: boom") != 1 {
			t.Errorf("the headline repeats:\n%s", body)
		}
	})

	t.Run("more than ten list ten and link the rest", func(t *testing.T) {
		var many []Alert
		for range 13 {
			many = append(many, Alert{Kind: KindSpike, Title: "spike: /x at 50%", Body: "/x is at 50% errors", CreatedAt: at})
		}

		_, body := compose(s, "shop", 7, many, at)

		if strings.Count(body, "spike: /x") != 10 || !strings.Contains(body, "And 3 more: https://logs.example.com/apps/7/issues.") {
			t.Errorf("body:\n%s", body)
		}
	})

	t.Run("times are in the owner's zone", func(t *testing.T) {
		madrid := &settings.Settings{TimeZone: "Europe/Madrid"}

		_, body := compose(madrid, "shop", 1, []Alert{
			{Kind: KindSpike, Title: "spike: /a", Body: "a", CreatedAt: at},
			{Kind: KindSpike, Title: "spike: /b", Body: "b", CreatedAt: at},
		}, at)

		if !strings.Contains(body, "14:00  spike: /a") { // 12:00 UTC is 14:00 CEST
			t.Errorf("body:\n%s", body)
		}
	})

	t.Run("an alert held through quiet hours says when it came in", func(t *testing.T) {
		quiet := &settings.Settings{TimeZone: "UTC", QuietHoursEnabled: true, QuietStart: 23, QuietEnd: 7}
		threeAM := time.Date(2026, 10, 15, 3, 15, 0, 0, time.UTC)

		subject, body := compose(quiet, "shop", 1, []Alert{{Kind: KindNewError, Title: "new: X", Body: "X", CreatedAt: threeAM}}, threeAM.Add(4*time.Hour))

		if subject != "shop — new: X" || !strings.HasPrefix(body, "This came in at 03:15, in your quiet hours.") {
			t.Errorf("got %q / %q", subject, body)
		}
	})
}

func TestIsQuietHours(t *testing.T) {
	at := func(h int) time.Time { return time.Date(2026, 10, 15, h, 30, 0, 0, time.UTC) }
	for _, c := range []struct {
		name       string
		start, end int
		zone       string
		hour       int
		want       bool
	}{
		{"inside a window that crosses midnight", 23, 7, "UTC", 2, true},
		{"after it", 23, 7, "UTC", 7, false},
		{"its first hour", 23, 7, "UTC", 23, true},
		{"the hour before it", 23, 7, "UTC", 22, false},
		{"a daytime window", 12, 14, "UTC", 13, true},
		{"outside a daytime window", 12, 14, "UTC", 14, false},
		{"in the owner's zone: 05:30 UTC is 07:30 in Madrid", 23, 7, "Europe/Madrid", 5, false},
		{"in the owner's zone: 04:30 UTC is 06:30 in Madrid", 23, 7, "Europe/Madrid", 4, true},
		{"an unknown zone falls back to UTC", 23, 7, "Mars/Olympus", 2, true},
		{"no zone is UTC", 23, 7, "", 6, true},
	} {
		t.Run(c.name, func(t *testing.T) {
			s := &settings.Settings{QuietHoursEnabled: true, QuietStart: c.start, QuietEnd: c.end, TimeZone: c.zone}
			if got := IsQuietHours(s, at(c.hour)); got != c.want {
				t.Errorf("IsQuietHours at %02d:30 = %v, want %v", c.hour, got, c.want)
			}
		})
	}

	t.Run("off when quiet hours are off", func(t *testing.T) {
		if IsQuietHours(&settings.Settings{QuietStart: 0, QuietEnd: 23}, at(5)) {
			t.Error("quiet hours are off")
		}
	})
}

func TestErrorRateSeverity(t *testing.T) {
	for _, c := range []struct {
		name    string
		current float64
		total   int64
		rate    float64
		want    string
	}{
		{"2% to 40% on 200 requests is critical", 40, 200, 2, "critical"},
		{"2% to 3% stays below 5σ", 3, 200, 2, ""},
		{"2% to 3.5% on 20,000 requests is 5σ but not double: a warning", 3.5, 20000, 2, "warning"},
		{"0.1% to 0.6% on 50,000 requests is under one point: a warning", 0.6, 50000, 0.1, "warning"},
		{"0.1% to 2% on 50,000 requests is critical", 2, 50000, 0.1, "critical"},
		{"10 requests are too few to judge", 100, 10, 1, ""},
		{"no change", 2, 1000, 2, ""},
		{"fewer errors than normal never alerts", 0, 1000, 5, ""},
	} {
		t.Run(c.name, func(t *testing.T) {
			_, got := errorRateSeverity(c.current, c.total, c.rate, 1, true)
			if got != c.want {
				t.Errorf("severity = %q, want %q", got, c.want)
			}
		})
	}

	t.Run("nothing before the history is ready", func(t *testing.T) {
		if _, got := errorRateSeverity(90, 1000, 1, 1, false); got != "" {
			t.Errorf("severity = %q", got)
		}
	})
}

func TestRobustRate(t *testing.T) {
	bucket := func(total, errors int64) events.EndpointStat {
		return events.EndpointStat{Total: total, Errors: errors}
	}
	var normal []events.EndpointStat
	for i := range 84 {
		normal = append(normal, bucket(200, int64(i%3+1))) // about 1%
	}

	t.Run("a history without outliers keeps its pooled rate", func(t *testing.T) {
		if got, want := robustRate(normal), sumStats(normal).ErrorRate(); got != want {
			t.Errorf("rate = %.2f, want %.2f", got, want)
		}
	})

	t.Run("a past outage does not raise the normal rate", func(t *testing.T) {
		withOutage := append(append([]events.EndpointStat{}, normal...), bucket(200, 100), bucket(200, 110), bucket(200, 95))

		if got := robustRate(withOutage); got > 1.2 {
			t.Errorf("rate = %.2f%%, want about 1%% (pooled with the outage it is %.2f%%)", got, sumStats(withOutage).ErrorRate())
		}
	})

	t.Run("an empty history is 0", func(t *testing.T) {
		if got := robustRate(nil); got != 0 {
			t.Errorf("rate = %v", got)
		}
	})
}

func TestAloneCritical(t *testing.T) {
	baseline := ErrorRateBaseline{Rate: 1, Dispersion: 1}
	for _, c := range []struct {
		name          string
		total, errors int64
		want          bool
	}{
		{"a bucket at 50% of 200", 200, 100, true},
		{"a bucket at 2% of 200", 200, 4, false},
		{"a small bucket where all 10 failed", 10, 10, true},
		{"a small bucket with one error", 10, 1, false},
		{"an empty bucket", 0, 0, false},
	} {
		t.Run(c.name, func(t *testing.T) {
			if got := aloneCritical(events.EndpointStat{Total: c.total, Errors: c.errors}, baseline); got != c.want {
				t.Errorf("aloneCritical = %v, want %v", got, c.want)
			}
		})
	}
}

func TestSmallEndpointFailing(t *testing.T) {
	stat := func(total, errors int64) events.EndpointStat {
		return events.EndpointStat{Total: total, Errors: errors}
	}
	quiet := ErrorRateBaseline{Rate: 1, Requests: 5000}
	for _, c := range []struct {
		name           string
		bucket, last15 events.EndpointStat
		baseline       ErrorRateBaseline
		want           bool
	}{
		{"all failing for 15 minutes", stat(10, 10), stat(30, 30), quiet, true},
		{"half failing, 6 errors", stat(4, 2), stat(12, 6), quiet, true},
		{"4 errors are too few", stat(4, 4), stat(8, 4), quiet, false},
		{"the bucket just checked is fine: it is over", stat(10, 0), stat(30, 20), quiet, false},
		{"a busy endpoint: the spike check judges it", stat(200, 200), stat(600, 600), quiet, false},
		{"an endpoint that normally fails 20%", stat(10, 10), stat(30, 30), ErrorRateBaseline{Rate: 20, Requests: 5000}, false},
		{"no history to call normal", stat(10, 10), stat(30, 30), ErrorRateBaseline{Rate: 0, Requests: 5}, false},
		{"no requests in the bucket", stat(0, 0), stat(30, 30), quiet, false},
	} {
		t.Run(c.name, func(t *testing.T) {
			if got := smallEndpointFailing(c.bucket, c.last15, c.baseline); got != c.want {
				t.Errorf("smallEndpointFailing = %v, want %v", got, c.want)
			}
		})
	}
}

func TestStillDrifting(t *testing.T) {
	baseline := ErrorRateBaseline{HourRate: 1}
	stat := func(total, errors int64) events.EndpointStat {
		return events.EndpointStat{Total: total, Errors: errors}
	}
	for _, c := range []struct {
		name   string
		drift  string
		last15 events.EndpointStat
		want   string
	}{
		{"a drift still on stays critical", "critical", stat(600, 30), "critical"},
		{"a drift back to normal is over", "critical", stat(600, 3), ""},
		{"a drift above normal but under the floor is a warning", "critical", stat(600, 9), "warning"},
		{"a warning still on stays a warning", "warning", stat(600, 9), "warning"},
		{"no drift stays none", "", stat(600, 300), ""},
	} {
		t.Run(c.name, func(t *testing.T) {
			if got := stillDrifting(c.drift, c.last15, baseline); got != c.want {
				t.Errorf("stillDrifting = %q, want %q", got, c.want)
			}
		})
	}
}

func TestLastCompleteBucket(t *testing.T) {
	for _, c := range []struct{ now, want string }{
		{"12:06:00", "12:00"}, // the bucket 12:00-12:05 ended a minute ago
		{"12:05:30", "11:55"}, // 12:00-12:05 ended 30 s ago: it can still fill
		{"12:09:59", "12:00"},
		{"12:11:00", "12:05"},
	} {
		now, _ := time.Parse("15:04:05", c.now)
		now = time.Date(2026, 10, 14, now.Hour(), now.Minute(), now.Second(), 0, time.UTC)
		if got := events.LastCompleteBucket(now).Format("15:04"); got != c.want {
			t.Errorf("at %s the bucket checked is %s, want %s", c.now, got, c.want)
		}
	}
}
