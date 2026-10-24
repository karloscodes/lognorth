package events

import "testing"

// The data migration builds issues at boot: an event whose context is not
// JSON must not stop the server.
func TestRecountIssuesSkipsAContextThatIsNotJSON(t *testing.T) {
	db := newTestDB(t)
	db.Create(&Event{APIKeyID: 1, IsError: true, IssueHash: "h", ErrorMessage: "boom", Context: ""})
	db.Create(&Event{APIKeyID: 1, IsError: true, IssueHash: "h", ErrorMessage: "boom", Context: `{"path":"/pay"}`})

	err := RecountIssues(db, []string{"h"})

	if err != nil {
		t.Fatalf("RecountIssues: %v", err)
	}
	var issue Issue
	db.Where("hash = ?", "h").First(&issue)
	if issue.Count != 2 || issue.Paths != `["/pay"]` {
		t.Errorf("issue = count %d, paths %s; want 2 and [\"/pay\"]", issue.Count, issue.Paths)
	}
}
