package alerts

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"lognorth/internal/events"
	"lognorth/internal/settings"
)

// New errors and regressions, through the processor's path: the event is
// stored, CheckError decides, the alert is recorded, and Deliver sends it.

func TestScenarioErrors(t *testing.T) {
	t.Run("a new error emails once, within a minute", func(t *testing.T) {
		t.Parallel()
		w := newWorld(t)
		at := w.start.Add(10 * time.Minute)
		w.failError(at, "shop", "PaymentError", "card declined")

		w.run(time.Hour)

		got := w.expectEmails(1)
		w.within(got[0], at, 2*time.Minute)
		if got[0].Subject != "shop — new: PaymentError: card declined" {
			t.Errorf("subject = %q, want the one it always had", got[0].Subject)
		}
		for _, want := range []string{"at app/checkout.rb:42", "https://logs.example.com/issues/", "/lognorth:investigate "} {
			if !strings.Contains(got[0].Text, want) {
				t.Errorf("the email lacks %q:\n%s", want, got[0].Text)
			}
		}
	})

	t.Run("the same error a thousand times sends one email", func(t *testing.T) {
		t.Parallel()
		w := newWorld(t)
		for i := range 1000 {
			w.failError(w.start.Add(time.Duration(i)*3*time.Second), "shop", "PaymentError", "card declined")
		}

		w.run(2 * time.Hour)

		w.expectEmails(1)
	})

	t.Run("twenty new errors in one deploy send one email", func(t *testing.T) {
		t.Parallel()
		w := newWorld(t)
		deploy := w.start.Add(10 * time.Minute)
		for i := range 20 {
			w.failError(deploy.Add(time.Duration(i)*time.Second), "shop", fmt.Sprintf("Error%d", i), "boom")
		}

		w.run(time.Hour)

		got := w.expectEmails(1)
		if !strings.Contains(got[0].Subject, "shop — 20 new errors") {
			t.Errorf("subject = %q, want the count", got[0].Subject)
		}
		if strings.Count(got[0].Text, "new: ") != maxListed || !strings.Contains(got[0].Text, "And 10 more") {
			t.Errorf("want 10 listed and 10 more counted:\n%s", got[0].Text)
		}
	})

	t.Run("new errors that trickle in every minute send at most one email per 10 minutes", func(t *testing.T) {
		t.Parallel()
		w := newWorld(t)
		for i := range 30 {
			w.failError(w.start.Add(time.Duration(i)*time.Minute), "shop", fmt.Sprintf("Error%d", i), "boom")
		}

		w.run(time.Hour)

		got := w.expectAtMost(4)
		for i := 1; i < len(got); i++ {
			if gap := got[i].At.Sub(got[i-1].At); gap < followUpGap {
				t.Errorf("two emails %s apart, want at least %s", gap, followUpGap)
			}
		}
		total := 0
		for _, e := range got {
			total += strings.Count(e.Text, "new: ") + strings.Count(e.Subject, "— new: ")
		}
		if total < 30 && !strings.Contains(strings.Join(texts(got), ""), "more") {
			t.Errorf("the emails name %d of the 30 errors and do not say there are more", total)
		}
	})

	t.Run("an error back after a quiet day emails 'back after'", func(t *testing.T) {
		t.Parallel()
		w := newWorld(t)
		w.failError(w.start.Add(time.Minute), "shop", "PaymentError", "card declined")
		w.failError(w.start.Add(50*time.Hour), "shop", "PaymentError", "card declined")

		w.run(51 * time.Hour)

		got := w.expectEmails(2)
		if !strings.Contains(got[1].Subject, "back after 2d: PaymentError: card declined") {
			t.Errorf("subject = %q, want a regression", got[1].Subject)
		}
	})

	t.Run("an error back after 6 hours sends nothing more", func(t *testing.T) {
		t.Parallel()
		w := newWorld(t)
		w.failError(w.start.Add(time.Minute), "shop", "PaymentError", "card declined")
		w.failError(w.start.Add(6*time.Hour), "shop", "PaymentError", "card declined")

		w.run(7 * time.Hour)

		w.expectEmails(1)
	})

	t.Run("a muted error that comes back sends nothing", func(t *testing.T) {
		t.Parallel()
		w := newWorld(t)
		w.failError(w.start.Add(time.Minute), "shop", "PaymentError", "card declined")
		w.run(time.Hour)
		var issue events.IssueSeen
		w.db.First(&issue)
		events.MuteIssue(w.db, issue.ErrorHash)
		w.failError(w.start.Add(50*time.Hour), "shop", "PaymentError", "card declined")

		w.run(50 * time.Hour)

		w.expectEmails(1)
	})

	t.Run("errors in two apps send one email per app", func(t *testing.T) {
		t.Parallel()
		w := newWorld(t)
		at := w.start.Add(10 * time.Minute)
		w.failError(at, "shop", "PaymentError", "card declined")
		w.failError(at, "shop", "TimeoutError", "upstream")
		w.failError(at, "blog", "TemplateError", "missing partial")

		w.run(time.Hour)

		if len(w.emailsWith("shop — 2 new errors")) != 1 || len(w.emailsWith("blog — new: TemplateError")) != 1 {
			t.Errorf("want one email for each app:\n%s", describe(w.emails()))
		}
	})

	t.Run("a new error and one back share an email", func(t *testing.T) {
		t.Parallel()
		w := newWorld(t)
		w.failError(w.start.Add(time.Minute), "shop", "PaymentError", "card declined")
		later := w.start.Add(50 * time.Hour)
		w.failError(later, "shop", "PaymentError", "card declined")
		w.failError(later, "shop", "TimeoutError", "upstream")

		w.run(51 * time.Hour)

		got := w.expectEmails(2)
		if !strings.Contains(got[1].Subject, "shop — 1 new error, 1 back") {
			t.Errorf("subject = %q, want the new and the returning error counted", got[1].Subject)
		}
	})
}

func TestScenarioQuietHours(t *testing.T) {
	night := func(w *world) {
		w.settings(func(s *settings.Settings) {
			s.QuietHoursEnabled = true
			s.QuietStart, s.QuietEnd = 23, 7
		})
	}

	t.Run("a new error at 3 AM goes out at 7, once", func(t *testing.T) {
		t.Parallel()
		w := newWorld(t)
		night(w)
		threeAM := time.Date(2026, 10, 15, 3, 0, 0, 0, time.UTC)
		w.failError(threeAM, "shop", "PaymentError", "card declined")

		w.runUntil(time.Date(2026, 10, 15, 9, 0, 0, 0, time.UTC))

		got := w.expectEmails(1)
		if got[0].At.Hour() != 7 || got[0].At.Minute() > 2 {
			t.Errorf("went out at %s, want at 07:00", got[0].At.Format("15:04"))
		}
		if !strings.Contains(got[0].Text, "This came in at 03:00, in your quiet hours.") {
			t.Errorf("the email does not say when it came in:\n%s", got[0].Text)
		}
	})

	t.Run("a night of alerts goes out as one 'while you were off' email", func(t *testing.T) {
		t.Parallel()
		w := newWorld(t)
		night(w)
		for h, class := range []string{"PaymentError", "TimeoutError", "ParseError"} {
			w.failError(time.Date(2026, 10, 15, 1+h, 0, 0, 0, time.UTC), "shop", class, "boom")
		}

		w.runUntil(time.Date(2026, 10, 15, 9, 0, 0, 0, time.UTC))

		got := w.expectEmails(1)
		if got[0].Subject != "shop — while you were off: 3 new errors" {
			t.Errorf("subject = %q", got[0].Subject)
		}
		for _, at := range []string{"01:00", "02:00", "03:00"} {
			if !strings.Contains(got[0].Text, at+"  new: ") {
				t.Errorf("the email does not list the error of %s:\n%s", at, got[0].Text)
			}
		}
	})

	t.Run("quiet hours follow the owner's zone", func(t *testing.T) {
		t.Parallel()
		w := newWorld(t)
		night(w)
		w.settings(func(s *settings.Settings) { s.TimeZone = "America/New_York" })
		// 08:00 UTC is 04:00 in New York (EDT, UTC-4): quiet there.
		w.failError(time.Date(2026, 10, 15, 8, 0, 0, 0, time.UTC), "shop", "PaymentError", "card declined")

		w.runUntil(time.Date(2026, 10, 15, 13, 0, 0, 0, time.UTC))

		got := w.expectEmails(1)
		if got[0].At.Hour() != 11 { // 07:00 in New York
			t.Errorf("went out at %s UTC, want 11:00 UTC (07:00 in New York)", got[0].At.Format("15:04"))
		}
		if !strings.Contains(got[0].Text, "at 04:00") {
			t.Errorf("the email gives the time in UTC, want New York's:\n%s", got[0].Text)
		}
	})

	t.Run("quiet hours end at 7 local on the day the clocks change", func(t *testing.T) {
		t.Parallel()
		w := newWorld(t)
		night(w)
		w.settings(func(s *settings.Settings) { s.TimeZone = "Europe/Madrid" })
		// Madrid leaves summer time on Sunday 25 October 2026 at 03:00 CEST.
		w.start = time.Date(2026, 10, 25, 0, 0, 0, 0, time.UTC)
		w.now = w.start
		w.failError(time.Date(2026, 10, 25, 1, 30, 0, 0, time.UTC), "shop", "PaymentError", "card declined")

		w.runUntil(time.Date(2026, 10, 25, 9, 0, 0, 0, time.UTC))

		got := w.expectEmails(1)
		if got[0].At.Hour() != 6 { // 07:00 CET is 06:00 UTC
			t.Errorf("went out at %s UTC, want 06:00 UTC (07:00 in Madrid after the change)", got[0].At.Format("15:04"))
		}
	})

	t.Run("without quiet hours a 3 AM error goes out at once", func(t *testing.T) {
		t.Parallel()
		w := newWorld(t)
		threeAM := time.Date(2026, 10, 15, 3, 0, 0, 0, time.UTC)
		w.failError(threeAM, "shop", "PaymentError", "card declined")

		w.runUntil(time.Date(2026, 10, 15, 4, 0, 0, 0, time.UTC))

		got := w.expectEmails(1)
		w.within(got[0], threeAM, 2*time.Minute)
	})
}

func texts(got []email) []string {
	var all []string
	for _, e := range got {
		all = append(all, e.Text)
	}
	return all
}
