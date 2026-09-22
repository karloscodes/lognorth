package http

import (
	"errors"
	"log/slog"
	"strings"

	"github.com/gofiber/fiber/v2"
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

// OnboardingPageAction handles GET /setup: the form that creates the owner.
func OnboardingPageAction(ctx *cartridge.Context) error {
	if setupDone(ctx) {
		return ctx.Redirect("/login", fiber.StatusFound)
	}
	return RenderPage(ctx.Ctx, "Onboarding", Props{})
}

// OnboardingAction handles POST /setup. It creates the owner and logs them in.
// Once any user exists it refuses, so setup cannot add a second admin.
func OnboardingAction(ctx *cartridge.Context) error {
	if setupDone(ctx) {
		return ctx.Redirect("/login", fiber.StatusFound)
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
		return RenderPage(ctx.Ctx, "Onboarding", Props{
			"email": email,
			"flash": flash.FlashMessage{Type: "error", Message: problem},
		})
	}

	err := users.CreateFirstUser(ctx.DB(), email, password)
	if errors.Is(err, users.ErrSetupDone) {
		return ctx.Redirect("/login", fiber.StatusFound)
	}
	if err != nil {
		ctx.Logger.Error("Failed to create user", slog.Any("error", err))
		return RenderPage(ctx.Ctx, "Onboarding", Props{
			"email": email,
			"flash": flash.FlashMessage{Type: "error", Message: "Failed to create account"},
		})
	}

	if user, err := users.FindByEmail(ctx.DB(), email); err == nil {
		ctx.Session.SetSession(ctx.Ctx, user.ID)
	}
	flash.SetFlash(ctx.Ctx, "success", "Account created successfully!")
	return ctx.Redirect("/", fiber.StatusFound)
}

// setupDone reports whether the first user exists.
func setupDone(ctx *cartridge.Context) bool {
	required, err := users.IsOnboardingRequired(ctx.DB())
	return err != nil || !required
}
