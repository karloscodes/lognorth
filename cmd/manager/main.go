// LogNorth Manager - Installation and management tool
package main

import (
	"fmt"
	"os"

	"github.com/karloscodes/matcha"
)

var currentVersion string = "dev"

func main() {
	m := matcha.New(matcha.Config{
		Name:        "lognorth",
		AppImage:    "karloscodes/lognorth:latest",
		BlueGreen:   true,
		CronUpdates: true,
		Backups:     true,
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
	fmt.Println("  version    Show version information")
	fmt.Println("  help       Show this help message")
}
