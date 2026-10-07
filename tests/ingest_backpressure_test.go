package tests

import (
	"context"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/karloscodes/cartridge/sqlite"
	"gorm.io/gorm"

	"lognorth/internal/apikeys"
	"lognorth/internal/database"
	"lognorth/internal/events"
)

// When writes queue longer than WriteWait, ingest answers 503 with
// Retry-After, so the SDK keeps the batch and sends it again later.
func TestIngestionBackpressure(t *testing.T) {
	// Several connections, as in production: the handler reads the API key
	// while another write holds its turn.
	manager := sqlite.NewManager(sqlite.Config{
		Path:         filepath.Join(t.TempDir(), "test.db"),
		MaxOpenConns: 4,
		MaxIdleConns: 4,
		WriteWait:    50 * time.Millisecond,
	})
	t.Cleanup(func() { _ = manager.Close() })
	db, err := manager.Connect()
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	if err := database.Migrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	_, fullKey, _ := apikeys.Create(db, "Production")
	server := newTestServer(t, manager, mountIngest)
	post := func() (int, string) {
		req := httptest.NewRequest("POST", "/api/v1/events", strings.NewReader(`{"message":"GET /a → 200"}`))
		req.Header.Set("Authorization", "Bearer "+fullKey)
		req.Header.Set("Content-Type", "application/json")
		resp, err := server.Test(req)
		if err != nil {
			t.Fatalf("request: %v", err)
		}
		return resp.StatusCode, resp.Header.Get("Retry-After")
	}

	t.Run("answers 503 with Retry-After while the write queue is full", func(t *testing.T) {
		started, release := make(chan struct{}), make(chan struct{})
		done := make(chan struct{})
		go func() {
			defer close(done)
			_ = manager.Write(context.Background(), func(tx *gorm.DB) error {
				close(started)
				<-release
				return nil
			})
		}()
		<-started

		begin := time.Now()
		status, retryAfter := post()
		elapsed := time.Since(begin)
		close(release)
		<-done

		if status != 503 {
			t.Errorf("status = %d, want 503", status)
		}
		// WriteWait is 50ms. A slow answer means something waited inside
		// SQLite (busy_timeout) instead of in the write queue.
		if elapsed > time.Second {
			t.Errorf("503 took %v, want about WriteWait", elapsed)
		}
		if retryAfter == "" {
			t.Error("Retry-After is missing")
		}
		var stored int64
		db.Model(&events.IngestedEvent{}).Count(&stored)
		if stored != 0 {
			t.Errorf("stored = %d, want 0", stored)
		}
	})

	t.Run("stores the batch when the client sends it again", func(t *testing.T) {
		status, _ := post()

		if status != 201 {
			t.Errorf("status = %d, want 201", status)
		}
		var stored int64
		db.Model(&events.IngestedEvent{}).Count(&stored)
		if stored != 1 {
			t.Errorf("stored = %d, want 1", stored)
		}
		var key apikeys.APIKey
		db.First(&key)
		if key.LastUsed.IsZero() {
			t.Error("last_used is not set after the batch was stored")
		}
	})
}
