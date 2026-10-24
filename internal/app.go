// Package internal contains LogNorth application functionality
package internal

import (
	"fmt"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"os"
	"time"

	"strings"

	"github.com/karloscodes/cartridge"
	"github.com/karloscodes/cartridge/inertia"
	cartridgemiddleware "github.com/karloscodes/cartridge/middleware"

	lognorth "lognorth"
	"lognorth/internal/config"
	"lognorth/internal/database"
	lbhttp "lognorth/internal/http"
	"lognorth/internal/jobs"
	"lognorth/internal/users"
)

// Application wraps cartridge.Application with LogNorth-specific components
type Application struct {
	*cartridge.Application
	DBManager *database.DBManager
}

// NewApp creates a new LogNorth application instance
func NewApp() (*Application, error) {
	cfg := config.GetConfig()

	// Ensure storage directory exists
	if err := os.MkdirAll(cfg.StoragePath, 0755); err != nil {
		return nil, fmt.Errorf("failed to create storage directory: %w", err)
	}

	// Enable dev mode in development
	if cfg.IsDevelopment() {
		lbhttp.SetDevMode(true)
	}

	// Set page title
	lbhttp.SetTitle("LogNorth")

	// The frontend runs Inertia v3, which reads the first page from a JSON
	// script element, not from an attribute on the root div.
	inertia.SetScriptElement(true)

	// Create logger
	logger := cartridge.NewLogger(cfg, nil)

	// Initialize database manager
	dbManager := database.NewDBManager(cfg, logger)
	if err := dbManager.Init(); err != nil {
		return nil, fmt.Errorf("failed to initialize database: %w", err)
	}

	// Run database migrations
	if err := dbManager.MigrateDatabase(); err != nil {
		return nil, fmt.Errorf("failed to run database migrations: %w", err)
	}

	// Create job dispatchers using Cartridge's built-in framework
	eventProcessor := cartridge.NewJobDispatcher(logger, dbManager, 5*time.Second, jobs.NewEventProcessor())
	cleanupProcessor := cartridge.NewJobDispatcher(logger, dbManager, 10*time.Minute, jobs.NewCleanupProcessor())

	// SPC alerting: detection runs in one job. Baselines come from stored
	// history on every check; nothing is learned or saved.
	anomalyChecker := cartridge.NewJobDispatcher(logger, dbManager, 5*time.Minute, jobs.NewAnomalyChecker())
	digestSender := cartridge.NewJobDispatcher(logger, dbManager, 1*time.Minute, jobs.NewDigestSender())

	// Uptime ping (passive silence detection at the app level).
	uptimeSweeper := cartridge.NewJobDispatcher(logger, dbManager, 1*time.Minute, jobs.NewUptimeSweeper())

	// Configure server
	serverConfig := cartridge.DefaultServerConfig()
	// Sec-Fetch-Site (CSRF protection) stays on for every route. Routes that
	// non-browser clients call (ingest, OTel, MCP) opt out one by one.

	// The installer puts kamal-proxy in front on the Docker network. Take the
	// client IP from X-Forwarded-For only when the request comes from a
	// private or loopback address; otherwise every client shares the proxy's
	// IP in the rate limiters. A client that reaches the app directly cannot
	// set its own IP this way.
	// In development the page loads from the Vite dev server, which the
	// policy would block.
	if !cfg.IsDevelopment() {
		serverConfig.ContentSecurityPolicy = contentSecurityPolicy
	}
	serverConfig.ProxyHeader = "X-Forwarded-For"
	serverConfig.TrustedProxies = trustedProxies

	if cfg.IsDevelopment() {
		// Development: serve from disk for hot-reload
		serverConfig.PublicDirectory = "web/public"
	} else {
		// Production: serve from embedded filesystem
		staticFS, _ := fs.Sub(lognorth.StaticFS, "web/dist/assets")
		publicFS, _ := fs.Sub(lognorth.PublicFS, "web/public")
		serverConfig.StaticFS = staticFS
		serverConfig.PublicFS = publicFS
		inertia.SetManifestData(lognorth.ManifestData)
	}

	// Create the cartridge application with background workers
	app, err := cartridge.NewApplication(cartridge.ApplicationOptions{
		Config:            cfg,
		Logger:            logger,
		DBManager:         dbManager,
		ServerConfig:      serverConfig,
		RouteMountFunc:    MountRoutes,
		BackgroundWorkers: []cartridge.BackgroundWorker{eventProcessor, cleanupProcessor, anomalyChecker, digestSender, uptimeSweeper},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create application: %w", err)
	}

	return &Application{
		Application: app,
		DBManager:   dbManager,
	}, nil
}

// contentSecurityPolicy lets the browser load only what the app ships: its
// own scripts, styles, fonts, and images. Inline styles stay allowed because
// components set them. Nothing inline runs as script.
const contentSecurityPolicy = "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; " +
	"img-src 'self' data:; font-src 'self'; connect-src 'self'; frame-ancestors 'self'; " +
	"base-uri 'self'; form-action 'self'; object-src 'none'"

// MountRoutes sets up all application routes
func MountRoutes(server *cartridge.Server) {
	cfg := config.GetConfig()

	// Set up session manager
	sessionMgr, err := cartridge.NewSessionManager(cartridge.SessionConfig{
		CookieName: "lognorth_session",
		Secret:     cfg.GetSessionSecret(),
		TTL:        time.Duration(cfg.GetLoginSessionTimeout()) * time.Second,
		Insecure:   !cfg.IsProduction(),
		LoginPath:  "/login",
	})
	if err != nil {
		// GetSessionSecret always returns 32 bytes or more.
		panic(fmt.Sprintf("session manager: %v", err))
	}
	server.SetSession(sessionMgr)

	// Get dependencies for middleware
	dbMgr := server.GetDBManager().(*database.DBManager)
	logger := server.GetLogger()

	// Rate limiters (only enabled in production)
	ingestRateLimiter := conditionalRateLimiter(cfg, cartridgemiddleware.RateLimiter(
		cartridgemiddleware.WithMax(1000),
		cartridgemiddleware.WithDuration(time.Minute),
		cartridgemiddleware.WithKeyGenerator(clientIP),
	))
	loginRateLimiter := conditionalRateLimiter(cfg, cartridgemiddleware.RateLimiter(
		cartridgemiddleware.WithMax(5),
		cartridgemiddleware.WithDuration(time.Minute),
		cartridgemiddleware.WithKeyGenerator(clientIP),
	))
	agentRateLimiter := conditionalRateLimiter(cfg, cartridgemiddleware.RateLimiter(
		cartridgemiddleware.WithMax(60),
		cartridgemiddleware.WithDuration(time.Minute),
		cartridgemiddleware.WithKeyGenerator(clientIP),
	))

	// Onboarding check middleware
	onboardingCheck := onboardingCheckMiddleware(dbMgr, logger)

	// Health check (no auth, no CSRF check). /up is the path that Chasen asks.
	health := func(c *cartridge.Context) error {
		return c.SendString("ok")
	}
	server.Get("/_health", health, &cartridge.RouteConfig{EnableSecFetchSite: cartridge.Bool(false)})
	server.Get("/up", health, &cartridge.RouteConfig{EnableSecFetchSite: cartridge.Bool(false)})

	// isFile says whether a path is the health check or a static file: what a
	// page load fetches besides the page.
	publicFiles, _ := fs.Sub(lognorth.PublicFS, "web/public")
	isFile := func(path string) bool {
		if path == "/_health" || path == "/up" || strings.HasPrefix(path, "/assets/") {
			return true
		}
		info, err := fs.Stat(publicFiles, strings.TrimPrefix(path, "/"))
		return err == nil && !info.IsDir()
	}

	// A status page on its own domain (status.example.com) answers before
	// any other route, so nothing else answers on that domain.
	server.Use(lbhttp.StatusDomainMiddleware(isFile))

	// Demo mode: global rate limit + block write operations. server.Use runs
	// on every request, so two kinds opt out of the limit. The health check:
	// the proxy calls it more often than the limit allows. Static files: one
	// page load fetches several, and a refused script leaves a blank page.
	if cfg.IsDemo() {
		notAPage := func(c *cartridge.Context) bool { return isFile(c.Path()) }
		server.Use(cartridgemiddleware.RateLimiter(
			cartridgemiddleware.WithMax(60),
			cartridgemiddleware.WithDuration(time.Minute),
			cartridgemiddleware.WithKeyGenerator(clientIP),
			cartridgemiddleware.WithSkip(notAPage),
		))
		server.Use(func(c *cartridge.Context) error {
			method := c.Method()
			if method == "POST" || method == "PUT" || method == "DELETE" || method == "PATCH" {
				if strings.HasPrefix(c.Path(), "/api/") {
					return c.Next()
				}
				return c.RedirectBack("/")
			}
			return c.Next()
		})
	}

	// API routes (no CSRF, uses API key auth + rate limiting)
	apiConfig := &cartridge.RouteConfig{
		EnableSecFetchSite: cartridge.Bool(false),
		CustomMiddleware:   []cartridge.HandlerFunc{ingestRateLimiter},
	}
	server.Post("/api/v1/events", lbhttp.IngestAction, apiConfig)
	server.Post("/api/v1/events/batch", lbhttp.IngestAction, apiConfig)

	// OpenTelemetry OTLP HTTP/protobuf endpoints
	// SDKs need: OTEL_EXPORTER_OTLP_ENDPOINT=https://logs.yoursite.com/otel
	server.Post("/otel/v1/logs", lbhttp.OtelIngestAction, apiConfig)
	server.Post("/otel/v1/traces", lbhttp.OtelSinkAction, apiConfig)
	server.Post("/otel/v1/metrics", lbhttp.OtelSinkAction, apiConfig)

	// MCP endpoint: the only way an agent reads the log. Authenticated with
	// the agent key, rate limited, read-only tools.
	agentConfig := &cartridge.RouteConfig{
		EnableSecFetchSite: cartridge.Bool(false),
		CustomMiddleware:   []cartridge.HandlerFunc{agentRateLimiter},
	}
	server.Post("/mcp", lbhttp.MCPAction, agentConfig)

	// Protected page config (with session auth + onboarding check)
	// In demo mode, skip auth entirely — everyone sees the app
	var protectedMiddleware []cartridge.HandlerFunc
	if cfg.IsDemo() {
		protectedMiddleware = []cartridge.HandlerFunc{}
	} else {
		protectedMiddleware = []cartridge.HandlerFunc{
			onboardingCheck,
			sessionMgr.Middleware(),
			lbhttp.RequireLiveSession,
		}
	}
	protectedConfig := &cartridge.RouteConfig{
		CustomMiddleware:   protectedMiddleware,
		EnableSecFetchSite: cartridge.Bool(true),
	}

	// Login/logout and onboarding
	if cfg.IsDemo() {
		// In demo mode, redirect login/setup to home
		demoRedirect := func(ctx *cartridge.Context) error {
			return ctx.Redirect("/")
		}
		server.Get("/login", demoRedirect, nil)
		server.Get("/setup", demoRedirect, nil)
	} else {
		loginConfig := &cartridge.RouteConfig{
			EnableSecFetchSite: cartridge.Bool(false),
		}
		loginPostConfig := &cartridge.RouteConfig{
			EnableSecFetchSite: cartridge.Bool(true),
			CustomMiddleware:   []cartridge.HandlerFunc{loginRateLimiter},
		}
		server.Get("/login", lbhttp.LoginPageAction, loginConfig)
		server.Post("/login", lbhttp.LoginAction, loginPostConfig)
		server.Post("/logout", lbhttp.LogoutAction, protectedConfig)

		// Onboarding (no auth required - this is initial setup)
		onboardingConfig := &cartridge.RouteConfig{
			EnableSecFetchSite: cartridge.Bool(false),
		}
		onboardingPostConfig := &cartridge.RouteConfig{
			EnableSecFetchSite: cartridge.Bool(true),
		}
		server.Get("/setup", lbhttp.OnboardingPageAction, onboardingConfig)
		server.Post("/setup", lbhttp.OnboardingAction, onboardingPostConfig)
	}

	// Status pages: public, so visitors read them without an account. Only
	// the signed-in owner writes notes on their outages.
	server.Get("/status/:slug", lbhttp.StatusPageAction, &cartridge.RouteConfig{EnableSecFetchSite: cartridge.Bool(false)})
	server.Post("/status/comments", lbhttp.AddStatusCommentAction, protectedConfig)
	server.Post("/status/comments/:id/delete", lbhttp.DeleteStatusCommentAction, protectedConfig)

	// Event JSON for the Logs and Endpoints pages. Session only: agents read
	// through MCP, and there is no public read API.
	server.Get("/api/v1/events", lbhttp.EventsAPIAction, protectedConfig)

	// Home page
	server.Get("/", lbhttp.HomePageAction, protectedConfig)

	// Setup page (apps management)
	server.Get("/apps", lbhttp.SetupPageAction, protectedConfig)
	server.Post("/apps/api-keys", lbhttp.CreateAPIKeyAction, protectedConfig)
	server.Post("/apps/api-keys/:id/rename", lbhttp.RenameAPIKeyAction, protectedConfig)
	server.Post("/apps/api-keys/:id/ping-url", lbhttp.SetPingURLAction, protectedConfig)
	server.Post("/apps/api-keys/:id/toggle-ping", lbhttp.TogglePingAction, protectedConfig)
	server.Post("/apps/api-keys/:id/status-page", lbhttp.SetAppStatusPageAction, protectedConfig)
	server.Post("/apps/api-keys/:id/revoke", lbhttp.RevokeAPIKeyAction, protectedConfig)
	server.Post("/apps/api-keys/:id/delete", lbhttp.DeleteAPIKeyAction, protectedConfig)

	// Global log redirects to home (force app selection)
	redirectHome := func(ctx *cartridge.Context) error {
		return ctx.Redirect("/")
	}
	server.Get("/log", redirectHome, protectedConfig)
	server.Get("/issues", redirectHome, protectedConfig)

	// App-scoped pages
	server.Get("/apps/:id/issues", lbhttp.AppIssuesPageAction, protectedConfig)
	server.Get("/apps/:id/log", lbhttp.AppLogPageAction, protectedConfig)
	server.Get("/apps/:id/log/:eventId", lbhttp.AppLogDetailPageAction, protectedConfig)
	server.Get("/apps/:id/endpoints", lbhttp.EndpointsPageAction, protectedConfig)

	// Trace view (global - traces can span apps)
	server.Get("/trace/:traceId", lbhttp.TracePageAction, protectedConfig)

	// Issue detail page
	server.Get("/apps/:id/issues/:hash", lbhttp.AppIssueDetailPageAction, protectedConfig)

	// Global issue detail (email alerts link here) — redirects to app-scoped page
	server.Get("/issues/:hash", lbhttp.IssueDetailPageAction, protectedConfig)

	// Issue actions
	server.Post("/issues/seen", lbhttp.MarkAllIssuesSeenAction, protectedConfig)
	server.Post("/issues/:hash/seen", lbhttp.MarkIssueSeenAction, protectedConfig)
	server.Post("/issues/:hash/mute", lbhttp.MuteIssueAction, protectedConfig)
	server.Post("/issues/:hash/unmute", lbhttp.UnmuteIssueAction, protectedConfig)

	// Settings — split into per-concern sub-pages. /settings redirects.
	server.Get("/settings", func(ctx *cartridge.Context) error {
		return ctx.Redirect("/settings/alerts")
	}, protectedConfig)
	server.Get("/settings/alerts", lbhttp.SettingsAlertsPageAction, protectedConfig)
	server.Get("/settings/data", lbhttp.SettingsDataPageAction, protectedConfig)
	server.Get("/settings/developer", lbhttp.SettingsDeveloperPageAction, protectedConfig)
	server.Get("/settings/appearance", lbhttp.SettingsAppearancePageAction, protectedConfig)
	server.Get("/settings/status", lbhttp.SettingsStatusPageAction, protectedConfig)
	server.Post("/settings/status", lbhttp.CreateStatusPageAction, protectedConfig)
	server.Post("/settings/status/:id", lbhttp.UpdateStatusPageAction, protectedConfig)
	server.Post("/settings/status/:id/delete", lbhttp.DeleteStatusPageAction, protectedConfig)
	server.Post("/settings", lbhttp.UpdateSettingsAction, protectedConfig)
	server.Post("/settings/test", lbhttp.TestNotificationAction, protectedConfig)

	// Agent API key management
	server.Post("/settings/agent-key", lbhttp.CreateAgentKeyAction, protectedConfig)
	server.Post("/settings/agent-key/regenerate", lbhttp.RegenerateAgentKeyAction, protectedConfig)
	server.Post("/settings/agent-key/revoke", lbhttp.RevokeAgentKeyAction, protectedConfig)
}

// clientIP is the rate limiter key. kamal-proxy appends the address it saw to
// X-Forwarded-For, so behind it the last entry is the real client; the earlier
// entries come from the client and can be anything, which is what c.IP()
// returns. A request that did not come through a trusted proxy is keyed on
// the connection address.
func clientIP(c *cartridge.Context) string {
	peer := peerIP(c.Request().RemoteAddr)
	if isTrustedProxy(peer) {
		if forwarded := c.Get("X-Forwarded-For"); forwarded != "" {
			entries := strings.Split(forwarded, ",")
			return strings.TrimSpace(entries[len(entries)-1])
		}
	}
	return peer
}

// trustedProxies are the peers whose X-Forwarded-For counts: loopback and
// the private ranges a Docker network uses.
var trustedProxies = []string{"127.0.0.1", "::1", "10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16", "fc00::/7"}

// peerIP is the address of the connection, without the port.
func peerIP(remoteAddr string) string {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		return remoteAddr
	}
	return host
}

func isTrustedProxy(ip string) bool {
	addr, err := netip.ParseAddr(ip)
	if err != nil {
		return false
	}
	addr = addr.Unmap()
	for _, entry := range trustedProxies {
		if prefix, err := netip.ParsePrefix(entry); err == nil {
			if prefix.Contains(addr) {
				return true
			}
		} else if trusted, err := netip.ParseAddr(entry); err == nil && trusted == addr {
			return true
		}
	}
	return false
}

// onboardingCheckMiddleware redirects to setup if no users exist
func onboardingCheckMiddleware(dbMgr *database.DBManager, logger *slog.Logger) cartridge.HandlerFunc {
	return func(c *cartridge.Context) error {
		db := dbMgr.GetConnection()

		required, err := users.IsOnboardingRequired(db)
		if err != nil {
			logger.Error("Failed to check if onboarding required", slog.Any("error", err))
			return c.Status(http.StatusInternalServerError).SendString("System error")
		}

		if required {
			return c.Redirect("/setup", http.StatusFound)
		}

		return c.Next()
	}
}

// conditionalRateLimiter returns the rate limiter in production, passthrough in dev
func conditionalRateLimiter(cfg *config.Config, limiter cartridge.HandlerFunc) cartridge.HandlerFunc {
	if cfg.IsProduction() {
		return limiter
	}
	return func(c *cartridge.Context) error {
		return c.Next()
	}
}
