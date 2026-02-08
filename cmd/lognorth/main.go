package main

import (
	"log"

	"lognorth/internal"
)

func main() {
	app, err := internal.NewApp()
	if err != nil {
		log.Fatalf("Failed to create application: %v", err)
	}

	if err := app.Run(); err != nil {
		log.Fatalf("Application error: %v", err)
	}
}
