package auth

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"relations/internal/database"
	"relations/internal/platform"
)

type Repository struct{ db *gorm.DB }

func invalidCredentials() error {
	return platform.Err(401, "invalid_credentials", "The provided credentials are invalid.")
}

func (r *Repository) userByEmail(ctx context.Context, email string) (User, error) {
	var user User
	err := r.db.WithContext(ctx).Where("email = ?", email).First(&user).Error
	return user, err
}

func (r *Repository) createAccount(ctx context.Context, user *User, session *database.RefreshSession) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(user).Error; err != nil {
			if errors.Is(err, gorm.ErrDuplicatedKey) {
				return platform.Err(409, "registration_failed", "An account could not be created with these details.")
			}
			return err
		}
		return tx.Create(session).Error
	})
}

func lockUser(tx *gorm.DB, userID string) (User, error) {
	var user User
	err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", userID).First(&user).Error
	return user, err
}

func (r *Repository) createSession(ctx context.Context, userID, expectedPasswordHash string, session *database.RefreshSession) (User, error) {
	var user User
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var err error
		user, err = lockUser(tx, userID)
		if err != nil || user.Disabled || (expectedPasswordHash != "" && user.PasswordHash != expectedPasswordHash) {
			return invalidCredentials()
		}
		return tx.Create(session).Error
	})
	return user, err
}

// All session/account writes lock the user before refresh rows. This ordering
// prevents deadlocks between password changes and concurrent token rotations.
func (r *Repository) rotate(ctx context.Context, tokenHash string, next *database.RefreshSession, now time.Time) (User, error) {
	var old database.RefreshSession
	if err := r.db.WithContext(ctx).Where("token_hash = ?", tokenHash).First(&old).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return User{}, invalidCredentials()
		}
		return User{}, err
	}
	var user User
	var rejection error
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var err error
		user, err = lockUser(tx, old.UserID)
		if err != nil {
			return invalidCredentials()
		}
		if err = tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("token_hash = ?", tokenHash).First(&old).Error; err != nil {
			return invalidCredentials()
		}
		if old.UsedAt != nil {
			// Commit reuse revocation even though the caller receives an error.
			rejection = invalidCredentials()
			return tx.Model(&database.RefreshSession{}).Where("family_id = ? AND revoked_at IS NULL", old.FamilyID).Update("revoked_at", now).Error
		}
		if user.Disabled || old.RevokedAt != nil || !old.ExpiresAt.After(now) {
			rejection = invalidCredentials()
			return nil
		}
		if err = tx.Model(&old).Update("used_at", now).Error; err != nil {
			return err
		}
		next.UserID, next.FamilyID = user.ID, old.FamilyID
		return tx.Create(next).Error
	})
	if err != nil {
		return User{}, err
	}
	return user, rejection
}

func (r *Repository) authenticatedUser(ctx context.Context, userID, sessionID string, version int64, now time.Time) (User, error) {
	var user User
	err := r.db.WithContext(ctx).Model(&database.User{}).
		Joins("JOIN refresh_sessions ON refresh_sessions.user_id = users.id").
		Where("users.id = ? AND users.auth_version = ? AND users.disabled = false AND refresh_sessions.id = ? AND refresh_sessions.revoked_at IS NULL AND refresh_sessions.expires_at > ?", userID, version, sessionID, now).
		Select("users.*").First(&user).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return user, invalidCredentials()
	}
	return user, err
}

func (r *Repository) revokeFamily(ctx context.Context, tokenHash string, now time.Time) error {
	var session database.RefreshSession
	err := r.db.WithContext(ctx).Where("token_hash = ?", tokenHash).First(&session).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if _, err := lockUser(tx, session.UserID); err != nil {
			return err
		}
		return tx.Model(&database.RefreshSession{}).Where("family_id = ? AND revoked_at IS NULL", session.FamilyID).Update("revoked_at", now).Error
	})
}

func (r *Repository) revokeAll(ctx context.Context, userID string, now time.Time) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if _, err := lockUser(tx, userID); err != nil {
			return invalidCredentials()
		}
		if err := tx.Model(&database.User{}).Where("id = ?", userID).Update("auth_version", gorm.Expr("auth_version + 1")).Error; err != nil {
			return err
		}
		return tx.Model(&database.RefreshSession{}).Where("user_id = ? AND revoked_at IS NULL", userID).Update("revoked_at", now).Error
	})
}

func (r *Repository) profile(ctx context.Context, userID string, input ProfileInput) (User, error) {
	var user User
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var err error
		user, err = lockUser(tx, userID)
		if err != nil || user.Disabled {
			return invalidCredentials()
		}
		if err = tx.Model(&user).Updates(map[string]any{"name": input.Name, "age": input.Age, "gender": input.Gender}).Error; err != nil {
			return err
		}
		user.Name, user.Age, user.Gender = input.Name, &input.Age, input.Gender
		return nil
	})
	return user, err
}

func (r *Repository) changePassword(ctx context.Context, userID, expectedHash, newHash string, now time.Time) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		user, err := lockUser(tx, userID)
		if err != nil || user.Disabled || user.PasswordHash != expectedHash {
			return invalidCredentials()
		}
		if err = tx.Model(&user).Updates(map[string]any{"password_hash": newHash, "auth_version": gorm.Expr("auth_version + 1")}).Error; err != nil {
			return err
		}
		return tx.Model(&database.RefreshSession{}).Where("user_id = ? AND revoked_at IS NULL", userID).Update("revoked_at", now).Error
	})
}

func (r *Repository) userByID(ctx context.Context, userID string) (User, error) {
	var user User
	err := r.db.WithContext(ctx).Where("id = ?", userID).First(&user).Error
	return user, err
}

func (r *Repository) deleteAccount(ctx context.Context, userID, expectedHash string) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		user, err := lockUser(tx, userID)
		if err != nil || user.Disabled || user.PasswordHash != expectedHash {
			return invalidCredentials()
		}
		if user.Role == "admin" {
			return platform.Err(403, "admin_delete_restricted", "Administrator deletion requires the administrator management procedure.")
		}
		if err = tx.Exec("DELETE FROM daily_usage WHERE scope_key = ?", "user:"+userID).Error; err != nil {
			return err
		}
		return tx.Delete(&user).Error
	})
}

func (r *Repository) googleAccount(ctx context.Context, subject, email, name string) (User, error) {
	var user User
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// A per-subject transaction lock makes concurrent first logins idempotent.
		if err := tx.Exec("SELECT pg_advisory_xact_lock(hashtextextended(?, 17))", "google:"+subject).Error; err != nil {
			return err
		}
		var identity database.AuthIdentity
		err := tx.Where("provider = 'google' AND subject = ?", subject).First(&identity).Error
		if err == nil {
			user, err = lockUser(tx, identity.UserID)
			if err != nil || user.Disabled {
				return invalidCredentials()
			}
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		var count int64
		if err = tx.Model(&database.User{}).Where("email = ?", email).Count(&count).Error; err != nil {
			return err
		}
		if count != 0 {
			return platform.Err(409, "account_link_required", "Sign in using the account's existing method. Automatic account linking is disabled.")
		}
		user = User{ID: uuid.NewString(), Email: email, Name: name, Role: "user", AuthVersion: 1}
		if err = tx.Create(&user).Error; err != nil {
			if errors.Is(err, gorm.ErrDuplicatedKey) {
				return platform.Err(409, "account_link_required", "Sign in using the account's existing method. Automatic account linking is disabled.")
			}
			return err
		}
		return tx.Create(&database.AuthIdentity{ID: uuid.NewString(), UserID: user.ID, Provider: "google", Subject: subject}).Error
	})
	return user, err
}

func (r *Repository) seedAdmin(ctx context.Context, user *User) (bool, error) {
	created := false
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("SELECT pg_advisory_xact_lock(78290532)").Error; err != nil {
			return err
		}
		var count int64
		if err := tx.Model(&database.User{}).Where("role = 'admin'").Count(&count).Error; err != nil {
			return err
		}
		if count != 0 {
			return nil
		}
		if err := tx.Create(user).Error; err != nil {
			if errors.Is(err, gorm.ErrDuplicatedKey) {
				return platform.Err(409, "admin_email_unavailable", "The administrator email already belongs to another account.")
			}
			return err
		}
		created = true
		return nil
	})
	return created, err
}
