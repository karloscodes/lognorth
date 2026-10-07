// Package users provides user management for LogNorth
package users

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/karloscodes/cartridge"
	"github.com/karloscodes/cartridge/crypto"
	"gorm.io/gorm"
)

// User represents an admin user
type User struct {
	ID                  uint   `gorm:"primaryKey"`
	Email               string `gorm:"uniqueIndex"`
	EncryptedPassword   string
	ResetPasswordToken  sql.NullString
	ResetPasswordSentAt sql.NullTime
	// SessionsEndedAt is the last logout or password change. A session issued
	// before it no longer counts.
	SessionsEndedAt sql.NullTime
	CreatedAt       time.Time `gorm:"autoCreateTime"`
	UpdatedAt       time.Time `gorm:"autoUpdateTime"`
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

// SetPassword replaces a user's password. lnctl reset-password uses it when
// the owner is locked out.
func SetPassword(db *gorm.DB, email, password string) error {
	if len(password) < 8 {
		return errors.New("password must be at least 8 characters")
	}
	user, err := FindByEmail(db, email)
	if err != nil {
		return err
	}
	hashed, err := crypto.GeneratePasswordHash(password)
	if err != nil {
		return err
	}
	// A new password ends the sessions that the old one opened.
	return db.Model(user).Updates(map[string]any{
		"encrypted_password": string(hashed),
		"sessions_ended_at":  time.Now().UTC(),
	}).Error
}

// EndSessions ends every session the user has open. The session cookie is
// signed and stateless, so the server cannot delete it: a logout only counts
// if a copy of the cookie stops working too.
func EndSessions(db *gorm.DB, userID uint) error {
	return db.Model(&User{}).Where("id = ?", userID).Update("sessions_ended_at", time.Now().UTC()).Error
}

// SessionAlive reports whether a session issued at issuedAt still counts.
// A cookie from before sessions carried an issue time has the zero time, so
// it works until the first logout.
func SessionAlive(db *gorm.DB, userID uint, issuedAt time.Time) bool {
	user, err := FindByID(db, userID)
	if err != nil {
		return false
	}
	return !user.SessionsEndedAt.Valid || issuedAt.After(user.SessionsEndedAt.Time)
}

// CreateUser creates a new user with the supplied credentials
// ErrSetupDone means the first user already exists.
var ErrSetupDone = errors.New("setup is already done")

// CreateFirstUser creates the owner during setup. The "no users yet" check
// and the insert run in one write transaction, so two setup sessions cannot
// both create a user.
func CreateFirstUser(dbManager cartridge.DBManager, email, password string) error {
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

	return cartridge.Write(context.Background(), dbManager, func(tx *gorm.DB) error {
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

	return db.Transaction(func(tx *gorm.DB) error {
		return tx.Create(&newUser).Error
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
