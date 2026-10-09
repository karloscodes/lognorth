package events

import (
	"strings"
	"testing"
)

func TestAlertBody(t *testing.T) {
	newEvent := func(release string) *Event {
		ctx := EventContext{ErrorClass: "TypeError", Error: "boom", ErrorFile: "app.ts", ErrorLine: 7, Release: release}
		event := &Event{Message: "boom", IsError: true}
		event.SetContext(&ctx)
		PopulateErrorFields(event, &ctx)
		return event
	}

	t.Run("names the release the error came in", func(t *testing.T) {
		body := alertBody(newEvent("a1b2c3d"), "https://logs.example.com")

		for _, want := range []string{"TypeError: boom", "at app.ts:7", "in release a1b2c3d", "/lognorth:investigate"} {
			if !strings.Contains(body, want) {
				t.Errorf("expected %q in:\n%s", want, body)
			}
		}
	})

	t.Run("leaves the release out when the SDK sent none", func(t *testing.T) {
		body := alertBody(newEvent(""), "https://logs.example.com")

		if strings.Contains(body, "release") {
			t.Errorf("expected no release line, got:\n%s", body)
		}
	})
}
