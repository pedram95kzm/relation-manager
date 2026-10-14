package auth

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
	"relations/internal/config"
	"relations/internal/platform"
)

// SeedDefaults initializes missing roles without changing existing accounts.
// Public demonstration credentials are confined to development and test runs.
func (s *Service) SeedDefaults(ctx context.Context) error {
	if !s.cfg.SeedDefaultAccounts {
		return nil
	}
	publicPassword := func(password string) bool {
		return password == config.DefaultAdminPassword || password == config.DefaultUserPassword
	}
	if s.cfg.Environment == "production" && (publicPassword(s.cfg.AdminPassword) || publicPassword(s.cfg.UserPassword)) {
		return platform.Err(400, "public_default_credentials", "Production initialization requires private passwords instead of public demonstration credentials.")
	}
	age := max(30, s.cfg.MinAge)
	if age > 120 {
		return platform.Err(400, "invalid_default_accounts", "Default account age must satisfy the configured eligibility policy.")
	}
	return s.repo.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Share the explicit administrator seed lock. The table lock also keeps
		// concurrent registration and Google provisioning from changing whether
		// a role is missing between the check and its initialization.
		if err := tx.Exec("SELECT pg_advisory_xact_lock(78290532)").Error; err != nil {
			return err
		}
		if err := tx.Exec("LOCK TABLE users IN SHARE ROW EXCLUSIVE MODE").Error; err != nil {
			return err
		}
		for _, account := range []struct {
			role, email, password, name string
		}{
			{"admin", s.cfg.AdminEmail, s.cfg.AdminPassword, "Administrator"},
			{"user", s.cfg.UserEmail, s.cfg.UserPassword, "Demo User"},
		} {
			var count int64
			if err := tx.Model(&User{}).Where("role = ?", account.role).Count(&count).Error; err != nil {
				return err
			}
			if count != 0 {
				continue
			}
			email, err := normalizeEmail(account.email)
			if err != nil || !validPassword(account.password) {
				return platform.Err(400, "invalid_default_accounts", fmt.Sprintf("Configure a valid default %s email and a password of 12–72 bytes.", account.role))
			}
			if err = tx.Model(&User{}).Where("email = ?", email).Count(&count).Error; err != nil {
				return err
			}
			if count != 0 {
				return platform.Err(409, "default_email_unavailable", fmt.Sprintf("The configured default %s email already belongs to an existing account.", account.role))
			}
			hash, err := bcrypt.GenerateFromPassword([]byte(account.password), 12)
			if err != nil {
				return err
			}
			user := User{ID: uuid.NewString(), Email: email, Name: account.name, PasswordHash: string(hash),
				Role: account.role, Age: &age, Gender: "prefer_not_to_say", AuthVersion: 1}
			if err = tx.Create(&user).Error; err != nil {
				if errors.Is(err, gorm.ErrDuplicatedKey) {
					return platform.Err(409, "default_email_unavailable", fmt.Sprintf("The configured default %s email already belongs to an existing account.", account.role))
				}
				return err
			}
		}
		return nil
	})
}
