package events

import (
	"encoding/json"
	"regexp"
	"slices"
	"strings"
)

// The processor redacts secrets before it stores an event, so LogNorth never
// keeps a key that an app logged by mistake. A value under a secret name
// (password, authorization, ...) is redacted silently. A value in a known
// secret format is redacted and alerts once per app and kind: logging it is a
// bug in the app, and the key needs rotating.

// secretFormats have a fixed prefix, so a match is almost never a false
// alarm. Add one when a real secret gets through. A format's regexp runs only
// on text that holds one of its prefixes: most events hold none, and a stack
// trace is long.
var secretFormats = []struct {
	kind     string
	prefixes []string
	re       *regexp.Regexp
}{
	{"Stripe secret key", []string{"k_live_"}, regexp.MustCompile(`\b[rs]k_live_[0-9A-Za-z]{20,}`)},
	{"AWS access key", []string{"AKIA", "ASIA"}, regexp.MustCompile(`\b(?:AKIA|ASIA)[0-9A-Z]{16}\b`)},
	{"GitHub token", []string{"ghp_", "gho_", "ghu_", "ghs_", "ghr_", "github_pat_"}, regexp.MustCompile(`\b(?:gh[pousr]_[0-9A-Za-z]{36,}|github_pat_[0-9A-Za-z_]{22,})`)},
	{"Slack token", []string{"xox"}, regexp.MustCompile(`\bxox[abprs]-[0-9A-Za-z-]{10,}`)},
	{"Anthropic API key", []string{"sk-ant-"}, regexp.MustCompile(`\bsk-ant-[0-9A-Za-z_-]{20,}`)},
	{"OpenAI API key", []string{"sk-proj-", "sk-svcacct-", "sk-admin-"}, regexp.MustCompile(`\bsk-(?:proj|svcacct|admin)-[0-9A-Za-z_-]{20,}`)},
	// A key cut short by the SDK has no END line: redact to the end.
	{"private key", []string{"PRIVATE KEY-----"}, regexp.MustCompile(`-----BEGIN[A-Z ]*PRIVATE KEY-----[\s\S]*?(?:-----END[A-Z ]*PRIVATE KEY-----|$)`)},
}

const redacted = "[redacted]"

// RedactSecrets replaces the secrets in an ingested event's message and
// context, and returns the kinds of secret it found by their format.
func RedactSecrets(ie *IngestedEvent) []string {
	r := &redactor{}
	ie.Message = r.text(ie.Message)

	dec := json.NewDecoder(strings.NewReader(ie.Context))
	dec.UseNumber() // a rewritten context keeps its numbers exactly
	var ctx any
	if err := dec.Decode(&ctx); err != nil {
		ie.Context = r.text(ie.Context) // not JSON: the formats still go
		return r.kinds
	}
	r.changed = false
	ctx = r.value("", ctx)
	if r.changed {
		clean, err := json.Marshal(ctx)
		if err != nil {
			clean = []byte("{}") // never store the secret
		}
		ie.Context = string(clean)
	}
	return r.kinds
}

type redactor struct {
	kinds   []string
	changed bool
}

// value returns v with its secrets redacted. Under a secret name, a string,
// an object, or a list is redacted whole; a number or a boolean stays.
func (r *redactor) value(name string, v any) any {
	hide := secretName(name)
	switch v := v.(type) {
	case string:
		clean := r.text(v)
		if hide && v != "" {
			r.changed = true
			return redacted
		}
		return clean
	case map[string]any:
		for k, x := range v {
			v[k] = r.value(k, x)
		}
		if hide && len(v) > 0 {
			r.changed = true
			return redacted
		}
	case []any:
		for i, x := range v {
			v[i] = r.value("", x)
		}
		if hide && len(v) > 0 {
			r.changed = true
			return redacted
		}
	}
	return v
}

// text replaces each secret format in s with a placeholder that names it.
func (r *redactor) text(s string) string {
	for _, f := range secretFormats {
		if !slices.ContainsFunc(f.prefixes, func(p string) bool { return strings.Contains(s, p) }) {
			continue
		}
		clean := f.re.ReplaceAllLiteralString(s, "[redacted: "+f.kind+"]")
		if clean == s {
			continue
		}
		s = clean
		r.changed = true
		if !slices.Contains(r.kinds, f.kind) {
			r.kinds = append(r.kinds, f.kind)
		}
	}
	return s
}

// secretName reports whether a context key names a secret, in any case and
// with or without separators: password, secret, authorization, cookie,
// api_key, private_key, or a name that ends in token.
func secretName(key string) bool {
	k := strings.ReplaceAll(strings.ReplaceAll(strings.ToLower(key), "_", ""), "-", "")
	for _, part := range []string{"password", "passwd", "secret", "authorization", "cookie", "apikey", "privatekey"} {
		if strings.Contains(k, part) {
			return true
		}
	}
	return strings.HasSuffix(k, "token")
}
