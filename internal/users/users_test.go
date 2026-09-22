package users

import (
	"os"
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

	if err := db.AutoMigrate(&User{}); err != nil {
		t.Fatalf("Failed to migrate: %v", err)
	}

	return db
}

func TestCreateUser(t *testing.T) {
	db := setupTestDB(t)

	err := CreateUser(db, "test@example.com", "password123")

	if err != nil {
		t.Fatalf("CreateUser failed: %v", err)
	}

	user, err := FindByEmail(db, "test@example.com")
	if err != nil {
		t.Fatalf("FindByEmail failed: %v", err)
	}
	if user.Email != "test@example.com" {
		t.Errorf("expected email 'test@example.com', got %s", user.Email)
	}
	if user.EncryptedPassword == "" {
		t.Error("expected password to be hashed")
	}
	if user.EncryptedPassword == "password123" {
		t.Error("password should be hashed, not stored in plain text")
	}
}

func TestCreateUser_EmptyEmail(t *testing.T) {
	db := setupTestDB(t)

	err := CreateUser(db, "", "password123")

	if err == nil {
		t.Error("expected error for empty email")
	}
}

func TestCreateUser_EmptyPassword(t *testing.T) {
	db := setupTestDB(t)

	err := CreateUser(db, "test@example.com", "")

	if err == nil {
		t.Error("expected error for empty password")
	}
}

func TestCreateUser_Duplicate(t *testing.T) {
	db := setupTestDB(t)
	CreateUser(db, "test@example.com", "password123")

	err := CreateUser(db, "test@example.com", "differentpassword")

	if err != ErrUserExists {
		t.Errorf("expected ErrUserExists, got %v", err)
	}
}

func TestFindByEmail_NotFound(t *testing.T) {
	db := setupTestDB(t)

	_, err := FindByEmail(db, "nonexistent@example.com")

	if err == nil {
		t.Error("expected error for non-existent user")
	}
}

func TestFindByID(t *testing.T) {
	db := setupTestDB(t)
	CreateUser(db, "test@example.com", "password123")

	user, _ := FindByEmail(db, "test@example.com")
	found, err := FindByID(db, user.ID)

	if err != nil {
		t.Fatalf("FindByID failed: %v", err)
	}
	if found.Email != "test@example.com" {
		t.Errorf("expected email 'test@example.com', got %s", found.Email)
	}
}

func TestChangePassword(t *testing.T) {
	db := setupTestDB(t)
	CreateUser(db, "test@example.com", "oldpassword")

	user1, _ := FindByEmail(db, "test@example.com")
	oldHash := user1.EncryptedPassword

	err := ChangePassword(db, "test@example.com", "newpassword")

	if err != nil {
		t.Fatalf("ChangePassword failed: %v", err)
	}

	user2, _ := FindByEmail(db, "test@example.com")
	if user2.EncryptedPassword == oldHash {
		t.Error("password hash should have changed")
	}
}

func TestChangePassword_EmptyPassword(t *testing.T) {
	db := setupTestDB(t)
	CreateUser(db, "test@example.com", "password123")

	err := ChangePassword(db, "test@example.com", "")

	if err == nil {
		t.Error("expected error for empty password")
	}
}

func TestCount(t *testing.T) {
	db := setupTestDB(t)

	count, _ := Count(db)
	if count != 0 {
		t.Errorf("expected 0 users, got %d", count)
	}

	CreateUser(db, "user1@example.com", "pass1")
	CreateUser(db, "user2@example.com", "pass2")

	count, _ = Count(db)
	if count != 2 {
		t.Errorf("expected 2 users, got %d", count)
	}
}

func TestIsOnboardingRequired(t *testing.T) {
	db := setupTestDB(t)

	required, _ := IsOnboardingRequired(db)
	if !required {
		t.Error("should require onboarding when no users exist")
	}

	CreateUser(db, "admin@example.com", "password")

	required, _ = IsOnboardingRequired(db)
	if required {
		t.Error("should not require onboarding when user exists")
	}
}
