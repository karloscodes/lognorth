package http

import (
	"github.com/karloscodes/cartridge"

	"lognorth/internal/apikeys"
	"lognorth/internal/events"
)

// AppWithStats represents an app with its statistics
type AppWithStats struct {
	ID    uint     `json:"id"`
	Name  string   `json:"name"`
	Stats AppStats `json:"stats"`
}

// AppStats holds per-app statistics
type AppStats struct {
	Total  int64 `json:"total"`
	Errors int64 `json:"errors"`
}

// HomePageAction handles GET /
func HomePageAction(ctx *cartridge.Context) error {
	db := ctx.DB()

	if !apikeys.HasAny(db) {
		return ctx.Ctx.Redirect("/apps")
	}

	// Get applications with stats
	keys, _ := apikeys.List(db)
	apps := make([]AppWithStats, 0, len(keys))
	for _, k := range keys {
		if k.Active {
			stats := events.GetStats(db, k.ID)
			apps = append(apps, AppWithStats{
				ID:   k.ID,
				Name: k.Name,
				Stats: AppStats{
					Total:  stats.Total,
					Errors: stats.Errors,
				},
			})
		}
	}

	// Get all issues across all apps for accurate counts
	recentIssues, err := events.ListIssuesWithState(db, 0, 200)
	if err != nil {
		ctx.Logger.Error("Failed to list issues", "error", err)
		recentIssues = []events.IssueWithState{}
	}

	return RenderPage(ctx.Ctx, "Home", Props{
		"applications": apps,
		"recentIssues": recentIssues,
	})
}
