// main.go - Admin control tool for LogNorth
package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"gorm.io/gorm"

	"lognorth/internal"
	"lognorth/internal/apikeys"
	"lognorth/internal/events"
	"lognorth/internal/users"
)

const (
	defaultShutdownTimeout = 30 * time.Second
)

// Command defines the interface for all command implementations
type Command interface {
	Name() string
	Description() string
	Execute(ctx context.Context, app *internal.Application, args []string) error
}

var commands = []Command{
	&MigrateCommand{},
	&StatusCommand{},
	&ResetPasswordCommand{},
	&HelpCommand{},
}

func main() {
	flag.Parse()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		sig := <-sigChan
		log.Printf("Received signal: %v, initiating cleanup...", sig)
		cancel()
	}()

	cmdName, args := parseArgs()

	cmd := findCommand(cmdName)
	if cmd == nil {
		showUsageAndExit()
	}

	app, err := internal.NewApp()
	if err != nil {
		log.Printf("Warning: Failed to initialize app: %v", err)
		log.Println("Proceeding with limited functionality...")
	}

	defer func() {
		if app != nil {
			shutdownCtx, cancel := context.WithTimeout(context.Background(), defaultShutdownTimeout)
			defer cancel()
			if err := app.Shutdown(shutdownCtx); err != nil {
				log.Printf("Warning: Cleanup error: %v", err)
			}
		}
	}()

	if err := cmd.Execute(ctx, app, args); err != nil {
		log.Fatalf("Command failed: %v", err)
	}

	log.Printf("Command %s completed successfully", cmd.Name())
}

// MigrateCommand runs database migrations
type MigrateCommand struct{}

func (c *MigrateCommand) Name() string        { return "migrate" }
func (c *MigrateCommand) Description() string { return "Runs database migrations" }

func (c *MigrateCommand) Execute(ctx context.Context, app *internal.Application, args []string) error {
	if app == nil {
		return fmt.Errorf("app initialization failed, cannot run migrations")
	}

	log.Println("Running database migrations...")

	if err := app.DBManager.MigrateDatabase(); err != nil {
		return fmt.Errorf("migration failed: %w", err)
	}

	log.Println("Migrations completed successfully")
	return nil
}

// StatusCommand shows system status
type StatusCommand struct{}

func (c *StatusCommand) Name() string        { return "status" }
func (c *StatusCommand) Description() string { return "Shows the current system status" }

func (c *StatusCommand) Execute(ctx context.Context, app *internal.Application, args []string) error {
	if app == nil {
		return fmt.Errorf("cannot check status: app initialization failed")
	}

	db := app.DBManager.GetConnection()

	var userCount int64
	if err := db.Model(&users.User{}).Count(&userCount).Error; err != nil {
		return fmt.Errorf("database error: %w", err)
	}

	var appCount int64
	if err := db.Model(&apikeys.APIKey{}).Count(&appCount).Error; err != nil {
		return fmt.Errorf("database error: %w", err)
	}

	var eventCount int64
	if err := db.Model(&events.Event{}).Count(&eventCount).Error; err != nil {
		return fmt.Errorf("database error: %w", err)
	}

	log.Println("System Status:")
	log.Println("- Database: Connected")
	log.Printf("- Users: %d", userCount)
	log.Printf("- Apps: %d", appCount)
	log.Printf("- Events: %d", eventCount)

	sqlDB, err := db.DB()
	if err != nil {
		return fmt.Errorf("failed to get SQL DB: %w", err)
	}

	log.Printf("- Max Open Connections: %d", sqlDB.Stats().MaxOpenConnections)
	log.Printf("- Open Connections: %d", sqlDB.Stats().OpenConnections)
	log.Printf("- In Use: %d", sqlDB.Stats().InUse)
	log.Printf("- Idle: %d", sqlDB.Stats().Idle)

	return nil
}

// ResetPasswordCommand sets a new password for a user who is locked out.
// Run it inside the container: docker exec -it lognorth /app/lnctl reset-password you@example.com
type ResetPasswordCommand struct{}

func (c *ResetPasswordCommand) Name() string { return "reset-password" }
func (c *ResetPasswordCommand) Description() string {
	return "Sets a new password for a user: reset-password <email>"
}

func (c *ResetPasswordCommand) Execute(ctx context.Context, app *internal.Application, args []string) error {
	if app == nil {
		return fmt.Errorf("app initialization failed")
	}
	if len(args) != 1 {
		return fmt.Errorf("usage: lnctl reset-password <email>")
	}
	email := args[0]

	in := bufio.NewReader(os.Stdin)
	password, err := readHidden(in, "New password: ")
	if err != nil {
		return err
	}
	again, err := readHidden(in, "Repeat it: ")
	if err != nil {
		return err
	}
	if password != again {
		return fmt.Errorf("the passwords do not match")
	}

	if err := users.SetPassword(app.DBManager.GetConnection(), email, password); err != nil {
		return fmt.Errorf("could not set the password for %s: %w", email, err)
	}
	log.Printf("Password updated for %s. To sign out every session, also rotate LOGNORTH_PRIVATE_KEY.", email)
	return nil
}

// readHidden reads one line without echoing it when stdin is a terminal.
// It uses the system stty, so it needs no extra dependency; with piped input
// stty fails quietly and the line is read as is.
func readHidden(in *bufio.Reader, prompt string) (string, error) {
	fmt.Fprint(os.Stderr, prompt)
	stty := func(mode string) {
		cmd := exec.Command("stty", mode)
		cmd.Stdin = os.Stdin
		_ = cmd.Run()
	}
	stty("-echo")
	defer func() {
		stty("echo")
		fmt.Fprintln(os.Stderr)
	}()

	line, err := in.ReadString('\n')
	if err != nil && line == "" {
		return "", fmt.Errorf("could not read the password: %w", err)
	}
	return strings.TrimRight(line, "\r\n"), nil
}

// HelpCommand shows usage information
type HelpCommand struct{}

func (c *HelpCommand) Name() string        { return "help" }
func (c *HelpCommand) Description() string { return "Shows usage information" }

func (c *HelpCommand) Execute(ctx context.Context, app *internal.Application, args []string) error {
	fmt.Println("Usage: lnctl [command] [args...]")
	fmt.Println("Available commands:")

	for _, cmd := range commands {
		fmt.Printf("  %s: %s\n", cmd.Name(), cmd.Description())
	}

	return nil
}

func parseArgs() (string, []string) {
	args := os.Args[1:]
	if len(args) == 0 {
		return "help", []string{}
	}
	return args[0], args[1:]
}

func findCommand(name string) Command {
	for _, cmd := range commands {
		if cmd.Name() == name {
			return cmd
		}
	}
	return nil
}

func showUsageAndExit() {
	fmt.Println("Usage: lnctl [command] [args...]")
	fmt.Println("Available commands:")

	for _, cmd := range commands {
		fmt.Printf("  %s: %s\n", cmd.Name(), cmd.Description())
	}

	os.Exit(1)
}

// Ensure gorm is imported
var _ *gorm.DB
