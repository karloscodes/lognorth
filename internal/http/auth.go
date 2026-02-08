package http

import (
	"log/slog"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/karloscodes/cartridge"
	"github.com/karloscodes/cartridge/crypto"
	"github.com/karloscodes/cartridge/flash"

	"lognorth/internal/onboarding"
	"lognorth/internal/users"
)

// LoginPageAction handles GET /login
func LoginPageAction(ctx *cartridge.Context) error {
	db := ctx.DB()

	// Check if onboarding is required first
	required, err := onboarding.IsOnboardingRequired(db)
	if err != nil {
		ctx.Logger.Error("Failed to check if onboarding required on login", slog.Any("error", err))
	} else if required {
		return ctx.Redirect("/setup", fiber.StatusFound)
	}

	// If already authenticated, redirect to home
	if ctx.Session.IsAuthenticated(ctx.Ctx) {
		return ctx.Redirect("/", fiber.StatusFound)
	}

	return RenderPage(ctx.Ctx, "Login", Props{})
}

// LoginAction handles POST /login
func LoginAction(ctx *cartridge.Context) error {
	// Parse login form
	email := ctx.FormValue("email")
	password := ctx.FormValue("password")

	if email == "" || password == "" {
		flash.SetFlash(ctx.Ctx, "error", "Email and password are required")
		return ctx.Redirect("/login", fiber.StatusFound)
	}

	db := ctx.DB()

	// Find user by email
	user, err := users.FindByEmail(db, email)

	// Always verify password to prevent timing attacks
	var passwordValid bool
	if err != nil {
		// User not found - verify against dummy hash to maintain constant time
		ctx.Logger.Debug("User not found during login", slog.String("email", email))
		dummyHash := "$2a$10$N9qo8uLOickgx2ZMRZoMyeIjZAgcfl7p92ldGxad68LJZdL17lhWy"
		crypto.VerifyPassword(dummyHash, password)
		passwordValid = false
	} else {
		passwordValid = crypto.VerifyPassword(user.EncryptedPassword, password)
		if !passwordValid {
			ctx.Logger.Debug("Invalid password attempt", slog.String("email", email))
		}
	}

	if !passwordValid {
		flash.SetFlash(ctx.Ctx, "error", "Invalid email or password")
		return ctx.Redirect("/login", fiber.StatusFound)
	}

	// Set session
	if err := ctx.Session.SetSession(ctx.Ctx, user.ID); err != nil {
		ctx.Logger.Error("Failed to set session", slog.Any("error", err))
		flash.SetFlash(ctx.Ctx, "error", "Login failed")
		return ctx.Redirect("/login", fiber.StatusFound)
	}

	ctx.Logger.Debug("Login successful", slog.String("email", email), slog.Uint64("userId", uint64(user.ID)))

	return ctx.Redirect("/", fiber.StatusFound)
}

// LogoutAction handles POST /logout
func LogoutAction(ctx *cartridge.Context) error {
	ctx.Session.ClearSession(ctx.Ctx)
	flash.SetFlash(ctx.Ctx, "success", "You have been logged out")
	return ctx.Redirect("/login", fiber.StatusFound)
}

// OnboardingPageAction handles GET /setup (onboarding)
func OnboardingPageAction(ctx *cartridge.Context) error {
	db := ctx.DB()

	// Check if onboarding is required
	required, err := onboarding.IsOnboardingRequired(db)
	if err != nil {
		ctx.Logger.Error("Failed to check if onboarding required", slog.Any("error", err))
		return ctx.Ctx.Status(fiber.StatusInternalServerError).SendString("Internal error")
	}

	// If not required, redirect to login
	if !required {
		return ctx.Redirect("/login", fiber.StatusFound)
	}

	// Get or create onboarding session
	sessionID := ctx.Ctx.Cookies("_onboarding_session")
	var session *onboarding.OnboardingSession

	if sessionID != "" {
		session, _ = onboarding.GetSession(db, sessionID)
	}

	if session == nil {
		// Generate new session ID
		sessionID = generateSessionID()
		session, err = onboarding.CreateSession(db, sessionID)
		if err != nil {
			ctx.Logger.Error("Failed to create onboarding session", slog.Any("error", err))
			return ctx.Ctx.Status(fiber.StatusInternalServerError).SendString("Internal error")
		}

		// Set cookie
		ctx.Ctx.Cookie(&fiber.Cookie{
			Name:     "_onboarding_session",
			Value:    sessionID,
			Expires:  time.Now().Add(1 * time.Hour),
			HTTPOnly: true,
			Secure:   ctx.Config.IsProduction(),
			SameSite: "Lax",
		})
	}

	return RenderPage(ctx.Ctx, "Onboarding", Props{
		"step":  session.Step,
		"email": session.Data.Email,
	})
}

// OnboardingUserAction handles POST /setup/user
func OnboardingUserAction(ctx *cartridge.Context) error {
	db := ctx.DB()

	email := ctx.FormValue("email")
	if email == "" {
		flash.SetFlash(ctx.Ctx, "error", "Email is required")
		return ctx.Redirect("/setup", fiber.StatusFound)
	}

	sessionID := ctx.Ctx.Cookies("_onboarding_session")
	if sessionID == "" {
		return ctx.Redirect("/setup", fiber.StatusFound)
	}

	session, err := onboarding.GetSession(db, sessionID)
	if err != nil {
		return ctx.Redirect("/setup", fiber.StatusFound)
	}

	// Update session with email and advance to password step
	session.Data.Email = email
	err = onboarding.UpdateSession(db, sessionID, onboarding.StepPassword, session.Data)
	if err != nil {
		ctx.Logger.Error("Failed to update onboarding session", slog.Any("error", err))
		flash.SetFlash(ctx.Ctx, "error", "Failed to save progress")
		return ctx.Redirect("/setup", fiber.StatusFound)
	}

	return ctx.Redirect("/setup", fiber.StatusFound)
}

// OnboardingPasswordAction handles POST /setup/password
func OnboardingPasswordAction(ctx *cartridge.Context) error {
	db := ctx.DB()

	password := ctx.FormValue("password")
	confirmPassword := ctx.FormValue("confirm_password")

	if password == "" {
		flash.SetFlash(ctx.Ctx, "error", "Password is required")
		return ctx.Redirect("/setup", fiber.StatusFound)
	}

	if len(password) < 8 {
		flash.SetFlash(ctx.Ctx, "error", "Password must be at least 8 characters")
		return ctx.Redirect("/setup", fiber.StatusFound)
	}

	if password != confirmPassword {
		flash.SetFlash(ctx.Ctx, "error", "Passwords do not match")
		return ctx.Redirect("/setup", fiber.StatusFound)
	}

	sessionID := ctx.Ctx.Cookies("_onboarding_session")
	if sessionID == "" {
		return ctx.Redirect("/setup", fiber.StatusFound)
	}

	session, err := onboarding.GetSession(db, sessionID)
	if err != nil {
		return ctx.Redirect("/setup", fiber.StatusFound)
	}

	if session.Data.Email == "" {
		flash.SetFlash(ctx.Ctx, "error", "Please enter your email first")
		return ctx.Redirect("/setup", fiber.StatusFound)
	}

	// Create the user
	err = users.CreateUser(db, session.Data.Email, password)
	if err != nil {
		ctx.Logger.Error("Failed to create user", slog.Any("error", err))
		flash.SetFlash(ctx.Ctx, "error", "Failed to create account")
		return ctx.Redirect("/setup", fiber.StatusFound)
	}

	// Complete onboarding
	onboarding.CompleteSession(db, sessionID)

	// Clear onboarding cookie
	ctx.Ctx.Cookie(&fiber.Cookie{
		Name:     "_onboarding_session",
		Value:    "",
		Expires:  time.Now().Add(-1 * time.Hour),
		HTTPOnly: true,
		Secure:   ctx.Config.IsProduction(),
		SameSite: "Lax",
	})

	// Auto-login the user
	user, _ := users.FindByEmail(db, session.Data.Email)
	if user != nil {
		ctx.Session.SetSession(ctx.Ctx, user.ID)
	}

	flash.SetFlash(ctx.Ctx, "success", "Account created successfully!")
	return ctx.Redirect("/", fiber.StatusFound)
}

// generateSessionID creates a random session ID
func generateSessionID() string {
	return time.Now().Format("20060102150405") + "-" + randomString(16)
}

func randomString(n int) string {
	const letters = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, n)
	for i := range b {
		b[i] = letters[time.Now().UnixNano()%int64(len(letters))]
		time.Sleep(1 * time.Nanosecond)
	}
	return string(b)
}
