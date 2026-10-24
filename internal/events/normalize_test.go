package events

import "testing"

func TestNormalizeMessage(t *testing.T) {
	for _, c := range []struct{ message, want string }{
		{"Couldn't find User with 'id'=1001", "Couldn't find User with 'id'=<n>"},
		{"Couldn't find User with 'id'=987654321", "Couldn't find User with 'id'=<n>"},
		{"timeout after 30s", "timeout after <n>s"},
		{"no row for 550e8400-e29b-41d4-a716-446655440000", "no row for <uuid>"},
		{"no row for 550E8400-E29B-41D4-A716-446655440000", "no row for <uuid>"},
		{"user foo@bar.com not found", "user <email> not found"},
		{"runtime error: invalid memory address 0xc000123abc", "runtime error: invalid memory address <hex>"},
		{"HTTP 500", "HTTP <n>"},
		{"dial tcp 10.0.0.5:5432: connect: connection refused", "dial tcp <n>:<n>: connect: connection refused"},
		{"price 19.99 is invalid", "price <n> is invalid"},
		{"commit a1b2c3d4e5 not found", "commit <hex> not found"},
		{"session user_1001 expired", "session user_<n> expired"},
		{"undefined method 'name' for nil", "undefined method 'name' for nil"},
		{"cafebabe is a word without a digit", "cafebabe is a word without a digit"},
		{"", ""},
	} {
		t.Run(c.message, func(t *testing.T) {
			if got := normalizeMessage(c.message); got != c.want {
				t.Errorf("normalizeMessage(%q) = %q, want %q", c.message, got, c.want)
			}
		})
	}
}

func TestComputeErrorHashesIgnoreIDs(t *testing.T) {
	t.Run("messages that differ by an id are one issue", func(t *testing.T) {
		h1a, h2a, h3a := computeErrorHashes("ActiveRecord::RecordNotFound", "Couldn't find User with 'id'=1001", "users_controller.rb", 12, 1)
		h1b, h2b, h3b := computeErrorHashes("ActiveRecord::RecordNotFound", "Couldn't find User with 'id'=1002", "users_controller.rb", 12, 1)

		if h1a != h1b || h2a != h2b || h3a != h3b {
			t.Error("an id in the message should not change the hashes")
		}
	})

	t.Run("messages that differ by a word stay apart", func(t *testing.T) {
		h1a, _, _ := computeErrorHashes("NoMethodError", "undefined method 'name' for nil", "", 0, 1)
		h1b, _, _ := computeErrorHashes("NoMethodError", "undefined method 'email' for nil", "", 0, 1)

		if h1a == h1b {
			t.Error("different words should give different hashes")
		}
	})

	t.Run("the stored message keeps the id", func(t *testing.T) {
		ctx := &EventContext{ErrorClass: "ActiveRecord::RecordNotFound", Error: "Couldn't find User with 'id'=1001"}
		event := &Event{APIKeyID: 1, IsError: true}

		PopulateErrorFields(event, ctx)

		if event.ErrorMessage != "Couldn't find User with 'id'=1001" {
			t.Errorf("error_message = %q, want the raw message", event.ErrorMessage)
		}
	})
}
