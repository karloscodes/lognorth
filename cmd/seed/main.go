// Seed command to populate LogNorth with test data
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
	"lognorth/internal/settings"
	"lognorth/internal/users"
)

var httpPaths = []string{
	"/api/users",
	"/api/users/123",
	"/api/orders",
	"/api/orders/456",
	"/api/products",
	"/api/products/789",
	"/api/auth/login",
	"/api/auth/logout",
	"/api/cart",
	"/api/checkout",
	"/api/search",
	"/api/webhooks/stripe",
	"/health",
}

var methods = []string{"GET", "POST", "PUT", "DELETE"}

var jobNames = []string{
	"SendWeeklyEmails",
	"ProcessPayments",
	"SyncInventory",
	"GenerateReports",
	"CleanupExpiredSessions",
}

var errorMessages = []string{
	"Database connection timeout",
	"User not found",
	"Invalid authentication token",
	"Payment gateway error: card declined",
	"Rate limit exceeded",
	"Internal server error",
	"Redis connection refused",
	"JSON parse error: unexpected token",
}

type appConfig struct {
	name         string
	requestCount int
	errorRate    float64 // 0.0 to 1.0
}

var apps = []appConfig{
	{name: "Production API", requestCount: 100, errorRate: 0.15},
	{name: "Staging API", requestCount: 50, errorRate: 0.25},
	{name: "Admin Dashboard", requestCount: 25, errorRate: 0.10},
}

func main() {
	cfg := config.GetConfig()

	if err := os.MkdirAll(cfg.StoragePath, 0755); err != nil {
		log.Fatalf("Failed to create storage directory: %v", err)
	}

	// Use the shared database manager
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	dbManager := database.NewDBManager(cfg, logger)
	if err := dbManager.Init(); err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}

	// Run migrations
	if err := dbManager.MigrateDatabase(); err != nil {
		log.Fatalf("Failed to run migrations: %v", err)
	}

	db := dbManager.GetConnection()

	// Create default user if none exists
	userCount, _ := users.Count(db)
	if userCount == 0 {
		if err := users.CreateUser(db, "admin@localhost", "password"); err != nil {
			log.Printf("Failed to create default user: %v", err)
		} else {
			fmt.Println("Created default user:")
			fmt.Println("  Email: admin@localhost")
			fmt.Println("  Password: password")
			fmt.Println()
		}
	}

	// Check if already seeded
	existingKeys, _ := apikeys.List(db)
	if len(existingKeys) > 0 {
		fmt.Println("Database already has API keys. Run 'rm storage/lognorth-development.db' first to fully reseed.")
		return
	}

	fmt.Println("Seeding LogNorth...")
	fmt.Println()

	for _, app := range apps {
		// Create API key for this app
		apiKey, fullKey, err := apikeys.Create(db, app.name)
		if err != nil {
			log.Printf("Failed to create API key for %s: %v", app.name, err)
			continue
		}

		fmt.Printf("Created: %s\n", app.name)
		fmt.Printf("  API Key: %s\n", fullKey)

		// Generate events for this app
		now := time.Now()
		totalEvents := 0
		errorCount := 0

		// Generate HTTP request events (some with trace groups)
		httpCount := int(float64(app.requestCount) * 0.8)
		for i := 0; i < httpCount; i++ {
			// Random timestamp within last 3 days
			hoursAgo := rand.Intn(72)
			minutesAgo := rand.Intn(60)
			timestamp := now.Add(-time.Duration(hoursAgo)*time.Hour - time.Duration(minutesAgo)*time.Minute)

			method := methods[rand.Intn(len(methods))]
			path := httpPaths[rand.Intn(len(httpPaths))]
			traceID := fmt.Sprintf("req-%d-%04d", timestamp.Unix(), rand.Intn(10000))

			// Determine status and error
			isError := rand.Float64() < app.errorRate
			var status int
			var errorMsg string

			if isError {
				status = []int{500, 502, 503}[rand.Intn(3)]
				errorMsg = errorMessages[rand.Intn(len(errorMessages))]
				errorCount++
			} else if rand.Float64() < 0.15 {
				status = []int{400, 401, 404, 429}[rand.Intn(4)]
			} else {
				status = []int{200, 201, 204}[rand.Intn(3)]
			}

			ip := fmt.Sprintf("10.0.%d.%d", rand.Intn(256), rand.Intn(256))
			user := ""
			if rand.Float64() < 0.3 {
				user = fmt.Sprintf("user_%d", rand.Intn(100))
			}

			// 30% chance of creating a traced request (multiple events with same trace_id)
			if rand.Float64() < 0.3 {
				// Event 1: DB query
				dbEvent := &events.Event{
					APIKeyID:   apiKey.ID,
					Timestamp:  timestamp.Add(-50 * time.Millisecond),
					Message:    "SELECT * FROM users WHERE id = ?",
					TraceID:    traceID,
					DurationMS: 5 + rand.Intn(20),
					IsError:    false,
				}
				dbCtx := &events.EventContext{Path: path, Method: method}
				dbEvent.SetContext(dbCtx)
				if err := events.Create(db, dbEvent); err == nil {
					totalEvents++
				}

				// Event 2: External API call (sometimes)
				if rand.Float64() < 0.5 {
					apiEvent := &events.Event{
						APIKeyID:   apiKey.ID,
						Timestamp:  timestamp.Add(-20 * time.Millisecond),
						Message:    "POST https://api.stripe.com/v1/charges",
						TraceID:    traceID,
						DurationMS: 50 + rand.Intn(150),
						IsError:    false,
					}
					apiCtx := &events.EventContext{Path: path, Method: method}
					apiEvent.SetContext(apiCtx)
					if err := events.Create(db, apiEvent); err == nil {
						totalEvents++
					}
				}
			}

			// Main request event
			duration := 10 + rand.Intn(200)
			message := fmt.Sprintf("%s %s → %d", method, path, status)

			ctx := &events.EventContext{
				Method: method,
				Path:   path,
				Status: status,
				IP:     ip,
			}
			if errorMsg != "" {
				ctx.Error = errorMsg
			}
			if user != "" {
				ctx.User = user
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
				log.Printf("Failed to create event: %v", err)
				continue
			}
			totalEvents++
		}

		// Generate background job events
		jobCount := app.requestCount - httpCount
		for i := 0; i < jobCount; i++ {
			hoursAgo := rand.Intn(72)
			minutesAgo := rand.Intn(60)
			timestamp := now.Add(-time.Duration(hoursAgo)*time.Hour - time.Duration(minutesAgo)*time.Minute)

			jobName := jobNames[rand.Intn(len(jobNames))]
			traceID := fmt.Sprintf("job-%d-%04d", timestamp.Unix(), rand.Intn(10000))
			duration := 1000 + rand.Intn(30000) // Jobs take longer

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
				log.Printf("Failed to create event: %v", err)
				continue
			}
			totalEvents++
		}

		// Generate guaranteed recent errors (for issues page)
		recentErrors := []struct {
			message   string
			errorMsg  string
			path      string
			method    string
			isJob     bool
			jobName   string
			count     int
			hoursAgo  int
		}{
			{errorMsg: "Database connection timeout", path: "/api/users", method: "GET", count: 5, hoursAgo: 2},
			{errorMsg: "Payment gateway error: card declined", path: "/api/checkout", method: "POST", count: 3, hoursAgo: 4},
			{errorMsg: "Redis connection refused", path: "/api/cart", method: "GET", count: 4, hoursAgo: 6},
			{errorMsg: "Rate limit exceeded", isJob: true, jobName: "SendWeeklyEmails", count: 2, hoursAgo: 1},
			{errorMsg: "Invalid authentication token", path: "/api/auth/login", method: "POST", count: 6, hoursAgo: 3},
		}

		for _, re := range recentErrors {
			for j := 0; j < re.count; j++ {
				timestamp := now.Add(-time.Duration(re.hoursAgo)*time.Hour - time.Duration(rand.Intn(30))*time.Minute)
				traceID := fmt.Sprintf("err-%d-%04d", timestamp.Unix(), rand.Intn(10000))
				duration := 50 + rand.Intn(500)

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
						IP:     fmt.Sprintf("10.0.%d.%d", rand.Intn(256), rand.Intn(256)),
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
					log.Printf("Failed to create error event: %v", err)
					continue
				}
				totalEvents++
				errorCount++
			}
		}

		fmt.Printf("  HTTP requests: %d, Jobs: %d\n", httpCount, jobCount)
		fmt.Printf("  Events: %d (%d errors)\n", totalEvents, errorCount)
		fmt.Println()
	}

	// Configure Mailpit for local email testing
	mailpitSettings := &settings.Settings{
		SMTPHost:              "localhost",
		SMTPPort:              1025,
		SMTPFrom:              "alerts@lognorth.local",
		NotifyEmail:           "dev@localhost",
		NotifyNewErrors:   true,
		NotifyRegressions: true,
		NotifySpikes:      true,
		SpikeThreshold:    10,
		BaseURL:               "http://localhost:8080",
	}
	if err := settings.Update(db, mailpitSettings); err != nil {
		log.Printf("Failed to configure Mailpit settings: %v", err)
	} else {
		fmt.Println("Email configured for Mailpit (localhost:1025)")
		fmt.Println("  Run: docker compose -f docker-compose.dev.yml up -d")
		fmt.Println("  View: http://localhost:8025")
	}

	fmt.Println()
	fmt.Println("Done. Run 'make dev' to start LogNorth.")
}
