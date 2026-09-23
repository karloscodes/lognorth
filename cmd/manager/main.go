// LogNorth Manager - Installation and management tool
package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/karloscodes/matcha"
)

var currentVersion string = "dev"

func main() {
	m := matcha.New(matcha.Config{
		Name:           "lognorth",
		AppImage:       "karloscodes/lognorth:latest",
		HealthPath:     "/_health",
		CronUpdates:    true,
		Backups:        true,
		Volumes:        []string{"/app/storage"},
		ManagerRepo:    "karloscodes/lognorth-releases",
		ManagerVersion: currentVersion,
	})

	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	var err error
	switch os.Args[1] {
	case "install":
		err = m.Install()
	case "update":
		err = m.Update()
	case "reload":
		err = m.Reload()
	case "status":
		err = m.Status()
	case "reset-password":
		err = resetPassword(os.Args[2:])
	case "version", "--version", "-v":
		fmt.Println(currentVersion)
		return
	case "help", "--help", "-h":
		printUsage()
		return
	default:
		fmt.Printf("Unknown command: %s\n", os.Args[1])
		printUsage()
		os.Exit(1)
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println("Usage: lognorth [command]")
	fmt.Println()
	fmt.Println("Commands:")
	fmt.Println("  install    Install LogNorth on this server")
	fmt.Println("  update     Update to the latest version")
	fmt.Println("  reload     Restart with current config")
	fmt.Println("  status     Check system status")
	fmt.Println("  reset-password <email>  Set a new password for a user")
	fmt.Println("  version    Show version information")
	fmt.Println("  help       Show this help message")
	fmt.Println()
	fmt.Println("To read this server from your laptop, install north: curl -fsSL https://lognorth.com/cli | sh")
}

// resetPassword runs lnctl reset-password inside the running app container,
// so a locked-out owner does not need to know docker exec.
func resetPassword(args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("usage: lognorth reset-password <email>")
	}

	// During a zero-downtime deploy both containers run; the -next one is newer.
	container := ""
	for _, name := range []string{"lognorth-next", "lognorth"} {
		out, err := exec.Command("docker", "inspect", "-f", "{{.State.Running}}", name).Output()
		if err == nil && strings.TrimSpace(string(out)) == "true" {
			container = name
			break
		}
	}
	if container == "" {
		return fmt.Errorf("LogNorth is not running; start it with: lognorth reload")
	}

	flags := "-i"
	if info, err := os.Stdin.Stat(); err == nil && info.Mode()&os.ModeCharDevice != 0 {
		flags = "-it" // a terminal: let lnctl hide the password as it is typed
	}
	cmd := exec.Command("docker", "exec", flags, container, "/app/lnctl", "reset-password", args[0])
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	return cmd.Run()
}
