// Package status builds the public status pages: one per product, each with
// its apps, their uptime over the last 90 days, their outages over the last
// 30, and the notes the owner wrote on them. Pings, daily counts, and
// outages come from the uptime context; this package stores the pages and
// the notes.
package status

import (
	"errors"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"gorm.io/gorm"

	"lognorth/internal/apikeys"
	"lognorth/internal/uptime"
)

// Days is how many daily bars the page draws. The all-time uptime goes back
// to the first ping. Outages need the pings, so they go back 30 days.
const Days = 90

const maxCommentLength = 1000

var (
	ErrCommentEmpty   = errors.New("Write something first")
	ErrCommentTooLong = errors.New("Too long — 1,000 characters max")
	ErrNoOutage       = errors.New("Can't find that outage on the status page")
)

// Comment is a note the owner writes on an outage. Outages are not stored:
// each one is a run of failed pings, so a comment points at its outage by
// the app and the first failed ping.
type Comment struct {
	ID          uint      `gorm:"primaryKey" json:"id"`
	APIKeyID    uint      `gorm:"not null;index:idx_status_comment_outage,priority:1" json:"-"`
	OutageStart time.Time `gorm:"not null;index:idx_status_comment_outage,priority:2" json:"-"`
	Body        string    `gorm:"size:1000;not null" json:"body"`
	CreatedAt   time.Time `json:"created_at"`
}

func (Comment) TableName() string { return "status_comments" }

// Report is everything one status page shows.
type Report struct {
	Page      Page       `json:"page"`
	Apps      []App      `json:"apps"`
	Incidents []Incident `json:"incidents"` // newest first
}

// App is one app on the page. It carries no URL: the page is public, and a
// ping URL can point at something private.
type App struct {
	ID        uint         `json:"id"`
	Name      string       `json:"name"`
	Status    string       `json:"status"` // as uptime.State
	LatencyMS int          `json:"latency_ms,omitempty"`
	CheckedAt *time.Time   `json:"checked_at,omitempty"`
	DownSince *time.Time   `json:"down_since,omitempty"`
	Uptime    *float64     `json:"uptime,omitempty"` // percent of the window's pings that were up
	Days      []uptime.Day `json:"days"`
	Since     string       `json:"since,omitempty"`    // the first day with pings, "2026-03-04"
	AllTime   *float64     `json:"all_time,omitempty"` // percent of all pings since then that were up
}

// Incident is one outage of an app, with the owner's notes on it.
type Incident struct {
	AppID    uint      `json:"app_id"`
	AppName  string    `json:"app_name"`
	Start    time.Time `json:"start"`
	End      time.Time `json:"end"`
	Ongoing  bool      `json:"ongoing,omitempty"`
	Comments []Comment `json:"comments"` // oldest first
}

// Build reads one page: its apps by name, and their outages.
func Build(db *gorm.DB, page Page, now time.Time) Report {
	keys := appsOn(db, page.ID)
	states := uptime.States(db, keys, now)
	comments := commentsByOutage(db, keys)

	report := Report{Page: page, Apps: []App{}, Incidents: []Incident{}}
	for _, k := range keys {
		history := uptime.HistoryFor(db, k.ID, now, Days)
		state := states[k.ID]
		report.Apps = append(report.Apps, App{
			ID:        k.ID,
			Name:      k.Name,
			Status:    state.Status,
			LatencyMS: state.LatencyMS,
			CheckedAt: state.CheckedAt,
			DownSince: state.DownSince,
			Uptime:    history.Uptime,
			Days:      history.Days,
			Since:     history.Since,
			AllTime:   history.AllTime,
		})
		for _, o := range history.Outages {
			notes := comments[outageKey{k.ID, o.Start.UnixNano()}]
			if notes == nil {
				notes = []Comment{}
			}
			report.Incidents = append(report.Incidents, Incident{AppID: k.ID, AppName: k.Name, Start: o.Start, End: o.End, Ongoing: o.Ongoing, Comments: notes})
		}
	}
	slices.SortFunc(report.Incidents, func(a, b Incident) int { return b.Start.Compare(a.Start) })
	return report
}

// AddComment adds a note to the app's outage that started at `start`. The
// outage must be on the page now.
func AddComment(db *gorm.DB, appID uint, start time.Time, body string, now time.Time) error {
	body = strings.TrimSpace(body)
	if body == "" {
		return ErrCommentEmpty
	}
	if utf8.RuneCountInString(body) > maxCommentLength {
		return ErrCommentTooLong
	}
	outage, ok := findOutage(db, appID, start, now)
	if !ok {
		return ErrNoOutage
	}
	return db.Create(&Comment{APIKeyID: appID, OutageStart: outage.Start, Body: body}).Error
}

// DeleteComment removes a note.
func DeleteComment(db *gorm.DB, id uint) error {
	return db.Delete(&Comment{}, id).Error
}

// DeleteOlderThan removes the notes on outages that started before the
// cutoff. Their pings are gone, so the page no longer shows them.
func DeleteOlderThan(db *gorm.DB, cutoff time.Time) (int64, error) {
	result := db.Where("outage_start < ?", cutoff).Delete(&Comment{})
	return result.RowsAffected, result.Error
}

func appsOn(db *gorm.DB, pageID uint) []apikeys.APIKey {
	var keys []apikeys.APIKey
	db.Where("active = ? AND status_page_id = ?", true, pageID).Order("name").Find(&keys)
	return keys
}

func findOutage(db *gorm.DB, appID uint, start time.Time, now time.Time) (uptime.Outage, bool) {
	var key apikeys.APIKey
	if err := db.Where("id = ? AND active = ? AND status_page_id <> 0", appID, true).First(&key).Error; err != nil {
		return uptime.Outage{}, false
	}
	for _, o := range uptime.HistoryFor(db, appID, now, Days).Outages {
		if o.Start.Equal(start) {
			return o, true
		}
	}
	return uptime.Outage{}, false
}

type outageKey struct {
	appID uint
	start int64 // UnixNano of the first failed ping
}

func commentsByOutage(db *gorm.DB, keys []apikeys.APIKey) map[outageKey][]Comment {
	byOutage := map[outageKey][]Comment{}
	if len(keys) == 0 {
		return byOutage
	}
	ids := make([]uint, 0, len(keys))
	for _, k := range keys {
		ids = append(ids, k.ID)
	}
	var rows []Comment
	db.Where("api_key_id IN ?", ids).Order("created_at, id").Find(&rows)
	for _, c := range rows {
		key := outageKey{c.APIKeyID, c.OutageStart.UnixNano()}
		byOutage[key] = append(byOutage[key], c)
	}
	return byOutage
}
