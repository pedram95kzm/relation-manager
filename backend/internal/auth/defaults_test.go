package auth

import (
	"context"
	"sync"
	"testing"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
	"relations/internal/config"
)

func defaultTestConfig() config.Config {
	cfg := authTestConfig()
	cfg.SeedDefaultAccounts = true
	cfg.AdminEmail, cfg.AdminPassword = "admin@relations.local", "RelationsAdmin2026!"
	cfg.UserEmail, cfg.UserPassword = "user@relations.local", "RelationsUser2026!"
	return cfg
}

func TestSeedDefaultsDisabledAndProductionPasswordPolicy(t *testing.T) {
	ctx := context.Background()
	// Disabled initialization must not contact a database or inspect credentials.
	if err := (&Service{cfg: config.Config{Environment: "production"}}).SeedDefaults(ctx); err != nil {
		t.Fatal(err)
	}
	for _, role := range []string{"admin", "user"} {
		for _, public := range []string{"RelationsAdmin2026!", "RelationsUser2026!"} {
			cfg := defaultTestConfig()
			cfg.Environment = "production"
			cfg.AdminPassword, cfg.UserPassword = "private-administrator-password", "private-ordinary-user-password"
			if role == "admin" {
				cfg.AdminPassword = public
			} else {
				cfg.UserPassword = public
			}
			err := (&Service{cfg: cfg}).SeedDefaults(ctx)
			expectCode(t, err, "public_default_credentials")
		}
	}
}

func TestSeedDefaultsCreatesEligibleAccountsAndPreservesChanges(t *testing.T) {
	db := authTestDB(t)
	cfg := defaultTestConfig()
	cfg.MinAge = 42
	service := New(cfg, db)
	ctx := context.Background()
	if err := service.SeedDefaults(ctx); err != nil {
		t.Fatal(err)
	}
	for _, input := range []LoginInput{{Email: cfg.AdminEmail, Password: cfg.AdminPassword}, {Email: cfg.UserEmail, Password: cfg.UserPassword}} {
		session, err := service.Login(ctx, input)
		if err != nil {
			t.Fatal(err)
		}
		user := session.User
		if user.Age == nil || *user.Age != 42 || user.Gender != "prefer_not_to_say" || !service.ProfileComplete(user) {
			t.Fatal("default profile is ineligible", user.ID)
		}
		if cost, err := bcrypt.Cost([]byte(user.PasswordHash)); err != nil || cost != 12 {
			t.Fatal("default password must use bcrypt cost 12", err)
		}
	}
	var ordinary User
	if err := db.First(&ordinary, "role = 'user'").Error; err != nil {
		t.Fatal(err)
	}
	changedHash, err := bcrypt.GenerateFromPassword([]byte("a-new-private-user-password"), 12)
	if err != nil {
		t.Fatal(err)
	}
	if err = db.Model(&ordinary).Updates(map[string]any{"password_hash": string(changedHash), "name": "Renamed account", "disabled": true, "auth_version": 8}).Error; err != nil {
		t.Fatal(err)
	}
	// Reinitialization must remain harmless even after credentials/config change.
	service.cfg.AdminEmail, service.cfg.AdminPassword = "invalid", "short"
	service.cfg.UserEmail, service.cfg.UserPassword = "invalid", "short"
	if err = service.SeedDefaults(ctx); err != nil {
		t.Fatal(err)
	}
	var after User
	if err = db.First(&after, "id = ?", ordinary.ID).Error; err != nil {
		t.Fatal(err)
	}
	if after.PasswordHash != string(changedHash) || after.Name != "Renamed account" || !after.Disabled || after.AuthVersion != 8 || after.Role != "user" {
		t.Fatal("initialization changed an existing account")
	}
	var count int64
	if err = db.Model(&User{}).Count(&count).Error; err != nil || count != 2 {
		t.Fatal("initialization created duplicate accounts", count, err)
	}
}

func TestSeedDefaultsCreatesOnlyMissingRoles(t *testing.T) {
	for _, existingRole := range []string{"admin", "user"} {
		t.Run(existingRole, func(t *testing.T) {
			db := authTestDB(t)
			cfg := defaultTestConfig()
			age := 55
			existing := User{ID: uuid.NewString(), Email: "existing-" + existingRole + "@example.test", Name: "Existing account", Role: existingRole,
				Age: &age, Gender: "other", Disabled: true, AuthVersion: 4}
			if err := db.Create(&existing).Error; err != nil {
				t.Fatal(err)
			}
			service := New(cfg, db)
			if err := service.SeedDefaults(context.Background()); err != nil {
				t.Fatal(err)
			}
			var users []User
			if err := db.Find(&users).Error; err != nil || len(users) != 2 {
				t.Fatal("missing-role initialization failed", len(users), err)
			}
			for _, user := range users {
				if user.ID == existing.ID && (user.Role != existingRole || user.Name != existing.Name || !user.Disabled || user.AuthVersion != 4 || user.Age == nil || *user.Age != age) {
					t.Fatal("existing role was changed")
				}
			}
		})
	}
}

func TestSeedDefaultsEmailConflictsNeverPromoteAccounts(t *testing.T) {
	for _, role := range []string{"admin", "user"} {
		t.Run(role, func(t *testing.T) {
			db := authTestDB(t)
			cfg := defaultTestConfig()
			otherRole, email := "user", cfg.AdminEmail
			if role == "user" {
				otherRole, email = "admin", cfg.UserEmail
			}
			existing := User{ID: uuid.NewString(), Email: email, Name: "Existing owner", Role: otherRole, AuthVersion: 1}
			if err := db.Create(&existing).Error; err != nil {
				t.Fatal(err)
			}
			err := New(cfg, db).SeedDefaults(context.Background())
			expectCode(t, err, "default_email_unavailable")
			var after User
			if err := db.First(&after, "id = ?", existing.ID).Error; err != nil || after.Role != otherRole || after.PasswordHash != "" {
				t.Fatal("initialization promoted or replaced the email owner", err)
			}
			var count int64
			if err := db.Model(&User{}).Count(&count).Error; err != nil || count != 1 {
				t.Fatal("conflicting initialization was not atomic", count, err)
			}
		})
	}
}

func TestSeedDefaultsBothAccountsCommitTogether(t *testing.T) {
	db := authTestDB(t)
	cfg := defaultTestConfig()
	cfg.UserEmail = cfg.AdminEmail
	err := New(cfg, db).SeedDefaults(context.Background())
	expectCode(t, err, "default_email_unavailable")
	var count int64
	if err := db.Model(&User{}).Count(&count).Error; err != nil || count != 0 {
		t.Fatal("a partial initialization committed", count, err)
	}
}

func TestSeedDefaultsConcurrentCallsAreIdempotent(t *testing.T) {
	db := authTestDB(t)
	service := New(defaultTestConfig(), db)
	const callers = 8
	start := make(chan struct{})
	errors := make(chan error, callers)
	var workers sync.WaitGroup
	for i := 0; i < callers; i++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			<-start
			errors <- service.SeedDefaults(context.Background())
		}()
	}
	close(start)
	workers.Wait()
	close(errors)
	for err := range errors {
		if err != nil {
			t.Fatal(err)
		}
	}
	for _, role := range []string{"admin", "user"} {
		var count int64
		if err := db.Model(&User{}).Where("role = ?", role).Count(&count).Error; err != nil || count != 1 {
			t.Fatal("concurrent initialization duplicated a role", role, count, err)
		}
	}
}
