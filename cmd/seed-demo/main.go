// Seed command to populate LogNorth demo with realistic data
package main

import (
	"fmt"
	"log"
	"log/slog"
	"math/rand"
	"os"
	"time"

	"lognorth/internal/apikeys"
	"lognorth/internal/config"
	"lognorth/internal/database"
	"lognorth/internal/events"
	"lognorth/internal/users"
)

var httpPaths = []string{
	"/api/users",
	"/api/users/:id",
	"/api/users/:id/profile",
	"/api/orders",
	"/api/orders/:id",
	"/api/orders/:id/refund",
	"/api/products",
	"/api/products/:id",
	"/api/products/search",
	"/api/auth/login",
	"/api/auth/logout",
	"/api/auth/refresh",
	"/api/cart",
	"/api/cart/items",
	"/api/checkout",
	"/api/checkout/confirm",
	"/api/payments/webhook",
	"/api/webhooks/stripe",
	"/api/notifications",
	"/api/search",
	"/api/uploads",
	"/api/settings",
	"/health",
	"/api/analytics/events",
	"/api/reports/daily",
}

var methods = []string{"GET", "GET", "GET", "POST", "PUT", "DELETE"} // GET-heavy like real traffic

var jobNames = []string{
	"SendWeeklyDigest",
	"ProcessPayments",
	"SyncInventory",
	"GenerateDailyReport",
	"CleanupExpiredSessions",
	"RefreshSearchIndex",
	"SendAbandonedCartEmails",
	"ProcessRefunds",
}

var errorMessages = []string{
	"connection timeout after 30s",
	"record not found",
	"invalid authentication token",
	"Stripe: card_declined",
	"rate limit exceeded (429)",
	"unexpected EOF",
	"Redis: connection refused",
	"JSON: unexpected token at position 42",
	"permission denied",
	"duplicate key: email already exists",
	"context deadline exceeded",
	"TLS handshake timeout",
}

var userAgents = []string{
	"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7)",
	"Mozilla/5.0 (Windows NT 10.0; Win64; x64)",
	"Mozilla/5.0 (iPhone; CPU iPhone OS 17_0)",
	"curl/8.1.2",
	"PostmanRuntime/7.32.3",
	"axios/1.6.0",
}

type appConfig struct {
	name         string
	requestCount int
	errorRate    float64
}

func main() {
	cfg := config.GetConfig()

	if err := os.MkdirAll(cfg.StoragePath, 0755); err != nil {
		log.Fatalf("Failed to create storage directory: %v", err)
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	dbManager := database.NewDBManager(cfg, logger)
	if err := dbManager.Init(); err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}

	if err := dbManager.MigrateDatabase(); err != nil {
		log.Fatalf("Failed to run migrations: %v", err)
	}

	db := dbManager.GetConnection()

	// Create demo user
	userCount, _ := users.Count(db)
	if userCount == 0 {
		if err := users.CreateUser(db, "demo@lognorth.com", "demo"); err != nil {
			log.Printf("Failed to create demo user: %v", err)
		} else {
			fmt.Println("Created demo user (demo@lognorth.com)")
		}
	}

	// Check if already seeded
	existingKeys, _ := apikeys.List(db)
	if len(existingKeys) > 0 {
		fmt.Println("Already seeded. Delete the database to re-seed.")
		return
	}

	apps := []appConfig{
		{name: "acme-api", requestCount: 2000, errorRate: 0.08},
		{name: "acme-web", requestCount: 800, errorRate: 0.05},
		{name: "acme-workers", requestCount: 400, errorRate: 0.12},
	}

	fmt.Println("Seeding demo data...")
	fmt.Println()

	for _, app := range apps {
		apiKey, _, err := apikeys.Create(db, app.name)
		if err != nil {
			log.Printf("Failed to create API key for %s: %v", app.name, err)
			continue
		}

		now := time.Now()
		totalEvents := 0
		errorCount := 0

		// Generate HTTP request events spread over 7 days
		httpCount := int(float64(app.requestCount) * 0.75)
		for i := 0; i < httpCount; i++ {
			// More recent events are more likely (realistic traffic pattern)
			hoursAgo := int(float64(rand.Intn(168)) * float64(rand.Intn(100)) / 100.0)
			if hoursAgo > 168 {
				hoursAgo = 168
			}
			minutesAgo := rand.Intn(60)
			timestamp := now.Add(-time.Duration(hoursAgo)*time.Hour - time.Duration(minutesAgo)*time.Minute)

			method := methods[rand.Intn(len(methods))]
			path := httpPaths[rand.Intn(len(httpPaths))]
			traceID := fmt.Sprintf("req_%x", rand.Int63())

			// Realistic duration distribution: mostly fast, some slow
			duration := 5 + rand.Intn(50)
			if rand.Float64() < 0.1 {
				duration = 200 + rand.Intn(2000) // slow requests
			}

			isError := rand.Float64() < app.errorRate
			var status int
			var errorMsg string

			if isError {
				status = []int{500, 502, 503, 500, 500}[rand.Intn(5)] // 500 most common
				errorMsg = errorMessages[rand.Intn(len(errorMessages))]
				errorCount++
				duration = duration * 3 // errors tend to be slower
			} else if rand.Float64() < 0.08 {
				status = []int{400, 401, 403, 404, 429}[rand.Intn(5)]
			} else {
				status = []int{200, 200, 200, 201, 204}[rand.Intn(5)] // 200 most common
			}

			message := fmt.Sprintf("%s %s → %d", method, path, status)

			ctx := &events.EventContext{
				Method: method,
				Path:   path,
				Status: status,
				IP:     fmt.Sprintf("10.%d.%d.%d", rand.Intn(10), rand.Intn(256), rand.Intn(256)),
			}
			if errorMsg != "" {
				ctx.Error = errorMsg
			}
			if rand.Float64() < 0.4 {
				ctx.User = fmt.Sprintf("user_%d", rand.Intn(50))
			}

			event := &events.Event{
				APIKeyID:   apiKey.ID,
				Timestamp:  timestamp,
				Message:    message,
				TraceID:    traceID,
				DurationMS: duration,
				IsError:    isError,
			}
			event.SetContext(ctx)

			if err := events.Create(db, event); err != nil {
				continue
			}
			totalEvents++
		}

		// Generate background job events
		jobCount := app.requestCount - httpCount
		for i := 0; i < jobCount; i++ {
			hoursAgo := rand.Intn(168)
			minutesAgo := rand.Intn(60)
			timestamp := now.Add(-time.Duration(hoursAgo)*time.Hour - time.Duration(minutesAgo)*time.Minute)

			jobName := jobNames[rand.Intn(len(jobNames))]
			traceID := fmt.Sprintf("job_%x", rand.Int63())
			duration := 500 + rand.Intn(15000)

			isError := rand.Float64() < app.errorRate
			var message string
			var errorMsg string

			if isError {
				message = fmt.Sprintf("%s failed", jobName)
				errorMsg = errorMessages[rand.Intn(len(errorMessages))]
				errorCount++
			} else {
				message = fmt.Sprintf("%s completed", jobName)
			}

			ctx := &events.EventContext{
				Job: jobName,
			}
			if errorMsg != "" {
				ctx.Error = errorMsg
			}

			event := &events.Event{
				APIKeyID:   apiKey.ID,
				Timestamp:  timestamp,
				Message:    message,
				TraceID:    traceID,
				DurationMS: duration,
				IsError:    isError,
			}
			event.SetContext(ctx)

			if err := events.Create(db, event); err != nil {
				continue
			}
			totalEvents++
		}

		// Generate clusters of recent errors (makes issues page interesting)
		recentErrorClusters := []struct {
			errorMsg string
			path     string
			method   string
			isJob    bool
			jobName  string
			count    int
			hoursAgo int
		}{
			{errorMsg: "connection timeout after 30s", path: "/api/users/:id", method: "GET", count: 23, hoursAgo: 1},
			{errorMsg: "Stripe: card_declined", path: "/api/checkout/confirm", method: "POST", count: 8, hoursAgo: 3},
			{errorMsg: "Redis: connection refused", path: "/api/cart", method: "GET", count: 45, hoursAgo: 0},
			{errorMsg: "context deadline exceeded", isJob: true, jobName: "ProcessPayments", count: 12, hoursAgo: 2},
			{errorMsg: "duplicate key: email already exists", path: "/api/users", method: "POST", count: 6, hoursAgo: 5},
			{errorMsg: "TLS handshake timeout", path: "/api/payments/webhook", method: "POST", count: 15, hoursAgo: 0},
		}

		for _, re := range recentErrorClusters {
			for j := 0; j < re.count; j++ {
				// Cluster errors close together in time (realistic)
				minutesSpread := rand.Intn(45)
				timestamp := now.Add(-time.Duration(re.hoursAgo)*time.Hour - time.Duration(minutesSpread)*time.Minute)
				traceID := fmt.Sprintf("err_%x", rand.Int63())
				duration := 100 + rand.Intn(3000)

				var message string
				var ctx *events.EventContext

				if re.isJob {
					message = fmt.Sprintf("%s failed", re.jobName)
					ctx = &events.EventContext{
						Job:   re.jobName,
						Error: re.errorMsg,
					}
				} else {
					message = fmt.Sprintf("%s %s → 500", re.method, re.path)
					ctx = &events.EventContext{
						Method: re.method,
						Path:   re.path,
						Status: 500,
						Error:  re.errorMsg,
						IP:     fmt.Sprintf("10.%d.%d.%d", rand.Intn(10), rand.Intn(256), rand.Intn(256)),
					}
					if rand.Float64() < 0.5 {
						ctx.User = fmt.Sprintf("user_%d", rand.Intn(50))
					}
				}

				event := &events.Event{
					APIKeyID:   apiKey.ID,
					Timestamp:  timestamp,
					Message:    message,
					TraceID:    traceID,
					DurationMS: duration,
					IsError:    true,
				}
				event.SetContext(ctx)

				if err := events.Create(db, event); err != nil {
					continue
				}
				totalEvents++
				errorCount++
			}
		}

		fmt.Printf("  %s: %d events (%d errors)\n", app.name, totalEvents, errorCount)
	}

	fmt.Println()
	fmt.Println("Done. Run with LOGNORTH_DEMO=true to start in demo mode.")
}
