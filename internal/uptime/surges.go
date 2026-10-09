package uptime

import (
	"slices"
	"time"

	"gorm.io/gorm"

	"lognorth/internal/events"
)

// OutageSurges is an outage with the issues that surged around it (see
// events.SurgedDuring). Issues is empty when nothing stood out, which is the
// honest answer more often than not: the app may be down for a reason that
// logs no error.
type OutageSurges struct {
	Outage
	Issues []events.Surge `json:"issues"`
}

// RecentOutages returns the app's outages since the cutoff, newest first, at
// most `limit`, each with the issues that surged around it.
func RecentOutages(db *gorm.DB, appID uint, since time.Time, limit int) ([]OutageSurges, error) {
	outages := Outages(db, appID, since)
	slices.Reverse(outages)
	if len(outages) > limit {
		outages = outages[:limit]
	}
	result := make([]OutageSurges, 0, len(outages))
	for _, o := range outages {
		issues, err := events.SurgedDuring(db, appID, o.Start, o.End)
		if err != nil {
			return nil, err
		}
		result = append(result, OutageSurges{Outage: o, Issues: issues})
	}
	return result, nil
}

// IssueOutage is an outage in which one issue surged, with its errors there.
type IssueOutage struct {
	Outage
	Count int64 `json:"count"`
}

// SurgedIn returns the outages in which the issue surged.
func SurgedIn(outages []OutageSurges, hash string) []IssueOutage {
	var hit []IssueOutage
	for _, o := range outages {
		if i := slices.IndexFunc(o.Issues, func(s events.Surge) bool { return s.Hash == hash }); i >= 0 {
			hit = append(hit, IssueOutage{Outage: o.Outage, Count: o.Issues[i].Count})
		}
	}
	return hit
}
