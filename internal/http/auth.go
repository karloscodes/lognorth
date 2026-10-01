package http

import (
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/karloscodes/cartridge"
	"github.com/karloscodes/cartridge/crypto"
	"github.com/karloscodes/cartridge/flash"

	"lognorth/internal/users"
)

// LoginPageAction handles GET /login
func LoginPageAction(ctx *cartridge.Context) error {
	db := ctx.DB()

	// Check if onboarding is required first
	required, err := users.IsOnboardingRequired(db)
	if err != nil {
		ctx.Logger.Error("Failed to check if onboarding required on login", slog.Any("error", err))
	} else if required {
		return ctx.Redirect("/setup", http.StatusFound)
	}

	// If already authenticated, redirect to home
	if sessionAlive(ctx) {
		return ctx.Redirect("/", http.StatusFound)
	}

	return RenderPage(ctx, "Login", Props{})
}

// LoginAction handles POST /login
func LoginAction(ctx *cartridge.Context) error {
	// Parse login form
	email := ctx.FormValue("email")
	password := ctx.FormValue("password")

	if email == "" || password == "" {
		setFlash(ctx, "error", "Email and password are required")
		return ctx.Redirect("/login", http.StatusFound)
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
		setFlash(ctx, "error", "Invalid email or password")
		return ctx.Redirect("/login", http.StatusFound)
	}

	// Set session
	if err := ctx.Session.SetSession(ctx, user.ID); err != nil {
		ctx.Logger.Error("Failed to set session", slog.Any("error", err))
		setFlash(ctx, "error", "Login failed")
		return ctx.Redirect("/login", http.StatusFound)
	}

	ctx.Logger.Debug("Login successful", slog.String("email", email), slog.Uint64("userId", uint64(user.ID)))

	return ctx.Redirect("/", http.StatusFound)
}

// RequireLiveSession turns away a session the owner ended with a logout or a
// password change. It runs after the session middleware, which checks the
// signature and the expiry.
func RequireLiveSession(ctx *cartridge.Context) error {
	if sessionAlive(ctx) {
		return ctx.Next()
	}
	ctx.Session.ClearSession(ctx)
	if ctx.Get("HX-Request") == "true" {
		return ctx.Status(http.StatusUnauthorized).SendString("authentication required")
	}
	return ctx.Redirect("/login", http.StatusFound)
}

func sessionAlive(ctx *cartridge.Context) bool {
	userID, ok := ctx.Session.GetUserID(ctx)
	if !ok {
		return false
	}
	issuedAt, _ := ctx.Session.IssuedAt(ctx)
	return users.SessionAlive(ctx.DB(), userID, issuedAt)
}

// LogoutAction handles POST /logout
func LogoutAction(ctx *cartridge.Context) error {
	if userID, ok := ctx.Session.GetUserID(ctx); ok {
		if err := users.EndSessions(ctx.DB(), userID); err != nil {
			ctx.Logger.Error("Failed to end sessions on logout", slog.Any("error", err))
		}
	}
	ctx.Session.ClearSession(ctx)
	setFlash(ctx, "success", "You have been logged out")
	return ctx.Redirect("/login", http.StatusFound)
}

// OnboardingPageAction handles GET /setup: the form that creates the owner.
func OnboardingPageAction(ctx *cartridge.Context) error {
	if setupDone(ctx) {
		return ctx.Redirect("/login", http.StatusFound)
	}
	return RenderPage(ctx, "Onboarding", Props{})
}

// OnboardingAction handles POST /setup. It creates the owner and logs them in.
// Once any user exists it refuses, so setup cannot add a second admin.
func OnboardingAction(ctx *cartridge.Context) error {
	if setupDone(ctx) {
		return ctx.Redirect("/login", http.StatusFound)
	}

	email := strings.TrimSpace(ctx.FormValue("email"))
	password := ctx.FormValue("password")

	var problem string
	switch {
	case email == "":
		problem = "Email is required"
	case len(password) < 8:
		problem = "Password must be at least 8 characters"
	case password != ctx.FormValue("confirm_password"):
		problem = "Passwords do not match"
	}
	if problem != "" {
		return RenderPage(ctx, "Onboarding", Props{
			"email": email,
			"flash": flash.FlashMessage{Type: "error", Message: problem},
		})
	}

	err := users.CreateFirstUser(ctx.DB(), email, password)
	if errors.Is(err, users.ErrSetupDone) {
		return ctx.Redirect("/login", http.StatusFound)
	}
	if err != nil {
		ctx.Logger.Error("Failed to create user", slog.Any("error", err))
		return RenderPage(ctx, "Onboarding", Props{
			"email": email,
			"flash": flash.FlashMessage{Type: "error", Message: "Failed to create account"},
		})
	}

	if user, err := users.FindByEmail(ctx.DB(), email); err == nil {
		ctx.Session.SetSession(ctx, user.ID)
	}
	setFlash(ctx, "success", "Account created successfully!")
	return ctx.Redirect("/", http.StatusFound)
}

// setupDone reports whether the first user exists.
func setupDone(ctx *cartridge.Context) bool {
	required, err := users.IsOnboardingRequired(ctx.DB())
	return err != nil || !required
}
