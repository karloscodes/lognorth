package apikeys

import (
	"os"
	"strings"
	"testing"

	"github.com/karloscodes/cartridge/sqlite"
	"gorm.io/gorm"
)

func setupTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	dbPath := "test_" + t.Name() + ".db"
	t.Cleanup(func() {
		os.Remove(dbPath)
		os.Remove(dbPath + "-wal")
		os.Remove(dbPath + "-shm")
	})

	manager := sqlite.NewManager(sqlite.Config{Path: dbPath, EnableWAL: true})
	db, err := manager.Connect()
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}

	if err := AutoMigrate(db); err != nil {
		t.Fatalf("Failed to migrate: %v", err)
	}

	return db
}

func TestGenerate(t *testing.T) {
	t.Run("app key prefix", func(t *testing.T) {
		key, err := Generate("lgn-")

		if err != nil {
			t.Fatalf("Generate failed: %v", err)
		}
		if !strings.HasPrefix(key, "lgn-") {
			t.Errorf("expected key to start with 'lgn-', got %s", key[:10])
		}
		if len(key) != 68 { // "lgn-" + 64 hex chars
			t.Errorf("expected key length 68, got %d", len(key))
		}
	})

	t.Run("agent key prefix", func(t *testing.T) {
		key, err := Generate("lgn-agent-")

		if err != nil {
			t.Fatalf("Generate failed: %v", err)
		}
		if !strings.HasPrefix(key, "lgn-agent-") {
			t.Errorf("expected key to start with 'lgn-agent-', got %s", key[:15])
		}
		if len(key) != 74 { // "lgn-agent-" + 64 hex chars
			t.Errorf("expected key length 74, got %d", len(key))
		}
	})
}

func TestGenerate_Uniqueness(t *testing.T) {
	keys := make(map[string]bool)

	for i := 0; i < 100; i++ {
		key, _ := Generate("lb_")
		if keys[key] {
			t.Fatalf("duplicate key generated: %s", key)
		}
		keys[key] = true
	}
}

func TestCreate(t *testing.T) {
	db := setupTestDB(t)

	apiKey, fullKey, err := Create(db, "Production")

	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	if apiKey.ID == 0 {
		t.Error("expected ID to be set")
	}
	if apiKey.Name != "Production" {
		t.Errorf("expected name 'Production', got %s", apiKey.Name)
	}
	if !apiKey.Active {
		t.Error("expected key to be active")
	}
	if apiKey.KeyPrefix != fullKey[:12] {
		t.Error("prefix should match first 12 chars of key")
	}
}

func TestValidate(t *testing.T) {
	db := setupTestDB(t)

	t.Run("valid key", func(t *testing.T) {
		_, fullKey, _ := Create(db, "Test")

		validated, err := Validate(db, fullKey)

		if err != nil {
			t.Fatalf("Validate failed: %v", err)
		}
		if validated.Name != "Test" {
			t.Errorf("expected name 'Test', got %s", validated.Name)
		}
	})

	t.Run("invalid key", func(t *testing.T) {
		_, err := Validate(db, "lb_invalid_key_that_does_not_exist")

		if err == nil {
			t.Error("expected error for invalid key")
		}
	})

	t.Run("revoked key", func(t *testing.T) {
		apiKey, fullKey, _ := Create(db, "Revoked")
		Revoke(db, apiKey.ID)

		_, err := Validate(db, fullKey)

		if err == nil {
			t.Error("expected error for revoked key")
		}
	})
}

func TestRevoke(t *testing.T) {
	db := setupTestDB(t)
	apiKey, _, _ := Create(db, "ToRevoke")

	err := Revoke(db, apiKey.ID)

	if err != nil {
		t.Fatalf("Revoke failed: %v", err)
	}

	var updated APIKey
	db.First(&updated, apiKey.ID)
	if updated.Active {
		t.Error("expected key to be inactive after revoke")
	}
}

func TestList(t *testing.T) {
	db := setupTestDB(t)
	Create(db, "First")
	Create(db, "Second")

	keys, err := List(db)

	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(keys) != 2 {
		t.Errorf("expected 2 keys, got %d", len(keys))
	}
}

func TestHasAny(t *testing.T) {
	db := setupTestDB(t)

	t.Run("no keys", func(t *testing.T) {
		if HasAny(db) {
			t.Error("expected false when no keys exist")
		}
	})

	t.Run("with active key", func(t *testing.T) {
		Create(db, "Active")

		if !HasAny(db) {
			t.Error("expected true when active key exists")
		}
	})

	t.Run("only revoked keys", func(t *testing.T) {
		// Revoke the key we created above
		keys, _ := List(db)
		for _, k := range keys {
			Revoke(db, k.ID)
		}

		if HasAny(db) {
			t.Error("expected false when only revoked keys exist")
		}
	})
}

func TestDelete(t *testing.T) {
	db := setupTestDB(t)
	apiKey, _, _ := Create(db, "ToDelete")

	err := Delete(db, apiKey.ID)

	if err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	_, err = FindByID(db, apiKey.ID)
	if err == nil {
		t.Error("expected error when finding deleted key")
	}
}

func TestValidateAppKey(t *testing.T) {
	db := setupTestDB(t)

	_, fullKey, _ := Create(db, "Test App")

	key, err := ValidateAppKey(db, fullKey)

	if err != nil {
		t.Fatalf("ValidateAppKey failed: %v", err)
	}
	if key.KeyType != KeyTypeApp {
		t.Errorf("expected key type 'app', got %s", key.KeyType)
	}
}

