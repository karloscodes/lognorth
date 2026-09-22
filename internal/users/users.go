// Package users provides user management for LogNorth
package users

import (
	"database/sql"
	"errors"
	"log/slog"
	"time"

	"github.com/karloscodes/cartridge/crypto"
	"github.com/karloscodes/cartridge/sqlite"
	"gorm.io/gorm"
)

// User represents an admin user
type User struct {
	ID                uint   `gorm:"primaryKey"`
	Email             string `gorm:"uniqueIndex"`
	EncryptedPassword string
	ResetPasswordToken  sql.NullString
	ResetPasswordSentAt sql.NullTime
	CreatedAt         time.Time `gorm:"autoCreateTime"`
	UpdatedAt         time.Time `gorm:"autoUpdateTime"`
}

// ErrUserExists is returned when attempting to create a user that already exists
var ErrUserExists = errors.New("user already exists")

// ErrUserNotFound is returned when a user lookup fails
var ErrUserNotFound = gorm.ErrRecordNotFound

// FindByEmail retrieves a user by email
func FindByEmail(db *gorm.DB, email string) (*User, error) {
	var user User
	if err := db.Where("email = ?", email).First(&user).Error; err != nil {
		return nil, err
	}
	return &user, nil
}

// FindByID retrieves a user by ID
func FindByID(db *gorm.DB, id uint) (*User, error) {
	var user User
	if err := db.Where("id = ?", id).First(&user).Error; err != nil {
		return nil, err
	}
	return &user, nil
}

// CreateUser creates a new user with the supplied credentials
// ErrSetupDone means the first user already exists.
var ErrSetupDone = errors.New("setup is already done")

// CreateFirstUser creates the owner during setup. The "no users yet" check
// and the insert run in one write transaction, so two setup sessions cannot
// both create a user.
func CreateFirstUser(db *gorm.DB, email, password string) error {
	if email == "" {
		return errors.New("email cannot be empty")
	}
	if password == "" {
		return errors.New("password cannot be empty")
	}

	hashedPassword, err := crypto.GeneratePasswordHash(password)
	if err != nil {
		return err
	}

	return sqlite.PerformWrite(slog.Default(), db, func(tx *gorm.DB) error {
		var count int64
		if err := tx.Model(&User{}).Count(&count).Error; err != nil {
			return err
		}
		if count > 0 {
			return ErrSetupDone
		}
		return tx.Create(&User{Email: email, EncryptedPassword: string(hashedPassword)}).Error
	})
}

func CreateUser(db *gorm.DB, email, password string) error {
	// Check existence first
	if _, err := FindByEmail(db, email); err == nil {
		return ErrUserExists
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}

	if email == "" {
		return errors.New("email cannot be empty")
	}
	if password == "" {
		return errors.New("password cannot be empty")
	}

	hashedPassword, err := crypto.GeneratePasswordHash(password)
	if err != nil {
		return err
	}

	newUser := User{
		Email:             email,
		EncryptedPassword: string(hashedPassword),
	}

	logger := slog.Default()
	return sqlite.PerformWrite(logger, db, func(tx *gorm.DB) error {
		return tx.Create(&newUser).Error
	})
}

// ChangePassword updates a user's password
func ChangePassword(db *gorm.DB, email, password string) error {
	if password == "" {
		return errors.New("password cannot be empty")
	}

	user, err := FindByEmail(db, email)
	if err != nil {
		return err
	}

	hashedPassword, err := crypto.GeneratePasswordHash(password)
	if err != nil {
		return err
	}

	logger := slog.Default()
	return sqlite.PerformWrite(logger, db, func(tx *gorm.DB) error {
		return tx.Model(user).Update("encrypted_password", string(hashedPassword)).Error
	})
}

// Count returns the number of users
func Count(db *gorm.DB) (int64, error) {
	var count int64
	err := db.Model(&User{}).Count(&count).Error
	return count, err
}

// IsOnboardingRequired returns true if no users exist
func IsOnboardingRequired(db *gorm.DB) (bool, error) {
	count, err := Count(db)
	if err != nil {
		return false, err
	}
	return count == 0, nil
}
