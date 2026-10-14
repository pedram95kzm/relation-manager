package auth

import (
	"context"
	"errors"
	"net/mail"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
	"relations/internal/config"
	"relations/internal/database"
	"relations/internal/platform"
)

type User = database.User

type Service struct {
	cfg        config.Config
	repo       *Repository
	now        func() time.Time
	dummyHash  []byte
	google     GoogleProvider
	loginLimit *platform.Limiter
}

func New(cfg config.Config, db *gorm.DB) *Service {
	dummy, _ := bcrypt.GenerateFromPassword([]byte("unused-dummy-password-for-timing"), 12)
	return &Service{cfg: cfg, repo: &Repository{db: db}, now: time.Now, dummyHash: dummy, google: newGoogleProvider(cfg), loginLimit: platform.NewLimiter(10, 15*time.Minute)}
}

type RegisterInput struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	Name     string `json:"name"`
}
type LoginInput struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}
type ProfileInput struct {
	Name   string `json:"name"`
	Age    int    `json:"age"`
	Gender string `json:"gender"`
}
type PasswordInput struct {
	CurrentPassword string `json:"current_password"`
	NewPassword     string `json:"new_password"`
}

func normalizeEmail(input string) (string, error) {
	email := strings.ToLower(strings.TrimSpace(input))
	parsed, err := mail.ParseAddress(email)
	if err != nil || len(email) > 254 || parsed.Address != email || parsed.Name != "" {
		return "", platform.Err(400, "invalid_email", "Enter a valid email address.")
	}
	return email, nil
}

func validName(name string) bool {
	return utf8.ValidString(name) && utf8.RuneCountInString(name) >= 1 && utf8.RuneCountInString(name) <= 120 && !strings.ContainsAny(name, "\x00\r\n")
}

func validPassword(password string) bool {
	return utf8.ValidString(password) && len(password) >= 12 && len(password) <= 72 && strings.TrimSpace(password) != ""
}

func (s *Service) Register(ctx context.Context, input RegisterInput) (SessionTokens, error) {
	email, err := normalizeEmail(input.Email)
	if err != nil {
		return SessionTokens{}, err
	}
	input.Name = strings.TrimSpace(input.Name)
	if !validName(input.Name) || !validPassword(input.Password) {
		return SessionTokens{}, platform.Err(400, "invalid_account", "Use a name of 1–120 characters and a password of 12–72 bytes.")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(input.Password), 12)
	if err != nil {
		return SessionTokens{}, err
	}
	user := User{ID: uuid.NewString(), Email: email, Name: input.Name, PasswordHash: string(hash), Role: "user", AuthVersion: 1}
	raw, session, err := newRefresh(user.ID, uuid.NewString(), s.now(), s.cfg.RefreshTTL)
	if err != nil {
		return SessionTokens{}, err
	}
	if err = s.repo.createAccount(ctx, &user, &session); err != nil {
		return SessionTokens{}, err
	}
	return s.signSession(user, session.ID, raw)
}

func (s *Service) Login(ctx context.Context, input LoginInput) (SessionTokens, error) {
	email, validationErr := normalizeEmail(input.Email)
	if !s.loginLimit.Allow(hashToken(email)) {
		return SessionTokens{}, platform.Err(429, "rate_limited", "Too many requests. Try again later.")
	}
	user, lookupErr := s.repo.userByEmail(ctx, email)
	hash := s.dummyHash
	if lookupErr == nil && user.PasswordHash != "" {
		hash = []byte(user.PasswordHash)
	}
	password := input.Password
	if !validPassword(password) {
		password = "invalid-password-placeholder"
	}
	comparisonErr := bcrypt.CompareHashAndPassword(hash, []byte(password))
	if lookupErr != nil && !errors.Is(lookupErr, gorm.ErrRecordNotFound) {
		return SessionTokens{}, lookupErr
	}
	if validationErr != nil || lookupErr != nil || comparisonErr != nil || user.Disabled || user.PasswordHash == "" || !validPassword(input.Password) {
		return SessionTokens{}, invalidCredentials()
	}
	return s.issueSession(ctx, user.ID, user.PasswordHash)
}

func (s *Service) IssueSession(ctx context.Context, userID string) (SessionTokens, error) {
	return s.issueSession(ctx, userID, "")
}

func (s *Service) issueSession(ctx context.Context, userID, expectedHash string) (SessionTokens, error) {
	if _, err := uuid.Parse(userID); err != nil {
		return SessionTokens{}, invalidCredentials()
	}
	raw, session, err := newRefresh(userID, uuid.NewString(), s.now(), s.cfg.RefreshTTL)
	if err != nil {
		return SessionTokens{}, err
	}
	user, err := s.repo.createSession(ctx, userID, expectedHash, &session)
	if err != nil {
		return SessionTokens{}, err
	}
	return s.signSession(user, session.ID, raw)
}

func (s *Service) Refresh(ctx context.Context, raw string) (SessionTokens, error) {
	if len(raw) != 43 {
		return SessionTokens{}, invalidCredentials()
	}
	nextRaw, next, err := newRefresh("", "", s.now(), s.cfg.RefreshTTL)
	if err != nil {
		return SessionTokens{}, err
	}
	user, err := s.repo.rotate(ctx, hashToken(raw), &next, s.now())
	if err != nil {
		return SessionTokens{}, err
	}
	return s.signSession(user, next.ID, nextRaw)
}

func (s *Service) Logout(ctx context.Context, raw string) error {
	if len(raw) != 43 {
		return nil
	}
	return s.repo.revokeFamily(ctx, hashToken(raw), s.now())
}

func (s *Service) LogoutAll(ctx context.Context, userID string) error {
	return s.repo.revokeAll(ctx, userID, s.now())
}

func (s *Service) ProfileComplete(user User) bool {
	return user.Age != nil && *user.Age >= s.cfg.MinAge && *user.Age <= 120 && user.Gender != ""
}

func (s *Service) UpdateProfile(ctx context.Context, userID string, input ProfileInput) (User, error) {
	input.Name = strings.TrimSpace(input.Name)
	if !validName(input.Name) {
		return User{}, platform.Err(400, "invalid_name", "Name must contain 1–120 characters.")
	}
	if input.Age < s.cfg.MinAge || input.Age > 120 {
		return User{}, platform.Err(400, "age_ineligible", "Age does not meet this service's eligibility policy.")
	}
	switch input.Gender {
	case "female", "male", "nonbinary", "other", "prefer_not_to_say":
	default:
		return User{}, platform.Err(400, "invalid_gender", "Select a supported gender option.")
	}
	return s.repo.profile(ctx, userID, input)
}

func (s *Service) ChangePassword(ctx context.Context, userID string, input PasswordInput) error {
	if !validPassword(input.NewPassword) {
		return platform.Err(400, "invalid_password", "New password must contain 12–72 bytes.")
	}
	user, err := s.repo.userByID(ctx, userID)
	if err != nil || user.Disabled {
		return invalidCredentials()
	}
	if user.PasswordHash == "" {
		return platform.Err(400, "password_account_required", "Password changes require an email/password account.")
	}
	if !validPassword(input.CurrentPassword) || bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(input.CurrentPassword)) != nil {
		return invalidCredentials()
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(input.NewPassword), 12)
	if err != nil {
		return err
	}
	return s.repo.changePassword(ctx, user.ID, user.PasswordHash, string(hash), s.now())
}

func (s *Service) DeleteAccount(ctx context.Context, userID, password string) error {
	user, err := s.repo.userByID(ctx, userID)
	if err != nil || user.Disabled {
		return invalidCredentials()
	}
	if user.PasswordHash == "" {
		return platform.Err(400, "reauthentication_unavailable", "Google-only account deletion requires a recent Google reauthentication procedure; contact the administrator.")
	}
	if !validPassword(password) || bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)) != nil {
		return invalidCredentials()
	}
	return s.repo.deleteAccount(ctx, user.ID, user.PasswordHash)
}

func (s *Service) GoogleAccount(ctx context.Context, subject, email, name string) (User, error) {
	if subject == "" || len(subject) > 255 {
		return User{}, invalidCredentials()
	}
	normalized, err := normalizeEmail(email)
	if err != nil {
		return User{}, invalidCredentials()
	}
	name = strings.TrimSpace(name)
	if !validName(name) {
		name = "Account"
	}
	return s.repo.googleAccount(ctx, subject, normalized, name)
}

func (s *Service) SeedAdmin(ctx context.Context) (bool, error) {
	if s.cfg.Environment == "production" && (s.cfg.AdminPassword == config.DefaultAdminPassword || s.cfg.AdminPassword == config.DefaultUserPassword) {
		return false, platform.Err(400, "public_default_credentials", "Production administrator initialization requires a private password.")
	}
	var count int64
	if err := s.repo.db.WithContext(ctx).Model(&User{}).Where("role = 'admin'").Count(&count).Error; err != nil {
		return false, err
	}
	if count != 0 {
		return false, nil
	}
	email, err := normalizeEmail(s.cfg.AdminEmail)
	if err != nil || !validPassword(s.cfg.AdminPassword) {
		return false, platform.Err(400, "invalid_admin_configuration", "Set ADMIN_EMAIL and an ADMIN_PASSWORD of 12–72 bytes before seeding.")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(s.cfg.AdminPassword), 12)
	if err != nil {
		return false, err
	}
	user := User{ID: uuid.NewString(), Email: email, Name: "Administrator", PasswordHash: string(hash), Role: "admin", AuthVersion: 1}
	return s.repo.seedAdmin(ctx, &user)
}
