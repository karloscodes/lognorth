package events

import (
	"time"

	"gorm.io/gorm"
)

// An issue is a symptom of another when the other one failed first in the
// same request (the same trace ID) in most of the requests the symptom
// failed in, and the other one is rare in requests in general. The trace ID
// is a direct link, and the lift test keeps out an error that happens in
// almost every request: it comes first everywhere, so it explains nothing.
const (
	chainWindow    = 7 * 24 * time.Hour
	chainMinTraces = 5   // fewer shared requests than this is too little to say
	chainMinShare  = 0.8 // the cause came first in this share of the symptom's requests
	chainMinLift   = 5   // and is this many times more common there than in all requests
)

// IssueLink is one issue failing first in another issue's requests.
type IssueLink struct {
	Symptom    string `json:"-"`
	Cause      string `json:"-"`
	Hash       string `json:"hash"` // the other issue: the cause, or the symptom
	Error      string `json:"error"`
	ErrorClass string `json:"error_class,omitempty"`
	Traces     int64  `json:"traces"` // requests where the cause failed first
	Of         int64  `json:"of"`     // requests the symptom failed in
}

// IssueChains is an issue's causes (issues that failed first in its
// requests) and its symptoms (issues it failed before in theirs).
type IssueChains struct {
	Causes   []IssueLink `json:"causes"`
	Symptoms []IssueLink `json:"symptoms"`
}

// ChainsOf returns the causes and the symptoms of one issue.
func ChainsOf(db *gorm.DB, appID uint, hash string, now time.Time) (IssueChains, error) {
	links, err := chains(db, appID, now)
	chains := IssueChains{Causes: []IssueLink{}, Symptoms: []IssueLink{}}
	if err != nil {
		return chains, err
	}
	for _, l := range links {
		switch hash {
		case l.Symptom:
			l.Hash = l.Cause
			chains.Causes = append(chains.Causes, l)
		case l.Cause:
			l.Hash = l.Symptom
			chains.Symptoms = append(chains.Symptoms, l)
		}
	}
	if err := issueNames(db, appID, chains.Causes); err != nil {
		return chains, err
	}
	return chains, issueNames(db, appID, chains.Symptoms)
}

// CausesByIssue returns each of the app's issues that has a cause, with its
// causes, for the issue list and the list_issues tool.
func CausesByIssue(db *gorm.DB, appID uint, now time.Time) (map[string][]IssueLink, error) {
	links, err := chains(db, appID, now)
	if err != nil {
		return nil, err
	}
	for i := range links {
		links[i].Hash = links[i].Cause
	}
	if err := issueNames(db, appID, links); err != nil {
		return nil, err
	}
	byIssue := map[string][]IssueLink{}
	for _, l := range links {
		byIssue[l.Symptom] = append(byIssue[l.Symptom], l)
	}
	return byIssue, nil
}

// chains returns every cause and symptom pair of the app in the window.
func chains(db *gorm.DB, appID uint, now time.Time) ([]IssueLink, error) {
	since := now.Add(-chainWindow).UTC()

	// Each symptom's requests, from its first event in each one; then every
	// other issue that failed before that event in the same request.
	var links []IssueLink
	err := db.Raw(`
		WITH symptom AS (
			SELECT issue_hash, trace_id, MIN(timestamp) AS at, MIN(id) AS first_id
			FROM events
			WHERE api_key_id = ? AND is_error = 1 AND issue_hash != '' AND trace_id != '' AND timestamp >= ?
			GROUP BY issue_hash, trace_id
		),
		totals AS (SELECT issue_hash, COUNT(*) AS n FROM symptom GROUP BY issue_hash),
		pairs AS (
			SELECT s.issue_hash AS symptom, c.issue_hash AS cause, COUNT(DISTINCT s.trace_id) AS traces
			FROM symptom s
			JOIN events c ON c.trace_id = s.trace_id AND c.api_key_id = ? AND c.is_error = 1
				AND c.issue_hash != '' AND c.issue_hash != s.issue_hash
				AND (c.timestamp < s.at OR (c.timestamp = s.at AND c.id < s.first_id))
			GROUP BY s.issue_hash, c.issue_hash
		)
		SELECT p.symptom, p.cause, p.traces, t.n AS of
		FROM pairs p JOIN totals t ON t.issue_hash = p.symptom
		WHERE p.traces >= ? AND p.traces >= ? * t.n
		ORDER BY p.traces DESC
	`, appID, since, appID, chainMinTraces, chainMinShare).Scan(&links).Error
	if err != nil || len(links) == 0 {
		return nil, err
	}

	// The lift test: how common is each cause in all requests of the window?
	var requests int64
	db.Model(&EndpointStat{}).Select("COALESCE(SUM(total), 0)").
		Where("api_key_id = ? AND bucket >= ?", appID, since).Scan(&requests)
	if requests == 0 {
		return nil, nil
	}
	strong := links[:0]
	for _, l := range links {
		var causeTraces int64
		db.Raw(`SELECT COUNT(DISTINCT trace_id) FROM events
			WHERE api_key_id = ? AND issue_hash = ? AND trace_id != '' AND timestamp >= ?`,
			appID, l.Cause, since).Scan(&causeTraces)
		inSymptom := float64(l.Traces) / float64(l.Of)
		overall := float64(causeTraces) / float64(requests)
		if overall > 0 && inSymptom/overall >= chainMinLift {
			strong = append(strong, l)
		}
	}
	return strong, nil
}

// issueNames fills in the error and class of each link's Hash.
func issueNames(db *gorm.DB, appID uint, links []IssueLink) error {
	for i := range links {
		var row struct {
			Error      string
			ErrorClass string
		}
		err := db.Raw(`SELECT error_message AS error, error_class FROM events
			WHERE api_key_id = ? AND issue_hash = ? ORDER BY id DESC LIMIT 1`, appID, links[i].Hash).Scan(&row).Error
		if err != nil {
			return err
		}
		links[i].Error, links[i].ErrorClass = row.Error, row.ErrorClass
	}
	return nil
}
