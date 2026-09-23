package main

// The laptop side of the lognorth command: connect, tail, and top read a
// LogNorth server over its MCP endpoint with a read-only agent key. They add
// no API of their own, so an agent and a person read the same tools.

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/term"
)

// remote is where the laptop commands read from.
type remote struct {
	URL  string `json:"url"`
	Key  string `json:"key"`
	From string `json:"-"` // where it came from: the environment or the file
}

// remotePath is ~/.config/lognorth/remote.json, or under $XDG_CONFIG_HOME.
func remotePath() (string, error) {
	dir := os.Getenv("XDG_CONFIG_HOME")
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		dir = filepath.Join(home, ".config")
	}
	return filepath.Join(dir, "lognorth", "remote.json"), nil
}

// loadRemote reads LOGNORTH_URL and LOGNORTH_AGENT_KEY first, the same
// variables the agent plugin uses, then the file lognorth connect saved.
func loadRemote() (remote, error) {
	if fromEnv() {
		return remote{URL: normalizeURL(os.Getenv("LOGNORTH_URL")), Key: os.Getenv("LOGNORTH_AGENT_KEY"), From: "LOGNORTH_URL"}, nil
	}

	path, err := remotePath()
	if err != nil {
		return remote{}, err
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return remote{}, errors.New("not connected yet. Run: lognorth connect https://logs.yoursite.com lgn-agent-...")
	}
	if err != nil {
		return remote{}, err
	}
	var r remote
	if err := json.Unmarshal(data, &r); err != nil {
		return remote{}, fmt.Errorf("could not read %s: %w", path, err)
	}
	r.From = path
	return r, nil
}

func fromEnv() bool {
	return os.Getenv("LOGNORTH_URL") != "" && os.Getenv("LOGNORTH_AGENT_KEY") != ""
}

func saveRemote(r remote) (string, error) {
	path, err := remotePath()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return "", err
	}
	data, _ := json.MarshalIndent(r, "", "  ")
	// The key reads your production logs, so only you may read the file.
	return path, os.WriteFile(path, append(data, '\n'), 0o600)
}

// normalizeURL accepts what people paste: a trailing slash or the /mcp path.
func normalizeURL(url string) string {
	url = strings.TrimRight(strings.TrimSpace(url), "/")
	url = strings.TrimSuffix(url, "/mcp")
	if !strings.Contains(url, "://") {
		scheme := "https://"
		if strings.HasPrefix(url, "localhost") || strings.HasPrefix(url, "127.0.0.1") {
			scheme = "http://" // a LogNorth on this machine has no TLS
		}
		url = scheme + url
	}
	return url
}

// connect checks the URL and key against the server, then saves them. It
// asks for whatever the arguments leave out, and hides the key as you paste it.
func connect(args []string) error {
	if len(args) > 2 {
		return errors.New("usage: lognorth connect [url] [agent key]")
	}
	for len(args) < 2 {
		answer, err := ask(len(args))
		if err != nil {
			return err
		}
		args = append(args, answer)
	}
	r := remote{URL: normalizeURL(args[0]), Key: strings.TrimSpace(args[1])}
	if !strings.HasPrefix(r.Key, "lgn-agent-") {
		if strings.HasPrefix(r.Key, "lgn-") {
			return errors.New("that is an app key, which sends events. Use the agent key from Settings > Developer: it starts with lgn-agent-")
		}
		return errors.New("an agent key starts with lgn-agent-. Find it in LogNorth under Settings > Developer")
	}

	apps, err := newClient(r).apps()
	if err != nil {
		return err
	}
	path, err := saveRemote(r)
	if err != nil {
		return fmt.Errorf("could not save the connection: %w", err)
	}

	names := make([]string, len(apps))
	for i, a := range apps {
		names[i] = a.Name
	}
	fmt.Printf("Connected to %s. %d app(s): %s\n", r.URL, len(apps), strings.Join(names, ", "))
	fmt.Printf("Saved to %s. Now try: lognorth tail, or lognorth top\n", path)
	if fromEnv() && normalizeURL(os.Getenv("LOGNORTH_URL")) != r.URL {
		fmt.Printf("Note: LOGNORTH_URL in your shell points at %s, and it wins over this file. Unset it to read %s.\n",
			normalizeURL(os.Getenv("LOGNORTH_URL")), r.URL)
	}
	return nil
}

// ask prompts for the URL (step 0) or the agent key (step 1).
func ask(step int) (string, error) {
	in := int(os.Stdin.Fd())
	if !term.IsTerminal(in) {
		return "", errors.New("usage: lognorth connect <url> <agent key>. The agent key is in LogNorth under Settings > Developer")
	}
	if step == 0 {
		fmt.Print("LogNorth URL (https://logs.yoursite.com): ")
		line, err := bufio.NewReader(os.Stdin).ReadString('\n')
		return strings.TrimSpace(line), err
	}
	fmt.Print("Agent key, from Settings > Developer (hidden): ")
	key, err := term.ReadPassword(in)
	fmt.Println()
	return strings.TrimSpace(string(key)), err
}

// errRateLimited means the server allows no more agent calls this minute.
var errRateLimited = errors.New("rate limited")

type client struct {
	remote
	http *http.Client
}

func newClient(r remote) *client {
	return &client{remote: r, http: &http.Client{Timeout: 10 * time.Second}}
}

// call runs one MCP tool and decodes its JSON answer into out.
func (c *client) call(tool string, args map[string]any, out any) error {
	body, _ := json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "tools/call",
		"params":  map[string]any{"name": tool, "arguments": args},
	})
	req, err := http.NewRequest(http.MethodPost, c.URL+"/mcp", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.Key)
	req.Header.Set("Content-Type", "application/json")

	res, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("could not reach %s: %w", c.URL, err)
	}
	defer res.Body.Close()

	switch {
	case res.StatusCode == http.StatusUnauthorized:
		return fatal{errors.New("the server rejected the agent key. Run lognorth connect again with a key from Settings > Developer")}
	case res.StatusCode == http.StatusTooManyRequests:
		return errRateLimited
	case res.StatusCode != http.StatusOK:
		return fmt.Errorf("%s answered %s", c.URL, res.Status)
	}

	var rpc struct {
		Result struct {
			IsError bool `json:"isError"`
			Content []struct {
				Text string `json:"text"`
			} `json:"content"`
		} `json:"result"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.NewDecoder(res.Body).Decode(&rpc); err != nil {
		return fmt.Errorf("%s did not answer like LogNorth: %w", c.URL, err)
	}
	if rpc.Error != nil {
		return errors.New(rpc.Error.Message)
	}
	if len(rpc.Result.Content) == 0 {
		return fmt.Errorf("%s returned nothing", tool)
	}
	text := rpc.Result.Content[0].Text
	if rpc.Result.IsError {
		if strings.HasPrefix(text, "unknown tool") {
			return fatal{errors.New("this needs a newer LogNorth. Run lognorth update on the server")}
		}
		return errors.New(text)
	}
	return json.Unmarshal([]byte(text), out)
}

type app struct {
	ID   uint   `json:"id"`
	Name string `json:"name"`
}

func (c *client) apps() ([]app, error) {
	var out struct {
		Apps []app `json:"apps"`
	}
	err := c.call("list_apps", map[string]any{}, &out)
	return out.Apps, err
}

// findApp matches an app by name, case-insensitive, or by id.
func findApp(apps []app, want string) (app, error) {
	for _, a := range apps {
		if strings.EqualFold(a.Name, want) || fmt.Sprint(a.ID) == want {
			return a, nil
		}
	}
	names := make([]string, len(apps))
	for i, a := range apps {
		names[i] = a.Name
	}
	return app{}, fmt.Errorf("no app named %q. Apps: %s", want, strings.Join(names, ", "))
}

// Colors, one per role, like the web app: dim for time and hints, red for
// failures, lime for good news. Off when the output is not a terminal or
// NO_COLOR is set.
type palette struct{ on bool }

func colorsFor(f *os.File) palette {
	return palette{on: os.Getenv("NO_COLOR") == "" && term.IsTerminal(int(f.Fd()))}
}

func (p palette) paint(code, s string) string {
	if !p.on || s == "" {
		return s
	}
	return "\x1b[" + code + "m" + s + "\x1b[0m"
}

func (p palette) dim(s string) string  { return p.paint("90", s) }
func (p palette) red(s string) string  { return p.paint("31", s) }
func (p palette) lime(s string) string { return p.paint("92", s) }
func (p palette) bold(s string) string { return p.paint("1", s) }
