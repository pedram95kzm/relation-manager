package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"relations/internal/database"
)

const tokenIssuer = "relations-manager"
const tokenAudience = "relations-web"

type accessClaims struct {
	SessionID   string `json:"sid"`
	AuthVersion int64  `json:"ver"`
	jwt.RegisteredClaims
}

type SessionTokens struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"-"`
	ExpiresIn    int64  `json:"expires_in"`
	User         User   `json:"user"`
}

func hashToken(raw string) string { return fmt.Sprintf("%x", sha256.Sum256([]byte(raw))) }

func newRefresh(userID, familyID string, now time.Time, ttl time.Duration) (string, database.RefreshSession, error) {
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		return "", database.RefreshSession{}, err
	}
	raw := base64.RawURLEncoding.EncodeToString(secret)
	return raw, database.RefreshSession{ID: uuid.NewString(), UserID: userID, FamilyID: familyID, TokenHash: hashToken(raw), ExpiresAt: now.Add(ttl), CreatedAt: now}, nil
}

func (s *Service) signSession(user User, sessionID, refresh string) (SessionTokens, error) {
	now := s.now()
	claims := accessClaims{SessionID: sessionID, AuthVersion: user.AuthVersion, RegisteredClaims: jwt.RegisteredClaims{
		Issuer: tokenIssuer, Audience: jwt.ClaimStrings{tokenAudience}, Subject: user.ID, ID: uuid.NewString(),
		IssuedAt: jwt.NewNumericDate(now), NotBefore: jwt.NewNumericDate(now), ExpiresAt: jwt.NewNumericDate(now.Add(s.cfg.AccessTTL)),
	}}
	access, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(s.cfg.JWTSecret))
	if err != nil {
		return SessionTokens{}, err
	}
	return SessionTokens{AccessToken: access, RefreshToken: refresh, ExpiresIn: int64(s.cfg.AccessTTL.Seconds()), User: user}, nil
}

func (s *Service) Authenticate(ctx context.Context, raw string) (User, error) {
	if len(raw) == 0 || len(raw) > 4096 {
		return User{}, invalidCredentials()
	}
	claims := &accessClaims{}
	token, err := jwt.ParseWithClaims(raw, claims, func(token *jwt.Token) (any, error) {
		if token.Method != jwt.SigningMethodHS256 {
			return nil, invalidCredentials()
		}
		return []byte(s.cfg.JWTSecret), nil
	}, jwt.WithValidMethods([]string{"HS256"}), jwt.WithIssuer(tokenIssuer), jwt.WithAudience(tokenAudience), jwt.WithExpirationRequired(), jwt.WithIssuedAt(), jwt.WithTimeFunc(s.now))
	if err != nil || !token.Valid || claims.AuthVersion <= 0 || claims.IssuedAt == nil || claims.NotBefore == nil || claims.ID == "" {
		return User{}, invalidCredentials()
	}
	if _, err = uuid.Parse(claims.Subject); err != nil {
		return User{}, invalidCredentials()
	}
	if _, err = uuid.Parse(claims.SessionID); err != nil {
		return User{}, invalidCredentials()
	}
	return s.repo.authenticatedUser(ctx, claims.Subject, claims.SessionID, claims.AuthVersion, s.now())
}
