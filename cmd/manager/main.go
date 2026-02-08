// LogNorth Manager - Installation and management tool
package main

import (
	"fmt"
	"os"
	"time"

	"lognorth/internal/manager/installer"
	"lognorth/internal/manager/logging"
)

var currentVersion string = "dev"

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	logger := logging.NewLogger(logging.Config{
		Level:   "info",
		Verbose: os.Getenv("VERBOSE") == "true",
	})

	switch os.Args[1] {
	case "install":
		runInstall(logger)
	case "update":
		runUpdate(logger)
	case "status":
		runStatus(logger)
	case "version", "--version", "-v":
		fmt.Println(currentVersion)
	case "help", "--help", "-h":
		printUsage()
	default:
		fmt.Printf("Unknown command: %s\n", os.Args[1])
		printUsage()
		os.Exit(1)
	}
}

func runInstall(logger *logging.Logger) {
	startTime := time.Now()

	inst := installer.NewInstaller(logger)

	if err := inst.RunCompleteInstallation(); err != nil {
		logger.Error("Installation failed: %v", err)
		os.Exit(1)
	}

	elapsed := time.Since(startTime).Round(time.Second)
	logger.Success("Installation completed in %s", elapsed)

	inst.DisplayCompletionMessage()
}

func runUpdate(logger *logging.Logger) {
	logger.Info("Checking for updates...")
	// TODO: Implement update logic
	logger.Warn("Update not yet implemented - pull latest image manually:")
	logger.Info("  docker pull karloscodes/lognorth:latest")
	logger.Info("  docker stop lognorth-app && docker rm lognorth-app")
	logger.Info("  # Then run: lognorth install")
}

func runStatus(logger *logging.Logger) {
	logger.Info("Checking system status...")

	// Check Docker
	inst := installer.NewInstaller(logger)
	running, err := inst.Docker().VerifyContainersRunning()
	if err != nil {
		logger.Error("Failed to check containers: %v", err)
		os.Exit(1)
	}

	if running {
		logger.Success("All containers running")
	} else {
		logger.Warn("Some containers not running")
	}
}

func printUsage() {
	fmt.Println("Usage: lognorth [command]")
	fmt.Println()
	fmt.Println("Commands:")
	fmt.Println("  install    Install LogNorth on this server")
	fmt.Println("  update     Update to the latest version")
	fmt.Println("  status     Check system status")
	fmt.Println("  version    Show version information")
	fmt.Println("  help       Show this help message")
}
