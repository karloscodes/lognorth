package http

import (
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"time"

	"github.com/karloscodes/cartridge"
	"gorm.io/gorm"

	"lognorth/internal/apikeys"
	"lognorth/internal/config"
	"lognorth/internal/settings"
	"lognorth/internal/status"
)

// Visitors get a page built at most once a minute: the pings change once a
// minute, and a status page gets its visitors all at once, during an outage.
// The signed-in owner always gets a fresh page, so a note shows right away.
var publicStatus = struct {
	sync.Mutex
	reports map[uint]cachedReport
}{reports: map[uint]cachedReport{}}

type cachedReport struct {
	builtAt time.Time
	report  status.Report
}

func cachedStatus(db *gorm.DB, page status.Page, now time.Time) status.Report {
	publicStatus.Lock()
	defer publicStatus.Unlock()
	cached, ok := publicStatus.reports[page.ID]
	if !ok || now.Sub(cached.builtAt) >= time.Minute || !cached.report.Page.UpdatedAt.Equal(page.UpdatedAt) {
		cached = cachedReport{builtAt: now, report: status.Build(db, page, now)}
		publicStatus.reports[page.ID] = cached
	}
	return cached.report
}

// renderStatus shows one page. On the page's own domain nobody is signed
// in: the session cookie belongs to LogNorth's domain.
func renderStatus(ctx *cartridge.Context, page status.Page, onOwnDomain bool) error {
	signedIn := !onOwnDomain && sessionAlive(ctx)
	var report status.Report
	if signedIn {
		report = status.Build(ctx.DB(), page, time.Now())
	} else {
		report = cachedStatus(ctx.DB(), page, time.Now())
	}
	return RenderPage(ctx, "Status", Props{"report": report, "signedIn": signedIn, "onOwnDomain": onOwnDomain})
}

// StatusPageAction handles GET /status/:slug. Anyone can read it.
func StatusPageAction(ctx *cartridge.Context) error {
	page, ok := status.PageBySlug(ctx.DB(), ctx.Params("slug"))
	if !ok {
		return ctx.Status(http.StatusNotFound).SendString("No status page here.")
	}
	return renderStatus(ctx, page, false)
}

// StatusDomainMiddleware serves a page on its own domain. There `/` is the
// page and its files load. Every other path goes back to `/`, so the
// dashboard and the login never answer on a status domain.
func StatusDomainMiddleware(isFile func(path string) bool) cartridge.HandlerFunc {
	return func(ctx *cartridge.Context) error {
		host := status.HostOnly(ctx.Hostname())
		if host == configHost() {
			return ctx.Next()
		}
		page, ok := status.PageByHost(ctx.DB(), host)
		if !ok || isFile(ctx.Path()) {
			return ctx.Next()
		}
		if ctx.Path() != "/" || (ctx.Method() != http.MethodGet && ctx.Method() != http.MethodHead) {
			return ctx.Redirect("/", http.StatusFound)
		}
		return renderStatus(ctx, page, true)
	}
}

// configHost is the host LogNorth was deployed on (BASE_URL), or "".
func configHost() string {
	u, err := url.Parse(config.GetConfig().GetBaseURL())
	if err != nil {
		return ""
	}
	return status.HostOnly(u.Host)
}

// ownHosts are the hosts LogNorth answers on: the deploy's, and the one
// saved in Settings for links in alerts.
func ownHosts(db *gorm.DB) []string {
	hosts := []string{configHost()}
	if s, err := settings.Get(db); err == nil {
		if u, err := url.Parse(s.BaseURL); err == nil && u.Host != "" {
			hosts = append(hosts, u.Host)
		}
	}
	return hosts
}

// AddStatusCommentAction handles POST /status/comments: a note on an outage.
func AddStatusCommentAction(ctx *cartridge.Context) error {
	appID, _ := strconv.ParseUint(ctx.FormValue("app_id"), 10, 64)
	start, err := time.Parse(time.RFC3339Nano, ctx.FormValue("start"))
	if err == nil {
		err = status.AddComment(ctx.DB(), uint(appID), start, ctx.FormValue("body"), time.Now())
	} else {
		err = status.ErrNoOutage
	}

	switch {
	case err == nil:
		setFlash(ctx, "success", "Note posted")
	case errors.Is(err, status.ErrCommentEmpty), errors.Is(err, status.ErrCommentTooLong), errors.Is(err, status.ErrNoOutage):
		setFlash(ctx, "error", err.Error())
	default:
		setFlash(ctx, "error", "Couldn't save the note")
	}
	return backToStatusPage(ctx)
}

// DeleteStatusCommentAction handles POST /status/comments/:id/delete.
func DeleteStatusCommentAction(ctx *cartridge.Context) error {
	id, err := ctx.ParamsInt("id")
	if err != nil || id <= 0 {
		setFlash(ctx, "error", "Can't find that note")
		return backToStatusPage(ctx)
	}
	if err := status.DeleteComment(ctx.DB(), uint(id)); err != nil {
		setFlash(ctx, "error", "Couldn't delete the note")
		return backToStatusPage(ctx)
	}
	setFlash(ctx, "success", "Note deleted")
	return backToStatusPage(ctx)
}

// backToStatusPage returns to the page the form was on.
func backToStatusPage(ctx *cartridge.Context) error {
	page, ok := status.PageBySlug(ctx.DB(), ctx.FormValue("page"))
	if !ok {
		return ctx.Redirect("/settings/status", http.StatusFound)
	}
	return ctx.Redirect("/status/"+page.Slug, http.StatusFound)
}

// statusPageRow is a page in Settings, with the names of its apps.
type statusPageRow struct {
	status.Page
	Apps []string `json:"apps"`
}

// SettingsStatusPageAction handles GET /settings/status
func SettingsStatusPageAction(ctx *cartridge.Context) error {
	db := ctx.DB()
	keys, _ := apikeys.List(db)
	appsByPage := map[uint][]string{}
	for _, k := range keys {
		if k.Active && k.StatusPageID != 0 {
			appsByPage[k.StatusPageID] = append(appsByPage[k.StatusPageID], k.Name)
		}
	}

	rows := []statusPageRow{}
	for _, p := range status.Pages(db) {
		apps := appsByPage[p.ID]
		if apps == nil {
			apps = []string{}
		}
		rows = append(rows, statusPageRow{Page: p, Apps: apps})
	}
	// The settings answer only on LogNorth's own domain, so the request's host is the DNS target.
	return RenderPage(ctx, "SettingsStatus", Props{"pages": rows, "host": status.HostOnly(ctx.Hostname())})
}

// CreateStatusPageAction handles POST /settings/status
func CreateStatusPageAction(ctx *cartridge.Context) error {
	db := ctx.DB()
	page, err := status.CreatePage(db, ctx.FormValue("name"), ctx.FormValue("domain"), ownHosts(db))
	if err != nil {
		setFlash(ctx, "error", pageError(err))
		return ctx.Redirect("/settings/status", http.StatusFound)
	}
	if page.Domain != "" {
		setFlash(ctx, "success", "Created "+page.Name+". Add its DNS record below, then pick its apps on the Apps page.")
	} else {
		setFlash(ctx, "success", "Created "+page.Name+". Now pick its apps on the Apps page.")
	}
	return ctx.Redirect("/settings/status", http.StatusFound)
}

// UpdateStatusPageAction handles POST /settings/status/:id
func UpdateStatusPageAction(ctx *cartridge.Context) error {
	db := ctx.DB()
	id, err := ctx.ParamsInt("id")
	if err == nil && id > 0 {
		err = status.UpdatePage(db, uint(id), ctx.FormValue("name"), ctx.FormValue("domain"), ownHosts(db))
	} else {
		err = status.ErrPageNotFound
	}
	if err != nil {
		setFlash(ctx, "error", pageError(err))
	} else {
		setFlash(ctx, "success", "Saved")
	}
	return ctx.Redirect("/settings/status", http.StatusFound)
}

// DeleteStatusPageAction handles POST /settings/status/:id/delete
func DeleteStatusPageAction(ctx *cartridge.Context) error {
	id, err := ctx.ParamsInt("id")
	if err != nil || id <= 0 || status.DeletePage(ctx.DB(), uint(id)) != nil {
		setFlash(ctx, "error", "Couldn't delete the status page")
		return ctx.Redirect("/settings/status", http.StatusFound)
	}
	setFlash(ctx, "success", "Status page deleted. Its apps are on no page now.")
	return ctx.Redirect("/settings/status", http.StatusFound)
}

// SetAppStatusPageAction handles POST /apps/api-keys/:id/status-page
func SetAppStatusPageAction(ctx *cartridge.Context) error {
	id, err := ctx.ParamsInt("id")
	if err != nil || id <= 0 {
		setFlash(ctx, "error", "Invalid app")
		return ctx.Redirect("/apps", http.StatusFound)
	}
	pageID, _ := strconv.ParseUint(ctx.FormValue("page_id"), 10, 64)
	if err := status.SetAppPage(ctx.DB(), uint(id), uint(pageID)); err != nil {
		setFlash(ctx, "error", pageError(err))
		return ctx.Redirect("/apps", http.StatusFound)
	}
	if pageID == 0 {
		setFlash(ctx, "success", "Off the status page")
	} else {
		setFlash(ctx, "success", "On the status page")
	}
	return ctx.Redirect("/apps", http.StatusFound)
}

// pageError is the message for a failed page change: the rule it broke, or
// a plain failure that leaks no database detail.
func pageError(err error) string {
	for _, known := range []error{status.ErrPageNameRequired, status.ErrPageNameTooLong, status.ErrBadDomain, status.ErrDomainTaken, status.ErrOwnDomain, status.ErrPageNotFound} {
		if errors.Is(err, known) {
			return err.Error()
		}
	}
	return "Couldn't save the status page"
}
