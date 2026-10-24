package alerts

import (
	"testing"
	"time"

	"lognorth/internal/events"
)

// An id in the message does not make a new issue, so it sends no new alert.
func TestCheckErrorIgnoresIDsInTheMessage(t *testing.T) {
	db := newTestDB(t)
	var news int
	for _, id := range []string{"1001", "1002", "1003"} {
		event := &events.Event{APIKeyID: 1, Message: "failed", IsError: true,
			Context: `{"error_class":"ActiveRecord::RecordNotFound","error":"Couldn't find User with 'id'=` + id + `","error_file":"users_controller.rb","error_line":12}`}
		if err := events.Create(db, event); err != nil {
			t.Fatal(err)
		}

		alert, err := CheckError(db, event, "", time.Now())

		if err != nil {
			t.Fatal(err)
		}
		if alert != nil {
			news++
		}
	}
	if news != 1 {
		t.Errorf("%d new alerts, want 1", news)
	}
}
