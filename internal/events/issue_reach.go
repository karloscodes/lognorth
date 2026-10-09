package events

import "gorm.io/gorm"

// IssueReach is who an issue hit and in which releases, read straight from
// its own events. The SDKs stamp user and release on each error, so these are
// facts about the issue, not guesses. Each field is empty when no event of the
// issue carries it.
type IssueReach struct {
	Users        int64  `json:"users"`
	Releases     int64  `json:"releases"`
	FirstRelease string `json:"first_release,omitempty"`
	LastRelease  string `json:"last_release,omitempty"`
}

// GetIssueReach counts the distinct users and releases of an issue's errors,
// and names the release it first and last appeared in.
func GetIssueReach(db *gorm.DB, hash string, apiKeyID uint) (IssueReach, error) {
	var reach IssueReach
	err := db.Raw(`
		SELECT
			COUNT(DISTINCT json_extract(context, '$.user')) AS users,
			COUNT(DISTINCT json_extract(context, '$.release')) AS releases
		FROM events
		WHERE issue_hash = ? AND api_key_id = ? AND is_error = 1
	`, hash, apiKeyID).Scan(&reach).Error
	if err != nil || reach.Releases == 0 {
		return reach, err
	}

	release := func(order string) string {
		var r string
		db.Raw(`
			SELECT json_extract(context, '$.release') FROM events
			WHERE issue_hash = ? AND api_key_id = ? AND is_error = 1 AND json_extract(context, '$.release') IS NOT NULL
			ORDER BY timestamp `+order+`, id `+order+` LIMIT 1
		`, hash, apiKeyID).Scan(&r)
		return r
	}
	reach.FirstRelease = release("ASC")
	reach.LastRelease = release("DESC")
	return reach, nil
}
