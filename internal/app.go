// Package internal contains LogNorth application functionality
package internal

import (
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"time"

	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/karloscodes/cartridge"
	"github.com/karloscodes/cartridge/inertia"
	cartridgemiddleware "github.com/karloscodes/cartridge/middleware"

	lognorth "lognorth"
	"lognorth/internal/config"
	"lognorth/internal/database"
	lbhttp "lognorth/internal/http"
	"lognorth/internal/jobs"
	"lognorth/internal/license"
	"lognorth/internal/onboarding"
	"lognorth/internal/settings"
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

	// SPC alerting jobs (replaces old spike checker)
	baselineUpdater := cartridge.NewJobDispatcher(logger, dbManager, 5*time.Minute, jobs.NewBaselineUpdater())
	anomalyChecker := cartridge.NewJobDispatcher(logger, dbManager, 5*time.Minute, jobs.NewAnomalyChecker())
	digestSender := cartridge.NewJobDispatcher(logger, dbManager, 1*time.Minute, jobs.NewDigestSender())

	// Uptime ping (passive silence detection at the app level).
	uptimeSweeper := cartridge.NewJobDispatcher(logger, dbManager, 1*time.Minute, jobs.NewUptimeSweeper())

	// License revalidation: hits Polar once a day to catch revoked/expired keys.
	licenseChecker := cartridge.NewJobDispatcher(logger, dbManager, 24*time.Hour, jobs.NewLicenseChecker())

	// Configure server
	serverConfig := cartridge.DefaultServerConfig()
	// Sec-Fetch-Site is enabled per-route for web routes, disabled for API
	serverConfig.EnableSecFetchSite = false

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
		BackgroundWorkers: []cartridge.BackgroundWorker{eventProcessor, cleanupProcessor, baselineUpdater, anomalyChecker, digestSender, uptimeSweeper, licenseChecker},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create application: %w", err)
	}

	// Set initial license status from database
	if s, err := settings.Get(dbManager.GetConnection()); err == nil {
		license.SetConfigured(s.LicenseStatus == "valid")
	}

	return &Application{
		Application: app,
		DBManager:   dbManager,
	}, nil
}

// MountRoutes sets up all application routes
func MountRoutes(server *cartridge.Server) {
	app := server.App()
	cfg := config.GetConfig()

	// Set up session manager
	sessionMgr := cartridge.NewSessionManager(cartridge.SessionConfig{
		CookieName: "lognorth_session",
		Secret:     cfg.GetSessionSecret(),
		TTL:        time.Duration(cfg.GetLoginSessionTimeout()) * time.Second,
		Secure:     cfg.IsProduction(),
		LoginPath:  "/login",
	})
	server.SetSession(sessionMgr)

	// Get dependencies for middleware
	dbMgr := server.GetDBManager().(*database.DBManager)
	logger := server.GetLogger()

	// Rate limiters (only enabled in production)
	ingestRateLimiter := conditionalRateLimiter(cfg, cartridgemiddleware.RateLimiter(
		cartridgemiddleware.WithMax(1000),
		cartridgemiddleware.WithDuration(time.Minute),
	))
	loginRateLimiter := conditionalRateLimiter(cfg, cartridgemiddleware.RateLimiter(
		cartridgemiddleware.WithMax(5),
		cartridgemiddleware.WithDuration(time.Minute),
	))
	agentRateLimiter := conditionalRateLimiter(cfg, cartridgemiddleware.RateLimiter(
		cartridgemiddleware.WithMax(60),
		cartridgemiddleware.WithDuration(time.Minute),
	))

	// Onboarding check middleware
	onboardingCheck := onboardingCheckMiddleware(dbMgr, logger)

	// Health check (no auth, no middleware)
	app.Get("/_health", func(c *fiber.Ctx) error {
		return c.SendString("ok")
	})

	// Demo mode: global rate limit + block write operations
	if cfg.IsDemo() {
		app.Use(cartridgemiddleware.RateLimiter(
			cartridgemiddleware.WithMax(60),
			cartridgemiddleware.WithDuration(time.Minute),
		))
		app.Use(func(c *fiber.Ctx) error {
			method := c.Method()
			if method == "POST" || method == "PUT" || method == "DELETE" || method == "PATCH" {
				if strings.HasPrefix(c.Path(), "/api/") {
					return c.Next()
				}
				referer := c.Get("Referer", "/")
				return c.Redirect(referer)
			}
			return c.Next()
		})
	}

	// API routes (no CSRF, uses API key auth + rate limiting)
	apiConfig := &cartridge.RouteConfig{
		EnableSecFetchSite: cartridge.Bool(false),
		CustomMiddleware:   []fiber.Handler{ingestRateLimiter},
	}
	server.Post("/api/v1/events", lbhttp.IngestAction, apiConfig)
	server.Post("/api/v1/events/batch", lbhttp.IngestAction, apiConfig)

	// OpenTelemetry OTLP HTTP/protobuf endpoints
	// SDKs need: OTEL_EXPORTER_OTLP_ENDPOINT=https://logs.yoursite.com/otel
	server.Post("/otel/v1/logs", lbhttp.OtelIngestAction, apiConfig)
	server.Post("/otel/v1/traces", lbhttp.OtelSinkAction, apiConfig)
	server.Post("/otel/v1/metrics", lbhttp.OtelSinkAction, apiConfig)

	// SSE endpoint (no middleware needed)
	apiReadConfig := &cartridge.RouteConfig{
		EnableSecFetchSite: cartridge.Bool(false),
	}
	app.Get("/api/v1/events/live", lbhttp.LiveEventsRawAction)
	server.Get("/api/v1/events", lbhttp.EventsAPIAction, apiReadConfig)
	server.Get("/api/v1/events/:id", lbhttp.EventDetailAPIAction, apiReadConfig)
	server.Get("/api/v1/issues", lbhttp.IssuesAPIAction, apiReadConfig)
	server.Get("/api/v1/api-keys", lbhttp.ListAPIKeysAction, apiReadConfig)

	// MCP endpoint: the only way an agent reads the log. Authenticated with
	// the agent key, rate limited, read-only tools.
	agentConfig := &cartridge.RouteConfig{
		EnableSecFetchSite: cartridge.Bool(false),
		CustomMiddleware:   []fiber.Handler{agentRateLimiter},
	}
	server.Post("/mcp", lbhttp.MCPAction, agentConfig)

	// Protected page config (with session auth + onboarding check)
	// In demo mode, skip auth entirely — everyone sees the app
	var protectedMiddleware []fiber.Handler
	if cfg.IsDemo() {
		protectedMiddleware = []fiber.Handler{}
	} else {
		protectedMiddleware = []fiber.Handler{
			onboardingCheck,
			sessionMgr.Middleware(),
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
			return ctx.Ctx.Redirect("/")
		}
		server.Get("/login", demoRedirect, nil)
		server.Get("/setup", demoRedirect, nil)
	} else {
		loginConfig := &cartridge.RouteConfig{
			EnableSecFetchSite: cartridge.Bool(false),
		}
		loginPostConfig := &cartridge.RouteConfig{
			EnableSecFetchSite: cartridge.Bool(true),
			CustomMiddleware:   []fiber.Handler{loginRateLimiter},
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
		server.Post("/setup/user", lbhttp.OnboardingUserAction, onboardingPostConfig)
		server.Post("/setup/password", lbhttp.OnboardingPasswordAction, onboardingPostConfig)
	}

	// Home page
	server.Get("/", lbhttp.HomePageAction, protectedConfig)

	// Setup page (apps management)
	server.Get("/apps", lbhttp.SetupPageAction, protectedConfig)
	server.Post("/apps/api-keys", lbhttp.CreateAPIKeyAction, protectedConfig)
	server.Post("/apps/api-keys/:id/rename", lbhttp.RenameAPIKeyAction, protectedConfig)
	server.Post("/apps/api-keys/:id/ping-url", lbhttp.SetPingURLAction, protectedConfig)
	server.Post("/apps/api-keys/:id/toggle-ping", lbhttp.TogglePingAction, protectedConfig)
	server.Post("/apps/api-keys/:id/revoke", lbhttp.RevokeAPIKeyAction, protectedConfig)
	server.Delete("/apps/api-keys/:id", lbhttp.DeleteAPIKeyAction, protectedConfig)

	// Global log redirects to home (force app selection)
	redirectHome := func(ctx *cartridge.Context) error {
		return ctx.Ctx.Redirect("/")
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
		return ctx.Ctx.Redirect("/settings/license")
	}, protectedConfig)
	server.Get("/settings/license", lbhttp.SettingsLicensePageAction, protectedConfig)
	server.Get("/settings/alerts", lbhttp.SettingsAlertsPageAction, protectedConfig)
	server.Get("/settings/data", lbhttp.SettingsDataPageAction, protectedConfig)
	server.Get("/settings/developer", lbhttp.SettingsDeveloperPageAction, protectedConfig)
	server.Post("/settings", lbhttp.UpdateSettingsAction, protectedConfig)
	server.Post("/settings/test", lbhttp.TestNotificationAction, protectedConfig)
	server.Post("/settings/test-smtp", lbhttp.TestSMTPAction, protectedConfig)

	// Agent API key management
	server.Post("/settings/agent-key", lbhttp.CreateAgentKeyAction, protectedConfig)
	server.Post("/settings/agent-key/regenerate", lbhttp.RegenerateAgentKeyAction, protectedConfig)
	server.Post("/settings/agent-key/revoke", lbhttp.RevokeAgentKeyAction, protectedConfig)
}

// onboardingCheckMiddleware redirects to setup if no users exist
func onboardingCheckMiddleware(dbMgr *database.DBManager, logger *slog.Logger) fiber.Handler {
	return func(c *fiber.Ctx) error {
		db := dbMgr.GetConnection()

		required, err := onboarding.IsOnboardingRequired(db)
		if err != nil {
			logger.Error("Failed to check if onboarding required", slog.Any("error", err))
			return c.Status(fiber.StatusInternalServerError).SendString("System error")
		}

		if required {
			return c.Redirect("/setup", fiber.StatusFound)
		}

		return c.Next()
	}
}

// conditionalRateLimiter returns the rate limiter in production, passthrough in dev
func conditionalRateLimiter(cfg *config.Config, limiter fiber.Handler) fiber.Handler {
	if cfg.IsProduction() {
		return limiter
	}
	return func(c *fiber.Ctx) error {
		return c.Next()
	}
}
