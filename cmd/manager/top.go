package main

// lognorth top: one app at a glance, like htop. Endpoints most broken first,
// the alerts firing now, and the uptime ping over 24 hours, refreshed every
// few seconds. Keys move the selection and open an endpoint's recent errors.

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"
	"unicode/utf8"

	"golang.org/x/term"
)

type endpointRow struct {
	Path        string  `json:"path"`
	Total       int64   `json:"total"`
	Errors      int64   `json:"errors"`
	ErrorRate   float64 `json:"error_rate"`
	AvgDuration int     `json:"avg_duration"`
}

type alertRow struct {
	Path    string    `json:"path"`
	Kind    string    `json:"kind"` // "error_rate" or "silence"
	Since   time.Time `json:"since"`
	Summary string    `json:"summary"`
}

type downRow struct {
	URL   string    `json:"url"`
	Since time.Time `json:"since"`
}

type uptimeReport struct {
	Status    string     `json:"status"` // up, failing, down, pending, off, none
	LatencyMS int        `json:"latency_ms"`
	DownSince *time.Time `json:"down_since"`
	Uptime24h *float64   `json:"uptime_24h"`
	Bars      []struct {
		Checks int `json:"checks"`
		Fails  int `json:"fails"`
	} `json:"bars"`
}

// Refresh every 5 seconds and read the uptime every 30. That is about 30
// calls a minute, half the server's limit for agent calls.
const (
	topEvery    = 5 * time.Second
	uptimeEvery = 30 * time.Second
)

var windows = []string{"1h", "24h", "7d"}

// topState is what the keys change.
type topState struct {
	host   string // the server, so you know which one you are reading
	apps   []app
	app    int // index into apps
	window int // index into windows
	sel    int // the selected endpoint
	open   bool
	path   string // the endpoint whose errors are open
}

// snapshot is what one refresh read, tagged with the app and window it is for.
type snapshot struct {
	appID     uint
	window    string
	endpoints []endpointRow
	alerts    []alertRow
	down      []downRow
	uptime    *uptimeReport
	errors    []event // the open endpoint's recent errors
	at        time.Time
	err       error
}

func top(args []string) error {
	fs := flag.NewFlagSet("top", flag.ContinueOnError)
	appName := fs.String("app", "", "start on this app, by name or id")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	in, out := int(os.Stdin.Fd()), int(os.Stdout.Fd())
	if !term.IsTerminal(in) || !term.IsTerminal(out) {
		return errors.New("top needs a terminal. To pipe the log, use lognorth tail")
	}

	r, err := loadRemote()
	if err != nil {
		return err
	}
	c := newClient(r)
	st := topState{host: strings.TrimPrefix(strings.TrimPrefix(r.URL, "https://"), "http://")}
	if st.apps, err = c.apps(); err != nil {
		return err
	}
	if len(st.apps) == 0 {
		return errors.New("no apps yet. Add one in LogNorth first")
	}
	if *appName != "" {
		a, err := findApp(st.apps, *appName)
		if err != nil {
			return err
		}
		for i := range st.apps {
			if st.apps[i].ID == a.ID {
				st.app = i
			}
		}
	}

	saved, err := term.MakeRaw(in)
	if err != nil {
		return err
	}
	defer term.Restore(in, saved)
	fmt.Print("\x1b[?1049h\x1b[?25l") // the alternate screen, no cursor
	defer fmt.Print("\x1b[?25h\x1b[?1049l")

	return runTop(c, st, os.Stdin, os.Stdout, colorsFor(os.Stdout))
}

func runTop(c *client, st topState, keysIn io.Reader, w *os.File, p palette) error {
	keys := make(chan string)
	go readKeys(keysIn, keys)

	results := make(chan snapshot, 1)
	var snap snapshot
	loading := false
	var lastUptime, waitUntil time.Time
	refresh := func(withUptime bool) {
		if loading {
			return
		}
		if withUptime || time.Since(lastUptime) >= uptimeEvery {
			withUptime, lastUptime = true, time.Now()
		}
		loading = true
		go func(st topState, prev *uptimeReport) {
			results <- read(c, st, withUptime, prev)
		}(st, snap.uptime)
	}
	draw := func() {
		width, height, err := term.GetSize(int(w.Fd()))
		if err != nil || width == 0 || height == 0 {
			width, height = 100, 30
		}
		lines := renderTop(st, snap, loading, time.Now(), width, height, p)
		fmt.Fprint(w, "\x1b[H"+strings.Join(lines, "\x1b[K\r\n")+"\x1b[K\x1b[J")
	}

	refresh(true)
	draw()
	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	for {
		select {
		case s := <-results:
			loading = false
			if s.appID != st.apps[st.app].ID || s.window != windows[st.window] {
				refresh(s.appID != st.apps[st.app].ID) // the keys moved on while it read
				continue
			}
			if errors.Is(s.err, errRateLimited) {
				waitUntil = time.Now().Add(15 * time.Second)
			}
			var stop fatal
			if errors.As(s.err, &stop) {
				return s.err
			}
			if s.err != nil && snap.appID == s.appID {
				snap.err, snap.at = s.err, s.at // keep the last good numbers on screen
			} else {
				snap = s
			}
			draw()

		case <-tick.C:
			if !loading && time.Since(snap.at) >= topEvery && time.Now().After(waitUntil) {
				refresh(false)
			}
			draw()

		case k, ok := <-keys:
			if !ok || k == "q" || k == "quit" {
				return nil
			}
			rows := len(snap.endpoints)
			switch k {
			case "j", "down":
				st.sel = min(st.sel+1, max(rows-1, 0))
			case "k", "up":
				st.sel = max(st.sel-1, 0)
			case "w":
				st.window = (st.window + 1) % len(windows)
				refresh(false)
			case "a":
				if len(st.apps) > 1 {
					st.app = (st.app + 1) % len(st.apps)
					st.sel, st.open = 0, false
					snap = snapshot{}
					refresh(true)
				}
			case "r":
				refresh(true)
			case "enter":
				if st.sel < rows {
					st.open, st.path = true, snap.endpoints[st.sel].Path
					refresh(false)
				}
			case "esc":
				st.open = false
			}
			draw()
		}
	}
}

// read makes one refresh's calls: endpoints and alerts every time, the
// uptime when asked, and the open endpoint's errors.
func read(c *client, st topState, withUptime bool, prev *uptimeReport) snapshot {
	a := st.apps[st.app]
	s := snapshot{appID: a.ID, window: windows[st.window], uptime: prev, at: time.Now()}

	var eps struct {
		Endpoints []endpointRow `json:"endpoints"`
	}
	if s.err = c.call("list_endpoints", map[string]any{"app_id": a.ID, "since": s.window, "limit": 200}, &eps); s.err != nil {
		return s
	}
	s.endpoints = eps.Endpoints

	var al struct {
		Alerts []alertRow `json:"alerts"`
		Down   []downRow  `json:"down"`
	}
	if s.err = c.call("list_alerts", map[string]any{"app_id": a.ID}, &al); s.err != nil {
		return s
	}
	s.alerts, s.down = al.Alerts, al.Down

	if withUptime {
		var u uptimeReport
		if s.err = c.call("uptime_timeline", map[string]any{"app_id": a.ID}, &u); s.err != nil {
			return s
		}
		s.uptime = &u
	}

	if st.open {
		var page struct {
			Events []event `json:"events"`
		}
		args := map[string]any{"app_id": a.ID, "search": st.path, "errors_only": true, "since": s.window, "limit": 8}
		if s.err = c.call("search_logs", args, &page); s.err != nil {
			return s
		}
		s.errors = page.Events
	}
	return s
}

// readKeys turns raw terminal input into key names.
func readKeys(r io.Reader, out chan<- string) {
	buf := make([]byte, 8)
	for {
		n, err := r.Read(buf)
		if err != nil || n == 0 {
			close(out)
			return
		}
		switch {
		case n >= 3 && buf[0] == 27 && buf[1] == '[' && buf[2] == 'A':
			out <- "up"
		case n >= 3 && buf[0] == 27 && buf[1] == '[' && buf[2] == 'B':
			out <- "down"
		case buf[0] == 27:
			out <- "esc"
		case buf[0] == 3: // ctrl+c, since raw mode turns off the signal
			out <- "quit"
		case buf[0] == '\r' || buf[0] == '\n':
			out <- "enter"
		default:
			out <- string(buf[0])
		}
	}
}

// line builds one screen line from painted pieces and tracks its visible width.
type line struct {
	b strings.Builder
	n int
}

func (l *line) add(s string, style func(string) string) *line {
	l.n += utf8.RuneCountInString(s)
	if style != nil {
		s = style(s)
	}
	l.b.WriteString(s)
	return l
}

// raw adds text that is already painted and takes n columns.
func (l *line) raw(s string, n int) *line {
	l.n += n
	l.b.WriteString(s)
	return l
}

// pad fills with spaces up to column `to`.
func (l *line) pad(to int) *line {
	if to > l.n {
		l.add(strings.Repeat(" ", to-l.n), nil)
	}
	return l
}

// right puts s at the end of a line `width` wide, when it fits.
func (l *line) right(s string, width int, style func(string) string) *line {
	if n := utf8.RuneCountInString(s); l.n+n < width {
		l.pad(width-n).add(s, style)
	}
	return l
}

func (l *line) String() string { return l.b.String() }

// renderTop draws the whole screen as lines, at most `height` of them, none
// wider than `width`.
func renderTop(st topState, s snapshot, loading bool, now time.Time, width, height int, p palette) []string {
	if width < 60 || height < 12 {
		return []string{"Make the window at least 60×12 for lognorth top."}
	}
	a := st.apps[st.app]
	var out []string
	push := func(l *line) { out = append(out, l.String()) }
	blank := func() { out = append(out, "") }

	// [lognorth] top · checkout-prod (1/3)                 ■ up 84ms · 05:02:11
	title := &line{}
	title.add("[", p.lime).add("lognorth", p.bold).add("]", p.lime).add(" top · ", p.dim).add(cut(a.Name, 24), p.bold)
	if len(st.apps) > 1 {
		title.add(fmt.Sprintf(" (%d/%d)", st.app+1, len(st.apps)), p.dim)
	}
	if st.host != "" {
		title.add(" · "+cut(st.host, 32), p.dim)
	}
	label, bad := uptimeText(s.uptime, now)
	clock := " · " + now.Format("15:04:05")
	title.pad(width - utf8.RuneCountInString(label+clock))
	if bad {
		title.add(label, p.red)
	} else {
		title.add(label, nil)
	}
	title.add(clock, p.dim)
	push(title)
	blank()

	// uptime 24h  ████████·····██████████  99.98%
	up := &line{}
	up.add("uptime 24h  ", p.dim)
	switch {
	case s.uptime == nil:
		up.add("reading…", p.dim)
	case s.uptime.Status == "none":
		up.add("no URL yet. Add one on the Apps page and LogNorth pings it every minute.", p.dim)
	case s.uptime.Status == "off":
		up.add("uptime checks are off for this app", p.dim)
	default:
		pct := ""
		if s.uptime.Uptime24h != nil {
			pct = "  " + percentText(*s.uptime.Uptime24h)
		}
		up.raw(strip(s.uptime, width-12-len(pct), p))
		up.add(pct, nil)
	}
	push(up)
	blank()

	// What is alerting now.
	flags := map[string]string{}
	for _, al := range s.alerts {
		flags[al.Path] = map[string]string{"silence": "SILENT"}[al.Kind]
		if flags[al.Path] == "" {
			flags[al.Path] = "SPIKE"
		}
	}
	alertLines := 0
	for _, d := range s.down {
		push((&line{}).add("DOWN   ", func(x string) string { return p.red(p.bold(x)) }).add(cut(d.URL+" since "+d.Since.Local().Format("15:04"), width-7), p.red))
		alertLines++
	}
	for _, al := range s.alerts {
		if alertLines == 3 {
			push((&line{}).add(fmt.Sprintf("+%d more alerts", len(s.alerts)+len(s.down)-3), p.dim))
			break
		}
		l := &line{}
		l.add(fit(flags[al.Path], 7), func(x string) string { return p.red(p.bold(x)) })
		l.add(fit(al.Path, 24)+"  ", nil)
		l.add(cut(al.Summary+" · since "+al.Since.Local().Format("15:04"), width-33), p.dim)
		push(l)
		alertLines++
	}
	if len(s.alerts)+len(s.down) == 0 && s.appID != 0 {
		push((&line{}).add("■", p.lime).add(" no alerts", p.dim))
	}
	blank()

	// The endpoint table fills what the errors panel and the footer leave.
	panel := 0
	if st.open {
		panel = 2 + max(1, len(s.errors))
	}
	rows := height - len(out) - 1 - panel - 2 // header, panel, blank, footer
	const nums = 9 + 8 + 7 + 8                // reqs, errors, err%, avg
	pathW := width - 2 - nums
	head := &line{}
	head.add("  "+fit("PATH", pathW)+fmt.Sprintf("%9s%8s%7s%8s", "REQS", "ERRORS", "ERR%", "AVG"), p.dim)
	push(head)

	if len(s.endpoints) == 0 {
		msg := "reading…"
		if s.appID != 0 {
			msg = "no requests in the last " + windows[st.window] + ". Press w for a longer window."
		}
		push((&line{}).add("  "+msg, p.dim))
		rows--
	}
	offset := max(0, st.sel-rows+1)
	for i := offset; i < len(s.endpoints) && i < offset+rows; i++ {
		e := s.endpoints[i]
		flag := flags[e.Path]
		pathText := e.Path
		if flag != "" {
			pathText = fit(e.Path, pathW-len(flag)-2) // leave room for the flag
		}
		l := &line{}
		marker, style := "  ", func(x string) string { return x }
		if i == st.sel {
			marker = "> "
			if p.on {
				style = func(x string) string { return "\x1b[7m" + x + "\x1b[27m" }
			}
		}
		l.add(marker, nil)
		if flag != "" {
			l.add(strings.TrimRight(pathText, " "), style).add(" "+flag, func(x string) string { return p.red(p.bold(x)) })
			l.add(strings.Repeat(" ", max(0, 2+pathW-l.n)), nil)
		} else {
			l.add(fit(pathText, pathW), style)
		}
		rate := "—"
		if e.Errors > 0 {
			rate = fmt.Sprintf("%.1f%%", e.ErrorRate)
		}
		l.add(fmt.Sprintf("%9s%8s", commas(e.Total), commas(e.Errors)), nil)
		switch {
		case flag == "SPIKE":
			l.add(fmt.Sprintf("%7s", rate), func(x string) string { return p.red(p.bold(x)) })
		case e.Errors == 0:
			l.add(fmt.Sprintf("%7s", rate), p.dim)
		default:
			l.add(fmt.Sprintf("%7s", rate), nil)
		}
		l.add(fmt.Sprintf("%8s", fmt.Sprintf("%dms", e.AvgDuration)), p.dim)
		push(l)
	}

	if st.open {
		blank()
		push((&line{}).add(cut("errors on "+st.path+" · last "+windows[st.window]+" · esc closes", width), p.dim))
		if len(s.errors) == 0 {
			push((&line{}).add("  none", p.dim))
		}
		for _, e := range s.errors {
			what := e.ErrorMessage
			if e.ErrorClass != "" {
				what = e.ErrorClass + ": " + what
			}
			if what == "" {
				what = e.Message
			}
			push((&line{}).add("  "+e.Timestamp.Local().Format("15:04:05")+"  ", p.dim).add(cut(what, width-12), p.red))
		}
	}

	// Pad to the footer, then the keys and the state of the last read.
	for len(out) < height-1 {
		blank()
	}
	out = out[:height-1]
	keys := "j/k move · enter errors · w window · r refresh · q quit"
	if len(st.apps) > 1 {
		keys = "j/k move · enter errors · w window · a app · r refresh · q quit"
	}
	foot := &line{}
	foot.add(cut(keys, width-28), p.dim)
	switch {
	case s.err != nil && errors.Is(s.err, errRateLimited):
		foot.right("rate limited · waiting", width, p.dim)
	case s.err != nil:
		foot.right(cut(s.err.Error(), 26), width, p.red)
	case loading:
		foot.right("reading…", width, p.dim)
	case !s.at.IsZero():
		foot.right(fmt.Sprintf("updated %ds ago", int(now.Sub(s.at).Seconds())), width, p.dim)
	}
	push(foot)
	return out
}

// strip draws the 24h uptime bars in `width` columns: red where a ping
// failed, grey where all were up, a dot where none ran.
func strip(u *uptimeReport, width int, p palette) (string, int) {
	cols := min(len(u.Bars), width)
	if cols <= 0 {
		return "", 0
	}
	var b strings.Builder
	for c := 0; c < cols; c++ {
		from, to := c*len(u.Bars)/cols, (c+1)*len(u.Bars)/cols
		checks, fails := 0, 0
		for _, bar := range u.Bars[from:to] {
			checks += bar.Checks
			fails += bar.Fails
		}
		switch {
		case fails > 0:
			b.WriteString(p.red("█"))
		case checks > 0:
			b.WriteString(p.dim("█"))
		default:
			b.WriteString(p.dim("·"))
		}
	}
	return b.String(), cols
}

// uptimeText is the app's ping at a glance: "■ up 84ms" or "■ down 12m".
func uptimeText(u *uptimeReport, now time.Time) (string, bool) {
	if u == nil {
		return "", false
	}
	switch u.Status {
	case "up":
		return fmt.Sprintf("■ up %dms", u.LatencyMS), false
	case "failing":
		return "■ failing", true
	case "down":
		if u.DownSince != nil {
			return "■ down " + shortSince(*u.DownSince, now), true
		}
		return "■ down", true
	case "pending":
		return "□ first ping soon", false
	}
	return "", false
}

func shortSince(t, now time.Time) string {
	d := now.Sub(t)
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	}
	return fmt.Sprintf("%dd", int(d.Hours()/24))
}

func percentText(pct float64) string {
	if pct >= 99.995 {
		return "100%"
	}
	return fmt.Sprintf("%.2f%%", pct)
}

// commas writes 1204 as 1,204.
func commas(n int64) string {
	s := fmt.Sprint(n)
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + "," + s[i:]
	}
	return s
}
