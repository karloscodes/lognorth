package http

import (
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"strings"

	"github.com/karloscodes/cartridge"

	"lognorth/internal/config"
)

// contentSecurityPolicy lets the browser load only what the app ships: its
// own scripts, styles, fonts, and images. Inline styles stay allowed because
// components set them. Nothing inline runs as script.
const contentSecurityPolicy = "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; " +
	"img-src 'self' data:; font-src 'self'; connect-src 'self'; frame-ancestors 'self'; " +
	"base-uri 'self'; form-action 'self'; object-src 'none'"

// SecurityHeaders adds the two headers cartridge leaves to the app. In
// development the page loads from the Vite dev server, so neither applies.
func SecurityHeaders(ctx *cartridge.Context) error {
	if !config.GetConfig().IsDevelopment() {
		ctx.Set("Content-Security-Policy", contentSecurityPolicy)
		// Only over HTTPS: the header tells the browser to refuse HTTP for a year.
		if ctx.Protocol() == "https" {
			ctx.Set("Strict-Transport-Security", "max-age=31536000")
		}
	}
	return ctx.Next()
}

// RefererPath is where to send the browser back to after a form post: the
// path of the page that sent it. A Referer that is missing or points at
// another site gives the fallback, so a redirect never leaves the app.
func RefererPath(ctx *cartridge.Context, fallback string) string {
	header := ctx.Get("Referer")
	referer, err := url.Parse(header)
	if header == "" || err != nil || (referer.Host != "" && referer.Host != ctx.Hostname()) {
		return fallback
	}
	// "//host" is another site to a browser. RequestURI escapes a backslash,
	// so "/\host" comes out as a plain path.
	path := referer.RequestURI()
	if !strings.HasPrefix(path, "/") || strings.HasPrefix(path, "//") {
		return fallback
	}
	return path
}

// ErrorHandler answers a request that failed outside a handler's own error
// replies: an unknown route, a body over the limit, a panic. The client gets
// the status and its standard text. The cause goes to the log, never out.
func ErrorHandler(logger *slog.Logger) cartridge.ErrorHandler {
	return func(ctx *cartridge.Context, err error) error {
		code := http.StatusInternalServerError
		var httpErr *cartridge.Error
		if errors.As(err, &httpErr) {
			code = httpErr.Code
		}

		if code >= http.StatusInternalServerError {
			logger.Error("Request failed", slog.Any("error", err), slog.Int("status", code),
				slog.String("method", ctx.Method()), slog.String("path", ctx.Path()))
		}

		message := http.StatusText(code)
		if ctx.Accepts("application/json") == "application/json" {
			return ctx.Status(code).JSON(cartridge.Map{"error": message})
		}
		return ctx.Status(code).SendString(message)
	}
}
