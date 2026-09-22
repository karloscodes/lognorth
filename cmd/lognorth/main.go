package main

import (
	"log"
	"time"

	"lognorth/internal"
)

func main() {
	// SQLite compares stored times as text, so every time the server writes or
	// queries must use the same offset: UTC.
	time.Local = time.UTC

	app, err := internal.NewApp()
	if err != nil {
		log.Fatalf("Failed to create application: %v", err)
	}

	if err := app.Run(); err != nil {
		log.Fatalf("Application error: %v", err)
	}
}
