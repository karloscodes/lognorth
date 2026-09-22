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

	if err := db.AutoMigrate(&APIKey{}); err != nil {
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

func TestRename(t *testing.T) {
	// Single DB for the whole test; each case uses its own app rows so there
	// is no cross-case interference. (setupTestDB's filename uses t.Name(),
	// which contains "/" for subtests and fails to open.)
	db := setupTestDB(t)

	t.Run("updates the name and trims whitespace", func(t *testing.T) {
		apiKey, _, _ := Create(db, "Original-trim")
		if err := Rename(db, apiKey.ID, "   Renamed   "); err != nil {
			t.Fatalf("Rename failed: %v", err)
		}
		reloaded, _ := FindByID(db, apiKey.ID)
		if reloaded.Name != "Renamed" {
			t.Errorf("expected name 'Renamed', got %q", reloaded.Name)
		}
	})

	t.Run("rejects empty or whitespace-only names", func(t *testing.T) {
		apiKey, _, _ := Create(db, "Keep-empty")
		for _, name := range []string{"", "   ", "\t\n"} {
			if err := Rename(db, apiKey.ID, name); err != ErrNameRequired {
				t.Errorf("name %q: expected ErrNameRequired, got %v", name, err)
			}
		}
		reloaded, _ := FindByID(db, apiKey.ID)
		if reloaded.Name != "Keep-empty" {
			t.Errorf("name should not have changed, got %q", reloaded.Name)
		}
	})

	t.Run("rejects names over 60 chars", func(t *testing.T) {
		apiKey, _, _ := Create(db, "Keep-long")
		if err := Rename(db, apiKey.ID, strings.Repeat("a", 61)); err != ErrNameTooLong {
			t.Errorf("expected ErrNameTooLong, got %v", err)
		}
	})

	t.Run("rejects a name already in use by another active app", func(t *testing.T) {
		Create(db, "Alpha-dup")
		beta, _, _ := Create(db, "Beta-dup")
		if err := Rename(db, beta.ID, "Alpha-dup"); err != ErrNameDuplicate {
			t.Errorf("expected ErrNameDuplicate, got %v", err)
		}
	})

	t.Run("allows reusing a name owned by a revoked app", func(t *testing.T) {
		old, _, _ := Create(db, "Recycled-revoked")
		Revoke(db, old.ID)
		fresh, _, _ := Create(db, "Placeholder-recycle")
		if err := Rename(db, fresh.ID, "Recycled-revoked"); err != nil {
			t.Errorf("should be allowed since old app is revoked, got %v", err)
		}
	})

	t.Run("returns ErrAppNotFound when id does not exist", func(t *testing.T) {
		if err := Rename(db, 999999, "Ghost"); err != ErrAppNotFound {
			t.Errorf("expected ErrAppNotFound, got %v", err)
		}
	})
}
