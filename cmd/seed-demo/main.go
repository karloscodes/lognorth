// Seed command to populate LogNorth demo with realistic Rails app data
package main

import (
	"fmt"
	"log"
	"log/slog"
	"math/rand"
	"os"
	"time"

	"gorm.io/gorm"

	"lognorth/internal/apikeys"
	"lognorth/internal/config"
	"lognorth/internal/database"
	"lognorth/internal/events"
	"lognorth/internal/users"
)

// Realistic Rails SaaS endpoints
var endpoints = []endpoint{
	// Auth - high volume, low errors
	{method: "POST", path: "/sessions", name: "sign_in", weight: 15, errorRate: 0.01},
	{method: "DELETE", path: "/sessions", name: "sign_out", weight: 8, errorRate: 0.001},
	{method: "POST", path: "/registrations", name: "sign_up", weight: 5, errorRate: 0.025}, // duplicate emails
	{method: "POST", path: "/passwords", name: "forgot_password", weight: 2, errorRate: 0.005},

	// Core CRUD - the meat of the app
	{method: "GET", path: "/projects", name: "list_projects", weight: 25, errorRate: 0.002},
	{method: "POST", path: "/projects", name: "create_project", weight: 8, errorRate: 0.008},
	{method: "GET", path: "/projects/:id", name: "show_project", weight: 20, errorRate: 0.003},
	{method: "PATCH", path: "/projects/:id", name: "update_project", weight: 6, errorRate: 0.005},
	{method: "DELETE", path: "/projects/:id", name: "delete_project", weight: 2, errorRate: 0.002},

	// Todos within projects
	{method: "GET", path: "/projects/:id/todos", name: "list_todos", weight: 18, errorRate: 0.002},
	{method: "POST", path: "/projects/:id/todos", name: "create_todo", weight: 12, errorRate: 0.005},
	{method: "PATCH", path: "/todos/:id", name: "update_todo", weight: 15, errorRate: 0.003},
	{method: "DELETE", path: "/todos/:id", name: "delete_todo", weight: 4, errorRate: 0.002},
	{method: "POST", path: "/todos/:id/complete", name: "complete_todo", weight: 10, errorRate: 0.002},

	// Comments
	{method: "POST", path: "/todos/:id/comments", name: "create_comment", weight: 8, errorRate: 0.003},
	{method: "DELETE", path: "/comments/:id", name: "delete_comment", weight: 2, errorRate: 0.002},

	// Dashboard and search
	{method: "GET", path: "/dashboard", name: "dashboard", weight: 20, errorRate: 0.005},
	{method: "GET", path: "/search", name: "search", weight: 10, errorRate: 0.008},

	// Payments - higher error rate (card declines are normal)
	{method: "POST", path: "/checkout", name: "checkout", weight: 3, errorRate: 0.04},
	{method: "POST", path: "/subscriptions", name: "create_subscription", weight: 2, errorRate: 0.03},
	{method: "DELETE", path: "/subscriptions/:id", name: "cancel_subscription", weight: 1, errorRate: 0.01},

	// Webhooks - occasional external failures
	{method: "POST", path: "/webhooks/stripe", name: "stripe_webhook", weight: 4, errorRate: 0.015},

	// Settings
	{method: "GET", path: "/settings", name: "show_settings", weight: 5, errorRate: 0.001},
	{method: "PATCH", path: "/settings", name: "update_settings", weight: 3, errorRate: 0.005},
	{method: "GET", path: "/settings/billing", name: "billing_settings", weight: 3, errorRate: 0.002},

	// Team
	{method: "POST", path: "/invitations", name: "invite_member", weight: 2, errorRate: 0.01},
	{method: "GET", path: "/team", name: "list_team", weight: 4, errorRate: 0.001},
}

type endpoint struct {
	method    string
	path      string
	name      string
	weight    int // relative frequency
	errorRate float64
}

// Jobs with realistic child events
var jobs = []jobDef{
	{
		name:      "SendWelcomeEmail",
		weight:    5,
		errorRate: 0.02,
		duration:  [2]int{800, 2000},
		children: []string{
			"Loading user profile",
			"Rendering welcome template",
			"Sending via SendGrid",
		},
	},
	{
		name:      "ProcessPayment",
		weight:    3,
		errorRate: 0.05,
		duration:  [2]int{1500, 4000},
		children: []string{
			"Validating payment method",
			"Charging card via Stripe",
			"Creating invoice record",
			"Sending receipt email",
		},
	},
	{
		name:      "SendInvitation",
		weight:    2,
		errorRate: 0.015,
		duration:  [2]int{600, 1500},
		children: []string{
			"Generating invitation token",
			"Rendering invitation email",
			"Sending via SendGrid",
		},
	},
	{
		name:      "GenerateDailyDigest",
		weight:    4,
		errorRate: 0.03,
		duration:  [2]int{3000, 8000},
		children: []string{
			"Fetching active projects",
			"Aggregating activity for 127 users",
			"Rendering digest templates",
			"Queuing 89 emails for delivery",
			"Digest generation complete",
		},
	},
	{
		name:      "SyncSubscription",
		weight:    3,
		errorRate: 0.04,
		duration:  [2]int{1000, 3000},
		children: []string{
			"Fetching subscription from Stripe",
			"Updating local subscription record",
			"Syncing usage metrics",
		},
	},
	{
		name:      "CleanupExpiredSessions",
		weight:    2,
		errorRate: 0.01,
		duration:  [2]int{500, 2000},
		children: []string{
			"Scanning sessions older than 30 days",
			"Deleting 342 expired sessions",
			"Freed 12MB of storage",
		},
	},
	{
		name:      "ProcessWebhook",
		weight:    4,
		errorRate: 0.025,
		duration:  [2]int{200, 800},
		children: []string{
			"Verifying webhook signature",
			"Processing event: invoice.paid",
			"Updating subscription status",
		},
	},
	{
		name:      "GenerateInvoice",
		weight:    2,
		errorRate: 0.02,
		duration:  [2]int{2000, 5000},
		children: []string{
			"Calculating usage for billing period",
			"Generating invoice PDF",
			"Uploading to S3",
			"Sending invoice email",
		},
	},
}

type jobDef struct {
	name      string
	weight    int
	errorRate float64
	duration  [2]int // min, max ms
	children  []string
}

// Structured errors with realistic Rails stack traces
var structuredErrors = []errorDef{
	// Database errors
	{
		class:   "ActiveRecord::RecordNotFound",
		message: "Couldn't find Project with 'id'=42",
		file:    "app/controllers/projects_controller.rb",
		line:    23,
		caller:  "show",
		stack: `/app/app/controllers/projects_controller.rb:23:in 'block in show'
/usr/local/bundle/gems/activerecord-7.1.3/lib/active_record/relation/finder_methods.rb:234:in 'find'
/app/app/controllers/projects_controller.rb:22:in 'show'
/usr/local/bundle/gems/actionpack-7.1.3/lib/action_controller/metal/basic_implicit_render.rb:6:in 'send_action'
/usr/local/bundle/gems/actionpack-7.1.3/lib/action_controller/metal/implicit_render.rb:4:in 'send_action'
/usr/local/bundle/gems/actionpack-7.1.3/lib/abstract_controller/base.rb:226:in 'process_action'
/usr/local/bundle/gems/actionpack-7.1.3/lib/action_controller/metal/rendering.rb:165:in 'process_action'
/usr/local/bundle/gems/actionpack-7.1.3/lib/abstract_controller/callbacks.rb:234:in 'block in process_action'
/usr/local/bundle/gems/activesupport-7.1.3/lib/active_support/callbacks.rb:118:in 'block in run_callbacks'`,
	},
	{
		class:   "ActiveRecord::RecordNotUnique",
		message: "PG::UniqueViolation: duplicate key value violates unique constraint \"users_email_key\"",
		file:    "app/models/user.rb",
		line:    12,
		caller:  "create!",
		stack: `/app/app/models/user.rb:12:in 'create!'
/app/app/controllers/registrations_controller.rb:18:in 'create'
/usr/local/bundle/gems/actionpack-7.1.3/lib/action_controller/metal/basic_implicit_render.rb:6:in 'send_action'
/usr/local/bundle/gems/actionpack-7.1.3/lib/action_controller/metal/implicit_render.rb:4:in 'send_action'
/usr/local/bundle/gems/actionpack-7.1.3/lib/abstract_controller/base.rb:226:in 'process_action'
/usr/local/bundle/gems/activerecord-7.1.3/lib/active_record/connection_adapters/postgresql_adapter.rb:768:in 'exec_params'`,
	},
	{
		class:   "ActiveRecord::ConnectionTimeoutError",
		message: "could not obtain a connection from the pool within 5.000 seconds (waited 5.001 seconds); all pooled connections were in use",
		file:    "lib/active_record/connection_adapters/abstract/connection_pool.rb",
		line:    293,
		caller:  "checkout",
		stack: `/usr/local/bundle/gems/activerecord-7.1.3/lib/active_record/connection_adapters/abstract/connection_pool.rb:293:in 'checkout'
/usr/local/bundle/gems/activerecord-7.1.3/lib/active_record/connection_adapters/abstract/connection_pool.rb:112:in 'connection'
/usr/local/bundle/gems/activerecord-7.1.3/lib/active_record/connection_handling.rb:265:in 'retrieve_connection'
/app/app/models/project.rb:45:in 'find_with_todos'
/app/app/controllers/projects_controller.rb:8:in 'show'`,
	},

	// Redis errors
	{
		class:   "Redis::CannotConnectError",
		message: "Error connecting to Redis on redis:6379 (Errno::ECONNREFUSED)",
		file:    "app/services/session_store.rb",
		line:    18,
		caller:  "get",
		stack: `/app/app/services/session_store.rb:18:in 'get'
/app/app/controllers/application_controller.rb:34:in 'current_user'
/app/app/controllers/application_controller.rb:28:in 'authenticate_user!'
/usr/local/bundle/gems/activesupport-7.1.3/lib/active_support/callbacks.rb:118:in 'block in run_callbacks'
/usr/local/bundle/gems/actionpack-7.1.3/lib/abstract_controller/callbacks.rb:234:in 'block in process_action'
/usr/local/bundle/gems/redis-5.0.8/lib/redis/client.rb:168:in 'connect'`,
	},
	{
		class:   "Redis::TimeoutError",
		message: "Connection timed out",
		file:    "app/services/cache_service.rb",
		line:    42,
		caller:  "fetch",
		stack: `/app/app/services/cache_service.rb:42:in 'fetch'
/app/app/controllers/dashboard_controller.rb:12:in 'index'
/usr/local/bundle/gems/redis-5.0.8/lib/redis/client.rb:134:in 'call_v'
/usr/local/bundle/gems/redis-5.0.8/lib/redis.rb:156:in 'get'
/usr/local/bundle/gems/actionpack-7.1.3/lib/action_controller/metal/basic_implicit_render.rb:6:in 'send_action'`,
	},

	// Payment errors
	{
		class:   "Stripe::CardError",
		message: "Your card was declined. Your request was in live mode, but used a known test card.",
		file:    "app/services/payment_service.rb",
		line:    67,
		caller:  "charge!",
		stack: `/app/app/services/payment_service.rb:67:in 'charge!'
/app/app/controllers/checkout_controller.rb:23:in 'create'
/usr/local/bundle/gems/stripe-10.1.0/lib/stripe/api_operations/request.rb:43:in 'execute_resource_request'
/usr/local/bundle/gems/stripe-10.1.0/lib/stripe/api_resource.rb:76:in 'request_stripe_object'
/usr/local/bundle/gems/stripe-10.1.0/lib/stripe/resources/payment_intent.rb:38:in 'create'
/usr/local/bundle/gems/actionpack-7.1.3/lib/action_controller/metal/basic_implicit_render.rb:6:in 'send_action'`,
	},
	{
		class:   "Stripe::InvalidRequestError",
		message: "No such customer: 'cus_OxR4pVmVc8DKxY'; a]similar object exists in live mode, but a test mode key was used to make this request.",
		file:    "app/services/subscription_service.rb",
		line:    34,
		caller:  "sync!",
		stack: `/app/app/services/subscription_service.rb:34:in 'sync!'
/app/app/jobs/sync_subscription_job.rb:12:in 'perform'
/usr/local/bundle/gems/stripe-10.1.0/lib/stripe/api_operations/request.rb:43:in 'execute_resource_request'
/usr/local/bundle/gems/activejob-7.1.3/lib/active_job/execution.rb:49:in 'block in perform_now'
/usr/local/bundle/gems/sidekiq-7.2.0/lib/sidekiq/job_retry.rb:97:in 'local'`,
	},
	{
		class:   "Stripe::APIConnectionError",
		message: "Could not connect to Stripe (api.stripe.com). Please check your internet connection and try again.",
		file:    "app/services/payment_service.rb",
		line:    45,
		caller:  "create_payment_intent",
		stack: `/app/app/services/payment_service.rb:45:in 'create_payment_intent'
/app/app/controllers/checkout_controller.rb:18:in 'create'
/usr/local/bundle/gems/stripe-10.1.0/lib/stripe/connection_manager.rb:134:in 'request'
/usr/local/bundle/gems/stripe-10.1.0/lib/stripe/stripe_client.rb:543:in 'execute_request_with_rescues'
/usr/local/bundle/gems/faraday-2.7.12/lib/faraday/connection.rb:456:in 'run_request'
/usr/local/bundle/gems/net-http-0.4.0/lib/net/http.rb:1512:in 'request'`,
	},

	// Auth errors
	{
		class:   "JWT::DecodeError",
		message: "Signature verification failed",
		file:    "app/services/auth_service.rb",
		line:    23,
		caller:  "decode_token",
		stack: `/app/app/services/auth_service.rb:23:in 'decode_token'
/app/app/controllers/api/base_controller.rb:15:in 'authenticate_api_user!'
/usr/local/bundle/gems/jwt-2.7.1/lib/jwt/decode.rb:45:in 'verify_signature'
/usr/local/bundle/gems/jwt-2.7.1/lib/jwt/decode.rb:27:in 'decode_segments'
/usr/local/bundle/gems/activesupport-7.1.3/lib/active_support/callbacks.rb:118:in 'block in run_callbacks'`,
	},
	{
		class:   "ActionController::InvalidAuthenticityToken",
		message: "Can't verify CSRF token authenticity.",
		file:    "actionpack/lib/action_controller/metal/request_forgery_protection.rb",
		line:    245,
		caller:  "handle_unverified_request",
		stack: `/usr/local/bundle/gems/actionpack-7.1.3/lib/action_controller/metal/request_forgery_protection.rb:245:in 'handle_unverified_request'
/usr/local/bundle/gems/actionpack-7.1.3/lib/action_controller/metal/request_forgery_protection.rb:287:in 'verify_authenticity_token'
/usr/local/bundle/gems/activesupport-7.1.3/lib/active_support/callbacks.rb:118:in 'block in run_callbacks'
/usr/local/bundle/gems/actionpack-7.1.3/lib/abstract_controller/callbacks.rb:234:in 'block in process_action'`,
	},

	// External service errors
	{
		class:   "Net::OpenTimeout",
		message: "execution expired",
		file:    "app/services/sendgrid_service.rb",
		line:    28,
		caller:  "deliver!",
		stack: `/app/app/services/sendgrid_service.rb:28:in 'deliver!'
/app/app/jobs/send_email_job.rb:15:in 'perform'
/usr/local/bundle/gems/net-http-0.4.0/lib/net/http.rb:1001:in 'connect'
/usr/local/bundle/gems/net-http-0.4.0/lib/net/http.rb:983:in 'do_start'
/usr/local/bundle/gems/activejob-7.1.3/lib/active_job/execution.rb:49:in 'block in perform_now'
/usr/local/bundle/gems/sidekiq-7.2.0/lib/sidekiq/job_retry.rb:97:in 'local'`,
	},
	{
		class:   "Faraday::ConnectionFailed",
		message: "Connection refused - connect(2) for \"api.sendgrid.com\" port 443 (api.sendgrid.com:443)",
		file:    "app/services/email_service.rb",
		line:    34,
		caller:  "send_email",
		stack: `/app/app/services/email_service.rb:34:in 'send_email'
/app/app/jobs/send_welcome_email_job.rb:8:in 'perform'
/usr/local/bundle/gems/faraday-2.7.12/lib/faraday/adapter/net_http.rb:118:in 'request_via_request_method'
/usr/local/bundle/gems/faraday-2.7.12/lib/faraday/adapter.rb:52:in 'connection'
/usr/local/bundle/gems/activejob-7.1.3/lib/active_job/execution.rb:49:in 'block in perform_now'`,
	},

	// Validation errors
	{
		class:   "ActiveRecord::RecordInvalid",
		message: "Validation failed: Email has already been taken",
		file:    "app/controllers/registrations_controller.rb",
		line:    15,
		caller:  "create",
		stack: `/app/app/controllers/registrations_controller.rb:15:in 'create'
/usr/local/bundle/gems/activerecord-7.1.3/lib/active_record/validations.rb:80:in 'raise_validation_error'
/usr/local/bundle/gems/activerecord-7.1.3/lib/active_record/validations.rb:53:in 'save!'
/usr/local/bundle/gems/activerecord-7.1.3/lib/active_record/transactions.rb:313:in 'block in save!'
/usr/local/bundle/gems/activerecord-7.1.3/lib/active_record/transactions.rb:365:in 'with_transaction_returning_status'`,
	},
	{
		class:   "ActiveRecord::RecordInvalid",
		message: "Validation failed: Name can't be blank, Name is too short (minimum is 3 characters)",
		file:    "app/controllers/projects_controller.rb",
		line:    34,
		caller:  "create",
		stack: `/app/app/controllers/projects_controller.rb:34:in 'create'
/usr/local/bundle/gems/activerecord-7.1.3/lib/active_record/validations.rb:80:in 'raise_validation_error'
/usr/local/bundle/gems/activerecord-7.1.3/lib/active_record/validations.rb:53:in 'save!'
/usr/local/bundle/gems/activerecord-7.1.3/lib/active_record/persistence.rb:579:in 'create!'
/usr/local/bundle/gems/actionpack-7.1.3/lib/action_controller/metal/basic_implicit_render.rb:6:in 'send_action'`,
	},

	// Job errors
	{
		class:   "Sidekiq::Shutdown",
		message: "Sidekiq is shutting down",
		file:    "app/jobs/generate_digest_job.rb",
		line:    45,
		caller:  "perform",
		stack: `/app/app/jobs/generate_digest_job.rb:45:in 'perform'
/usr/local/bundle/gems/sidekiq-7.2.0/lib/sidekiq/processor.rb:123:in 'process'
/usr/local/bundle/gems/sidekiq-7.2.0/lib/sidekiq/processor.rb:89:in 'run'
/usr/local/bundle/gems/activejob-7.1.3/lib/active_job/execution.rb:49:in 'block in perform_now'
/usr/local/bundle/gems/activejob-7.1.3/lib/active_job/callbacks.rb:99:in 'block in run_callbacks'`,
	},
	{
		class:   "ActiveJob::DeserializationError",
		message: "Error while trying to deserialize arguments: Couldn't find User with 'id'=999",
		file:    "app/jobs/send_welcome_email_job.rb",
		line:    8,
		caller:  "perform",
		stack: `/app/app/jobs/send_welcome_email_job.rb:8:in 'perform'
/usr/local/bundle/gems/activejob-7.1.3/lib/active_job/arguments.rb:82:in 'deserialize'
/usr/local/bundle/gems/activejob-7.1.3/lib/active_job/arguments.rb:45:in 'deserialize'
/usr/local/bundle/gems/activejob-7.1.3/lib/active_job/core.rb:131:in 'deserialize_arguments'
/usr/local/bundle/gems/sidekiq-7.2.0/lib/sidekiq/job_retry.rb:97:in 'local'`,
	},
}

type errorDef struct {
	class   string
	message string
	file    string
	line    int
	caller  string
	stack   string
}

// Error mapping for endpoints
var endpointErrors = map[string][]int{
	"sign_in":             {8, 9},    // JWT, CSRF
	"sign_up":             {1, 12},   // duplicate email, validation
	"show_project":        {0},       // not found
	"update_project":      {0, 13},   // not found, validation
	"delete_project":      {0},       // not found
	"checkout":            {5, 6, 7}, // Stripe errors
	"create_subscription": {5, 6, 7}, // Stripe errors
	"stripe_webhook":      {7, 10},   // Stripe API, timeout
	"dashboard":           {2, 3, 4}, // DB pool, Redis
	"search":              {2, 4},    // DB pool, Redis timeout
	"invite_member":       {10, 11},  // SendGrid errors
}

// Error mapping for jobs
var jobErrors = map[string][]int{
	"SendWelcomeEmail":       {10, 11, 15}, // SendGrid, deserialization
	"ProcessPayment":         {5, 6, 7},    // Stripe errors
	"SendInvitation":         {10, 11},     // SendGrid errors
	"GenerateDailyDigest":    {2, 14},      // DB pool, Sidekiq shutdown
	"SyncSubscription":       {6, 7},       // Stripe errors
	"CleanupExpiredSessions": {2},          // DB pool
	"ProcessWebhook":         {6, 7},       // Stripe errors
	"GenerateInvoice":        {6, 10},      // Stripe, timeout
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
			fmt.Println("Created demo user (demo@lognorth.com / demo)")
		}
	}

	// Check if already seeded
	existingKeys, _ := apikeys.List(db)
	if len(existingKeys) > 0 {
		fmt.Println("Already seeded. Delete the database to re-seed.")
		return
	}

	now := time.Now()

	fmt.Println("Seeding realistic Rails app data...")
	fmt.Println()

	// One app - like a real Rails app with Sidekiq workers
	appName := "rails-demo"
	apiKey, _, err := apikeys.Create(db, appName)
	if err != nil {
		log.Fatalf("Failed to create API key: %v", err)
	}
	fmt.Printf("  Created app: %s\n", appName)
	fmt.Println()

	// Build weighted endpoint list
	var weightedEndpoints []endpoint
	for _, ep := range endpoints {
		for i := 0; i < ep.weight; i++ {
			weightedEndpoints = append(weightedEndpoints, ep)
		}
	}

	// Build weighted job list
	var weightedJobs []jobDef
	for _, j := range jobs {
		for i := 0; i < j.weight; i++ {
			weightedJobs = append(weightedJobs, j)
		}
	}

	fmt.Println("  Generating HTTP events...")
	httpCount := 0
	httpErrors := 0
	httpTarget := 52000

	// Define an incident window: Redis went down 3 days ago for 2 hours
	incidentStart := now.Add(-72 * time.Hour)
	incidentEnd := incidentStart.Add(2 * time.Hour)

	// Generate 60 days of data
	daysBack := 60

	for i := 0; i < httpTarget; i++ {
		// Timestamp distribution: more recent = more likely
		// Use exponential decay - most events in last 7 days
		dayOffset := int(float64(daysBack) * (1 - rand.Float64()*rand.Float64()))
		hourOffset := rand.Intn(24)
		minuteOffset := rand.Intn(60)
		timestamp := now.Add(-time.Duration(dayOffset)*24*time.Hour -
			time.Duration(hourOffset)*time.Hour -
			time.Duration(minuteOffset)*time.Minute)

		// Traffic patterns
		hour := timestamp.Hour()
		weekday := timestamp.Weekday()
		skipChance := 0.0

		// Night traffic is sparse (2am-6am)
		if hour >= 2 && hour < 6 {
			skipChance = 0.85
		}
		// Weekend traffic is lighter
		if weekday == time.Saturday || weekday == time.Sunday {
			skipChance = skipChance + 0.4
			if skipChance > 0.9 {
				skipChance = 0.9
			}
		}

		if rand.Float64() < skipChance {
			continue
		}

		ep := weightedEndpoints[rand.Intn(len(weightedEndpoints))]
		traceID := fmt.Sprintf("req_%x", rand.Int63())

		// Duration: mostly fast, some slow
		duration := 10 + rand.Intn(80)
		if rand.Float64() < 0.08 {
			duration = 150 + rand.Intn(500) // slower requests
		}
		if rand.Float64() < 0.02 {
			duration = 500 + rand.Intn(2000) // very slow
		}

		// Error rate - check for incident window
		errorRate := ep.errorRate
		isInIncident := timestamp.After(incidentStart) && timestamp.Before(incidentEnd)
		if isInIncident && (ep.name == "sign_in" || ep.name == "dashboard" || ep.name == "show_project") {
			errorRate = 0.35 // Redis down = session failures
		}

		isError := rand.Float64() < errorRate

		var status int
		if isError {
			status = []int{500, 500, 500, 502, 503}[rand.Intn(5)]
			duration = duration * 3
			httpErrors++
		} else if rand.Float64() < 0.03 {
			status = []int{400, 401, 403, 404, 422}[rand.Intn(5)]
		} else {
			if ep.method == "POST" {
				status = []int{200, 201, 201}[rand.Intn(3)]
			} else if ep.method == "DELETE" {
				status = 204
			} else {
				status = 200
			}
		}

		message := fmt.Sprintf("%s %s → %d", ep.method, ep.path, status)

		ctx := &events.EventContext{
			Method: ep.method,
			Path:   ep.path,
			Status: status,
			IP:     randomIP(),
		}

		if rand.Float64() < 0.6 {
			ctx.User = fmt.Sprintf("user_%d", rand.Intn(200))
		}

		if isError {
			errDef := pickEndpointError(ep.name, isInIncident)
			ctx.Error = errDef.message
			ctx.ErrorClass = errDef.class
			ctx.ErrorFile = errDef.file
			ctx.ErrorLine = errDef.line
			ctx.ErrorCaller = errDef.caller
			ctx.StackTrace = errDef.stack
		}

		// Create main request event FIRST
		event := &events.Event{
			APIKeyID:   apiKey.ID,
			Timestamp:  timestamp,
			Message:    message,
			TraceID:    traceID,
			DurationMS: duration,
			IsError:    isError,
		}
		event.SetContext(ctx)
		if isError {
			events.PopulateErrorFields(event, ctx)
		}

		if err := events.Create(db, event); err != nil {
			continue
		}
		httpCount++

		// Add trace children AFTER the request (DB queries, cache, etc.)
		if rand.Float64() < 0.3 {
			childCount := 1 + rand.Intn(3)
			for c := 0; c < childCount; c++ {
				// Children happen AFTER parent starts (offset 10-50% of duration)
				childOffset := time.Duration(10+rand.Intn(duration/2)) * time.Millisecond
				childMsg := pickInternalOperation()
				childDuration := 1 + rand.Intn(30)
				createEvent(db, apiKey.ID, traceID, timestamp.Add(childOffset), childMsg, &events.EventContext{}, childDuration, false)
			}
		}

		// Some requests trigger background jobs
		if !isError && status < 300 {
			if (ep.name == "sign_up" && rand.Float64() < 0.9) ||
				(ep.name == "checkout" && rand.Float64() < 0.8) ||
				(ep.name == "invite_member" && rand.Float64() < 0.85) {

				jobName := ""
				switch ep.name {
				case "sign_up":
					jobName = "SendWelcomeEmail"
				case "checkout":
					jobName = "ProcessPayment"
				case "invite_member":
					jobName = "SendInvitation"
				}

				if jobName != "" {
					createLinkedJob(db, apiKey.ID, traceID, timestamp.Add(100*time.Millisecond), jobName, isInIncident)
				}
			}
		}

		if httpCount%10000 == 0 {
			fmt.Printf("    %d HTTP events...\n", httpCount)
		}
	}

	fmt.Printf("  Generated %d HTTP events (%d errors)\n", httpCount, httpErrors)
	fmt.Println()

	// Generate standalone job events (periodic jobs not triggered by HTTP)
	fmt.Println("  Generating background job events...")
	jobCount := 0
	jobErrorCount := 0
	jobTarget := 12000

	for i := 0; i < jobTarget; i++ {
		dayOffset := int(float64(daysBack) * (1 - rand.Float64()*rand.Float64()))
		hourOffset := rand.Intn(24)
		minuteOffset := rand.Intn(60)
		timestamp := now.Add(-time.Duration(dayOffset)*24*time.Hour -
			time.Duration(hourOffset)*time.Hour -
			time.Duration(minuteOffset)*time.Minute)

		job := weightedJobs[rand.Intn(len(weightedJobs))]
		traceID := fmt.Sprintf("job_%x", rand.Int63())

		isInIncident := timestamp.After(incidentStart) && timestamp.Before(incidentEnd)
		errorRate := job.errorRate
		if isInIncident && (job.name == "GenerateDailyDigest" || job.name == "CleanupExpiredSessions") {
			errorRate = 0.4 // DB pool exhausted during incident
		}

		isError := rand.Float64() < errorRate
		duration := job.duration[0] + rand.Intn(job.duration[1]-job.duration[0])

		if isError {
			duration = duration * 2
			jobErrorCount++
		}

		// Create child events
		for c, childMsg := range job.children {
			childOffset := time.Duration(c*duration/len(job.children)) * time.Millisecond
			childDuration := 50 + rand.Intn(300)

			// Last child might be the error
			childIsError := isError && c == len(job.children)-1

			childCtx := &events.EventContext{Job: job.name}
			if childIsError {
				errDef := pickJobError(job.name)
				childCtx.Error = errDef.message
				childCtx.ErrorClass = errDef.class
				childCtx.ErrorFile = errDef.file
				childCtx.ErrorLine = errDef.line
				childCtx.ErrorCaller = errDef.caller
				childCtx.StackTrace = errDef.stack
			}

			createEvent(db, apiKey.ID, traceID, timestamp.Add(childOffset), childMsg, childCtx, childDuration, childIsError)
		}

		// Create main job event
		var message string
		if isError {
			message = fmt.Sprintf("%s failed", job.name)
		} else {
			message = fmt.Sprintf("%s completed", job.name)
		}

		ctx := &events.EventContext{Job: job.name}
		if isError {
			errDef := pickJobError(job.name)
			ctx.Error = errDef.message
			ctx.ErrorClass = errDef.class
			ctx.ErrorFile = errDef.file
			ctx.ErrorLine = errDef.line
			ctx.ErrorCaller = errDef.caller
			ctx.StackTrace = errDef.stack
		}

		event := &events.Event{
			APIKeyID:   apiKey.ID,
			Timestamp:  timestamp.Add(time.Duration(duration) * time.Millisecond),
			Message:    message,
			TraceID:    traceID,
			DurationMS: duration,
			IsError:    isError,
		}
		event.SetContext(ctx)
		if isError {
			events.PopulateErrorFields(event, ctx)
		}

		if err := events.Create(db, event); err != nil {
			continue
		}
		jobCount++

		if jobCount%5000 == 0 {
			fmt.Printf("    %d job events...\n", jobCount)
		}
	}

	fmt.Printf("  Generated %d job events (%d errors)\n", jobCount, jobErrorCount)
	fmt.Println()

	// Generate rich traces in the last 10 minutes (visible at top of logs)
	fmt.Println("  Generating recent rich traces...")
	recentTraceCount := 0

	recentTraces := []struct {
		endpoint   string
		method     string
		path       string
		status     int
		duration   int
		children   []string
		triggerJob string
	}{
		{
			endpoint: "checkout",
			method:   "POST",
			path:     "/checkout",
			status:   201,
			duration: 850,
			children: []string{
				"Loading user profile",
				"Fetching cart items",
				"Validating session",
				"Creating payment intent",
				"Recording order",
			},
			triggerJob: "ProcessPayment",
		},
		{
			endpoint: "sign_up",
			method:   "POST",
			path:     "/registrations",
			status:   201,
			duration: 320,
			children: []string{
				"Checking email availability",
				"Creating user account",
				"Initializing session",
			},
			triggerJob: "SendWelcomeEmail",
		},
		{
			endpoint: "dashboard",
			method:   "GET",
			path:     "/dashboard",
			status:   200,
			duration: 180,
			children: []string{
				"Loading cached dashboard",
				"Fetching recent projects",
				"Counting pending todos",
			},
		},
		{
			endpoint: "project_show",
			method:   "GET",
			path:     "/projects/:id",
			status:   200,
			duration: 95,
			children: []string{
				"Loading project",
				"Fetching todos",
				"Loading project stats",
			},
		},
		{
			endpoint: "create_todo",
			method:   "POST",
			path:     "/projects/:id/todos",
			status:   201,
			duration: 120,
			children: []string{
				"Checking project access",
				"Creating todo",
				"Invalidating cache",
			},
		},
	}

	for i := 0; i < 15; i++ {
		trace := recentTraces[i%len(recentTraces)]
		minutesAgo := i / 2 // spread over ~8 minutes
		secondsAgo := (i % 2) * 30
		timestamp := now.Add(-time.Duration(minutesAgo)*time.Minute - time.Duration(secondsAgo)*time.Second)
		traceID := fmt.Sprintf("recent_%x", rand.Int63())
		user := fmt.Sprintf("user_%d", rand.Intn(200))

		// Main HTTP request FIRST (when the request starts)
		message := fmt.Sprintf("%s %s → %d", trace.method, trace.path, trace.status)
		ctx := &events.EventContext{
			Method: trace.method,
			Path:   trace.path,
			Status: trace.status,
			IP:     randomIP(),
			User:   user,
		}
		event := &events.Event{
			APIKeyID:   apiKey.ID,
			Timestamp:  timestamp,
			Message:    message,
			TraceID:    traceID,
			DurationMS: trace.duration,
			IsError:    false,
		}
		event.SetContext(ctx)
		events.Create(db, event)
		recentTraceCount++

		// Child events happen AFTER the request starts
		for c, childMsg := range trace.children {
			childOffset := time.Duration((c+1)*trace.duration/(len(trace.children)+1)) * time.Millisecond
			childDuration := 5 + rand.Intn(50)
			createEvent(db, apiKey.ID, traceID, timestamp.Add(childOffset), childMsg, &events.EventContext{}, childDuration, false)
			recentTraceCount++
		}

		// Trigger job if applicable (after request completes)
		if trace.triggerJob != "" {
			createLinkedJob(db, apiKey.ID, traceID, timestamp.Add(time.Duration(trace.duration+100)*time.Millisecond), trace.triggerJob, false)
			recentTraceCount++
		}
	}

	fmt.Printf("  Generated %d recent trace events\n", recentTraceCount)
	fmt.Println()

	// Generate a recent error burst (makes Issues page interesting)
	fmt.Println("  Generating recent error burst...")
	burstCount := 0

	// Stripe API issues in the last hour
	for i := 0; i < 45; i++ {
		minutesAgo := rand.Intn(60)
		timestamp := now.Add(-time.Duration(minutesAgo) * time.Minute)
		traceID := fmt.Sprintf("burst_%x", rand.Int63())

		errDef := structuredErrors[7] // Stripe::APIConnectionError

		ctx := &events.EventContext{
			Method:      "POST",
			Path:        "/checkout",
			Status:      500,
			IP:          randomIP(),
			Error:       errDef.message,
			ErrorClass:  errDef.class,
			ErrorFile:   errDef.file,
			ErrorLine:   errDef.line,
			ErrorCaller: errDef.caller,
			StackTrace:  errDef.stack,
		}
		if rand.Float64() < 0.7 {
			ctx.User = fmt.Sprintf("user_%d", rand.Intn(200))
		}

		event := &events.Event{
			APIKeyID:   apiKey.ID,
			Timestamp:  timestamp,
			Message:    "POST /checkout → 500",
			TraceID:    traceID,
			DurationMS: 5000 + rand.Intn(25000), // Long timeouts
			IsError:    true,
		}
		event.SetContext(ctx)
		events.PopulateErrorFields(event, ctx)

		if err := events.Create(db, event); err != nil {
			continue
		}
		burstCount++
	}

	fmt.Printf("  Generated %d burst errors\n", burstCount)
	fmt.Println()
	fmt.Println("Done. Run with LOGNORTH_DEMO=true to start in demo mode.")
}

func createEvent(db *gorm.DB, apiKeyID uint, traceID string, timestamp time.Time, message string, ctx *events.EventContext, durationMS int, isError bool) {
	event := &events.Event{
		APIKeyID:   apiKeyID,
		Timestamp:  timestamp,
		Message:    message,
		TraceID:    traceID,
		DurationMS: durationMS,
		IsError:    isError,
	}
	event.SetContext(ctx)
	if isError {
		events.PopulateErrorFields(event, ctx)
	}
	events.Create(db, event)
}

func createLinkedJob(db *gorm.DB, apiKeyID uint, traceID string, timestamp time.Time, jobName string, isInIncident bool) {
	var job jobDef
	for _, j := range jobs {
		if j.name == jobName {
			job = j
			break
		}
	}

	errorRate := job.errorRate
	if isInIncident {
		errorRate = 0.3
	}
	isError := rand.Float64() < errorRate

	duration := job.duration[0] + rand.Intn(job.duration[1]-job.duration[0])
	if isError {
		duration = duration * 2
	}

	// Create child events
	for c, childMsg := range job.children {
		childOffset := time.Duration(c*duration/len(job.children)) * time.Millisecond
		childDuration := 50 + rand.Intn(300)
		createEvent(db, apiKeyID, traceID, timestamp.Add(childOffset), childMsg, &events.EventContext{Job: jobName}, childDuration, false)
	}

	var message string
	ctx := &events.EventContext{Job: jobName}

	if isError {
		message = fmt.Sprintf("%s failed", jobName)
		errDef := pickJobError(jobName)
		ctx.Error = errDef.message
		ctx.ErrorClass = errDef.class
		ctx.ErrorFile = errDef.file
		ctx.ErrorLine = errDef.line
		ctx.ErrorCaller = errDef.caller
		ctx.StackTrace = errDef.stack
	} else {
		message = fmt.Sprintf("%s completed", jobName)
	}

	event := &events.Event{
		APIKeyID:   apiKeyID,
		Timestamp:  timestamp.Add(time.Duration(duration) * time.Millisecond),
		Message:    message,
		TraceID:    traceID,
		DurationMS: duration,
		IsError:    isError,
	}
	event.SetContext(ctx)
	if isError {
		events.PopulateErrorFields(event, ctx)
	}
	events.Create(db, event)
}

func pickEndpointError(endpointName string, isInIncident bool) errorDef {
	// During incident, Redis errors are more likely
	if isInIncident {
		return structuredErrors[3] // Redis::CannotConnectError
	}

	indices, ok := endpointErrors[endpointName]
	if !ok || len(indices) == 0 {
		// Default to a generic error
		return structuredErrors[rand.Intn(len(structuredErrors))]
	}
	return structuredErrors[indices[rand.Intn(len(indices))]]
}

func pickJobError(jobName string) errorDef {
	indices, ok := jobErrors[jobName]
	if !ok || len(indices) == 0 {
		return structuredErrors[rand.Intn(len(structuredErrors))]
	}
	return structuredErrors[indices[rand.Intn(len(indices))]]
}

func pickInternalOperation() string {
	ops := []string{
		"Loading user profile",
		"Checking permissions",
		"Validating session",
		"Fetching project data",
		"Loading todos",
		"Updating cache",
		"Recording audit log",
		"Checking rate limit",
	}
	return ops[rand.Intn(len(ops))]
}

func randomIP() string {
	return fmt.Sprintf("10.%d.%d.%d", rand.Intn(10), rand.Intn(256), rand.Intn(256))
}
