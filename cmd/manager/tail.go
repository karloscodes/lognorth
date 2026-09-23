package main

// lognorth tail: your production log in the terminal, like tail -f.

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

// event is one log line as search_logs returns it.
type event struct {
	ID           uint      `json:"id"`
	APIKeyID     uint      `json:"api_key_id"`
	Timestamp    time.Time `json:"timestamp"`
	Message      string    `json:"message"`
	TraceID      string    `json:"trace_id"`
	DurationMS   int       `json:"duration_ms"`
	IsError      bool      `json:"is_error"`
	Context      string    `json:"context"`
	ErrorClass   string    `json:"error_class"`
	ErrorMessage string    `json:"error_message"`
	ErrorFile    string    `json:"error_file"`
	ErrorLine    int       `json:"error_line"`
}

// request reads the HTTP fields SDKs put in the event's context.
func (e event) request() (method, path string, status int) {
	var r struct {
		Method string `json:"method"`
		Path   string `json:"path"`
		Status int    `json:"status"`
	}
	json.Unmarshal([]byte(e.Context), &r)
	return r.Method, r.Path, r.Status
}

// tailer follows the log. It pages by event id, so it prints each event once,
// in the order the server stored them.
type tailer struct {
	c         *client
	filter    map[string]any  // search, errors_only, app_id
	names     map[uint]string // app names, shown when several apps share the stream
	colors    palette
	lastID    uint
	lastTrace string // the trace of the last request printed; its other events indent
}

func tail(args []string) error {
	fs := flag.NewFlagSet("tail", flag.ContinueOnError)
	appName := fs.String("app", "", "only this app, by name or id")
	errorsOnly := fs.Bool("errors", false, "only failed requests and errors")
	path := fs.String("path", "", "only events that mention this path, e.g. /checkout")
	backlog := fs.Int("n", 20, "how many recent events to show first")
	fs.Usage = func() {
		fmt.Fprintln(fs.Output(), "Usage: lognorth tail [flags] [search]")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}

	r, err := loadRemote()
	if err != nil {
		return err
	}
	c := newClient(r)
	apps, err := c.apps()
	if err != nil {
		return err
	}

	t := &tailer{c: c, filter: map[string]any{}, colors: colorsFor(os.Stdout)}
	if search := strings.TrimSpace(*path + " " + strings.Join(fs.Args(), " ")); search != "" {
		t.filter["search"] = search
	}
	if *errorsOnly {
		t.filter["errors_only"] = true
	}
	switch {
	case *appName != "":
		a, err := findApp(apps, *appName)
		if err != nil {
			return err
		}
		t.filter["app_id"] = a.ID
	case len(apps) > 1:
		t.names = map[uint]string{}
		for _, a := range apps {
			t.names[a.ID] = a.Name
		}
	}

	fmt.Fprintln(os.Stdout, t.colors.dim(fmt.Sprintf("tailing %s (from %s) · ctrl+c stops", r.URL, r.From)))
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return t.run(ctx, os.Stdout, *backlog, 2*time.Second)
}

// run prints the recent events, then new ones as they arrive, until ctx ends.
func (t *tailer) run(ctx context.Context, w io.Writer, backlog int, every time.Duration) error {
	if err := t.backlog(w, backlog); err != nil {
		return err
	}
	if t.lastID == 0 && backlog > 0 {
		fmt.Fprintln(w, t.colors.dim("no events in the last hour · waiting for new ones"))
	}

	noted := ""
	note := func(s string) {
		if s != noted {
			fmt.Fprintln(w, t.colors.dim(s))
			noted = s
		}
	}
	wait := every
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(wait):
		}

		wait = every
		err := t.poll(w)
		var stop fatal
		switch {
		case err == nil:
			noted = ""
		case errors.As(err, &stop):
			return err
		case errors.Is(err, errRateLimited):
			note("the server is rate limiting agent calls · slowing down")
			wait = 10 * time.Second
		default:
			note(err.Error() + " · retrying")
			wait = 5 * time.Second
		}
	}
}

// backlog prints the newest n events of the last hour, oldest first. It
// reads at least one, so even with n = 0 the follow starts after the newest.
func (t *tailer) backlog(w io.Writer, n int) error {
	var page struct {
		Events []event `json:"events"`
	}
	if err := t.c.call("search_logs", t.args("limit", min(max(n, 1), 200)), &page); err != nil {
		return err
	}
	for i := len(page.Events) - 1; i >= 0; i-- {
		if i < n {
			t.print(w, page.Events[i])
		} else {
			t.lastID = max(t.lastID, page.Events[i].ID)
		}
	}
	return nil
}

// poll prints what arrived since the last event printed. A full page means
// more are waiting, so it reads again at once.
func (t *tailer) poll(w io.Writer) error {
	const pageSize = 200
	for {
		args := t.args("limit", pageSize)
		if t.lastID > 0 {
			args["after_id"] = t.lastID
		}
		var page struct {
			Events []event `json:"events"`
		}
		if err := t.c.call("search_logs", args, &page); err != nil {
			return err
		}

		if t.lastID == 0 {
			// Nothing printed yet: this is the last hour, newest first.
			for i := len(page.Events) - 1; i >= 0; i-- {
				t.print(w, page.Events[i])
			}
			return nil
		}
		for _, e := range page.Events {
			if e.ID <= t.lastID {
				return fatal{errors.New("this server cannot follow the log yet. Run lognorth update on the server")}
			}
			t.print(w, e)
		}
		if len(page.Events) < pageSize {
			return nil
		}
	}
}

func (t *tailer) args(key string, value any) map[string]any {
	args := map[string]any{key: value}
	for k, v := range t.filter {
		args[k] = v
	}
	return args
}

func (t *tailer) print(w io.Writer, e event) {
	if e.ID > t.lastID {
		t.lastID = e.ID
	}
	fmt.Fprintln(w, t.format(e))
}

// format writes one event as one line: the time, the app when there are
// several, then the request, the error, or the message.
//
//	05:00:58.012  POST   /checkout  500 · 1204ms  Stripe::CardError: Your card was declined.
//	05:00:58.013    Charging card  847ms
func (t *tailer) format(e event) string {
	p := t.colors
	parts := []string{p.dim(e.Timestamp.Local().Format("15:04:05.000"))}
	if t.names != nil {
		parts = append(parts, p.dim(fit(t.names[e.APIKeyID], 14)))
	}

	indent := ""
	if e.TraceID != "" && e.TraceID == t.lastTrace {
		indent = "  "
	}
	failure := e.ErrorMessage
	if e.ErrorClass != "" {
		failure = strings.TrimSuffix(e.ErrorClass+": "+e.ErrorMessage, ": ")
	}
	where := ""
	if e.ErrorFile != "" {
		where = p.dim(fmt.Sprintf("%s:%d", e.ErrorFile, e.ErrorLine))
	}

	method, path, status := e.request()
	switch {
	case method != "" && path != "":
		t.lastTrace = e.TraceID
		req := fmt.Sprintf("%-6s %s", method, path)
		meta := fmt.Sprintf("%d · %dms", status, e.DurationMS)
		if e.IsError || status >= 500 {
			parts = append(parts, p.red(req), p.red(meta))
			if failure != "" {
				parts = append(parts, p.red(failure))
			}
			if where != "" {
				parts = append(parts, where)
			}
		} else {
			parts = append(parts, req, p.dim(meta))
		}
	case e.IsError:
		if failure == "" {
			failure = e.Message
		}
		parts = append(parts, indent+p.red(p.bold(failure)))
		if where != "" {
			parts = append(parts, where)
		}
	default:
		parts = append(parts, indent+e.Message)
		if e.DurationMS > 0 {
			parts = append(parts, p.dim(fmt.Sprintf("%dms", e.DurationMS)))
		}
	}
	return strings.Join(parts, "  ")
}

// fatal marks an error that retrying cannot fix.
type fatal struct{ error }

// cut shortens s to at most n characters, ending in … when it had to.
func cut(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	if n <= 1 {
		return string(r[:max(n, 0)])
	}
	return string(r[:n-1]) + "…"
}

// fit cuts or pads s to exactly n characters, for columns.
func fit(s string, n int) string {
	s = cut(s, n)
	return s + strings.Repeat(" ", max(0, n-len([]rune(s))))
}
