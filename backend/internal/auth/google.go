package auth

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/mail"
	"strings"
	"time"
	"unicode"

	"cloud.google.com/go/auth/credentials/idtoken"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"gorm.io/gorm"
	"relations/internal/config"
	"relations/internal/database"
	"relations/internal/platform"
)

const GoogleStateTTL = 5 * time.Minute

// GoogleProvider verifies the provider's signature before returning claims.
// The interface permits offline tests without any Google credentials.
type GoogleProvider interface {
	AuthorizationURL(state, nonce, verifier string) string
	ExchangeIdentity(ctx context.Context, code, verifier string) (GoogleClaims, error)
}

type GoogleClaims struct {
	Issuer, Audience, Subject, Nonce, Email, Name, AuthorizedParty string
	Expires, IssuedAt                                              int64
	EmailVerified                                                  bool
}

type GoogleStart struct {
	State, AuthorizationURL string
}

type googleProvider struct {
	config    oauth2.Config
	client    *http.Client
	validator *idtoken.Validator
	initErr   error
}

func newGoogleProvider(cfg config.Config) GoogleProvider {
	client := &http.Client{Timeout: 10 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}}
	// Disable SDK debug logging even if GOOGLE_SDK_GO_LOGGING_LEVEL is set:
	// provider responses contain tokens and must never enter application logs.
	validator, err := idtoken.NewValidator(&idtoken.ValidatorOptions{Client: client, Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
	return &googleProvider{
		config: oauth2.Config{ClientID: cfg.GoogleClientID, ClientSecret: cfg.GoogleClientSecret,
			RedirectURL: cfg.GoogleCallbackURL, Scopes: []string{"openid", "email", "profile"}, Endpoint: google.Endpoint},
		client: client, validator: validator, initErr: err,
	}
}

func (p *googleProvider) AuthorizationURL(state, nonce, verifier string) string {
	return p.config.AuthCodeURL(state, oauth2.S256ChallengeOption(verifier), oauth2.SetAuthURLParam("nonce", nonce), oauth2.AccessTypeOnline)
}

func (p *googleProvider) ExchangeIdentity(ctx context.Context, code, verifier string) (GoogleClaims, error) {
	if p.initErr != nil {
		return GoogleClaims{}, p.initErr
	}
	ctx = context.WithValue(ctx, oauth2.HTTPClient, p.client)
	token, err := p.config.Exchange(ctx, code, oauth2.VerifierOption(verifier))
	if err != nil {
		return GoogleClaims{}, err
	}
	raw, ok := token.Extra("id_token").(string)
	if !ok || raw == "" || len(raw) > 16384 {
		return GoogleClaims{}, errors.New("missing identity token")
	}
	// Validate verifies the signature against Google's JWKs, audience and expiry;
	// application claims and the request nonce are checked by the service below.
	payload, err := p.validator.Validate(ctx, raw, p.config.ClientID)
	if err != nil {
		return GoogleClaims{}, err
	}
	claimString := func(key string) string { value, _ := payload.Claims[key].(string); return value }
	verified, _ := payload.Claims["email_verified"].(bool)
	return GoogleClaims{Issuer: payload.Issuer, Audience: payload.Audience, Subject: payload.Subject,
		Expires: payload.Expires, IssuedAt: payload.IssuedAt, Nonce: claimString("nonce"),
		Email: claimString("email"), Name: claimString("name"), AuthorizedParty: claimString("azp"), EmailVerified: verified}, nil
}

func (s *Service) GoogleEnabled() bool {
	return s.google != nil && s.cfg.GoogleClientID != "" && s.cfg.GoogleClientSecret != "" && s.cfg.GoogleCallbackURL != ""
}

func (s *Service) googleFlow() googleFlow {
	return googleFlow{provider: s.google, store: postgresGoogleStates{db: s.repo.db}, audience: s.cfg.GoogleClientID,
		now: s.now, account: s.GoogleAccount, issue: s.IssueSession}
}

func (s *Service) BeginGoogle(ctx context.Context) (GoogleStart, error) {
	if !s.GoogleEnabled() {
		return GoogleStart{}, platform.Err(503, "google_unavailable", "Google sign-in is not configured.")
	}
	return s.googleFlow().begin(ctx)
}

func (s *Service) CompleteGoogle(ctx context.Context, state, cookieState, code, providerError string) (SessionTokens, error) {
	if !s.GoogleEnabled() {
		return SessionTokens{}, platform.Err(503, "google_unavailable", "Google sign-in is not configured.")
	}
	return s.googleFlow().complete(ctx, state, cookieState, code, providerError)
}

type googleStateStore interface {
	Create(context.Context, database.OAuthState) error
	Consume(context.Context, string, time.Time) (database.OAuthState, error)
}

type postgresGoogleStates struct{ db *gorm.DB }

func (p postgresGoogleStates) Create(ctx context.Context, state database.OAuthState) error {
	return p.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("expires_at <= ?", state.CreatedAt).Delete(&database.OAuthState{}).Error; err != nil {
			return err
		}
		return tx.Create(&state).Error
	})
}

func (p postgresGoogleStates) Consume(ctx context.Context, hash string, now time.Time) (database.OAuthState, error) {
	var state database.OAuthState
	// A single committed UPDATE consumes the state before contacting Google.
	// Concurrent callbacks cannot both redeem it, including failed exchanges.
	result := p.db.WithContext(ctx).Raw(`UPDATE oauth_states SET used_at = ?
		WHERE state_hash = ? AND used_at IS NULL AND expires_at > ? RETURNING *`, now, hash, now).Scan(&state)
	if result.Error != nil {
		return state, result.Error
	}
	if result.RowsAffected != 1 {
		return state, googleStateError()
	}
	return state, nil
}

type googleFlow struct {
	provider GoogleProvider
	store    googleStateStore
	audience string
	now      func() time.Time
	account  func(context.Context, string, string, string) (User, error)
	issue    func(context.Context, string) (SessionTokens, error)
}

func (g googleFlow) begin(ctx context.Context) (GoogleStart, error) {
	now := g.now().UTC()
	state, nonce, verifier := oauth2.GenerateVerifier(), oauth2.GenerateVerifier(), oauth2.GenerateVerifier()
	record := database.OAuthState{StateHash: googleHash(state), Verifier: verifier, NonceHash: googleHash(nonce),
		CreatedAt: now, ExpiresAt: now.Add(GoogleStateTTL)}
	if err := g.store.Create(ctx, record); err != nil {
		return GoogleStart{}, err
	}
	return GoogleStart{State: state, AuthorizationURL: g.provider.AuthorizationURL(state, nonce, verifier)}, nil
}

func (g googleFlow) complete(ctx context.Context, state, cookieState, code, providerError string) (SessionTokens, error) {
	if !googleRandomValue(state) || !googleRandomValue(cookieState) || subtle.ConstantTimeCompare([]byte(state), []byte(cookieState)) != 1 {
		return SessionTokens{}, googleStateError()
	}
	record, err := g.store.Consume(ctx, googleHash(state), g.now().UTC())
	if err != nil {
		return SessionTokens{}, err
	}
	if providerError == "access_denied" {
		return SessionTokens{}, platform.Err(401, "oauth_cancelled", "Google sign-in was cancelled.")
	}
	if providerError != "" || code == "" || len(code) > 4096 {
		return SessionTokens{}, googleFailedError()
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	claims, err := g.provider.ExchangeIdentity(ctx, code, record.Verifier)
	if err != nil {
		// Provider errors can embed tokens, codes, or response bodies. Discard them.
		return SessionTokens{}, googleFailedError()
	}
	email, name, err := validateGoogleClaims(claims, g.audience, record.NonceHash, g.now().UTC())
	if err != nil {
		return SessionTokens{}, err
	}
	user, err := g.account(ctx, claims.Subject, email, name)
	if err != nil {
		return SessionTokens{}, err
	}
	return g.issue(ctx, user.ID)
}

func validateGoogleClaims(claims GoogleClaims, audience, nonceHash string, now time.Time) (string, string, error) {
	fail := func() (string, string, error) {
		return "", "", platform.Err(401, "oauth_identity_invalid", "Google identity could not be verified.")
	}
	if (claims.Issuer != "https://accounts.google.com" && claims.Issuer != "accounts.google.com") ||
		audience == "" || claims.Audience != audience || claims.Expires <= now.Unix() || claims.IssuedAt <= 0 ||
		claims.IssuedAt > now.Add(time.Minute).Unix() || claims.Expires <= claims.IssuedAt ||
		(claims.AuthorizedParty != "" && claims.AuthorizedParty != audience) || !claims.EmailVerified ||
		!googleRandomValue(claims.Nonce) || subtle.ConstantTimeCompare([]byte(googleHash(claims.Nonce)), []byte(nonceHash)) != 1 {
		return fail()
	}
	if len(claims.Subject) < 1 || len(claims.Subject) > 255 {
		return fail()
	}
	for _, c := range claims.Subject {
		if c < 0x21 || c > 0x7e {
			return fail()
		}
	}
	email := strings.TrimSpace(claims.Email)
	address, err := mail.ParseAddress(email)
	if err != nil || len(email) > 254 || address.Address != email || strings.ContainsAny(email, "\r\n") {
		return fail()
	}
	name := strings.TrimSpace(strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, claims.Name))
	if name == "" {
		name = "Google user"
	}
	if runes := []rune(name); len(runes) > 120 {
		name = string(runes[:120])
	}
	return strings.ToLower(email), name, nil
}

func googleHash(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func googleRandomValue(value string) bool {
	if len(value) != 43 {
		return false
	}
	decoded, err := base64.RawURLEncoding.DecodeString(value)
	return err == nil && len(decoded) == 32
}

func googleStateError() error {
	return platform.Err(401, "oauth_state_invalid", "Google sign-in expired or could not be verified. Please start again.")
}

func googleFailedError() error {
	return platform.Err(401, "oauth_failed", "Google sign-in could not be completed. Please start again.")
}
