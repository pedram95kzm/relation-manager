package auth

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"cloud.google.com/go/auth/credentials/idtoken"
	"golang.org/x/oauth2"
	"relations/internal/config"
	"relations/internal/database"
	"relations/internal/platform"
)

type memoryGoogleStates struct {
	mu     sync.Mutex
	states map[string]database.OAuthState
}

func (m *memoryGoogleStates) Create(_ context.Context, state database.OAuthState) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.states[state.StateHash] = state
	return nil
}

func (m *memoryGoogleStates) Consume(_ context.Context, hash string, now time.Time) (database.OAuthState, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	state, ok := m.states[hash]
	if !ok || state.UsedAt != nil || !state.ExpiresAt.After(now) {
		return database.OAuthState{}, googleStateError()
	}
	state.UsedAt = &now
	m.states[hash] = state
	return state, nil
}

type fakeGoogleProvider struct {
	claims                GoogleClaims
	nonce, verifier, code string
	err                   error
	count                 atomic.Int32
	deadline              bool
}

func (f *fakeGoogleProvider) AuthorizationURL(state, nonce, verifier string) string {
	f.nonce, f.verifier, f.claims.Nonce = nonce, verifier, nonce
	return "https://accounts.google.com/auth?state=" + state
}

func (f *fakeGoogleProvider) ExchangeIdentity(ctx context.Context, code, verifier string) (GoogleClaims, error) {
	f.count.Add(1)
	f.code = code
	_, f.deadline = ctx.Deadline()
	if verifier != f.verifier {
		return GoogleClaims{}, errors.New("incorrect verifier")
	}
	return f.claims, f.err
}

type googleHarness struct {
	flow     googleFlow
	provider *fakeGoogleProvider
	store    *memoryGoogleStates
	now      time.Time
	accounts atomic.Int32
}

func newGoogleHarness() *googleHarness {
	h := &googleHarness{now: time.Date(2026, 10, 11, 9, 0, 0, 0, time.UTC)}
	h.provider = &fakeGoogleProvider{claims: GoogleClaims{Issuer: "https://accounts.google.com", Audience: "test-client",
		Subject: "google-subject-123", Email: "Verified@Example.com", Name: "Verified User", EmailVerified: true,
		Expires: h.now.Add(time.Hour).Unix(), IssuedAt: h.now.Unix()}}
	h.store = &memoryGoogleStates{states: make(map[string]database.OAuthState)}
	h.flow = googleFlow{provider: h.provider, store: h.store, audience: "test-client", now: func() time.Time { return h.now },
		account: func(_ context.Context, subject, email, name string) (User, error) {
			h.accounts.Add(1)
			if subject != "google-subject-123" || email != "verified@example.com" || name != "Verified User" {
				return User{}, errors.New("wrong verified identity")
			}
			return User{ID: "account-id"}, nil
		}, issue: func(_ context.Context, id string) (SessionTokens, error) {
			return SessionTokens{AccessToken: "short-lived-access", RefreshToken: "private-refresh", User: User{ID: id}}, nil
		}}
	return h
}

func requireGoogleError(t *testing.T, err error, code string) {
	t.Helper()
	var appErr *platform.Error
	if !errors.As(err, &appErr) || appErr.Code != code {
		t.Fatalf("wanted %s, got %v", code, err)
	}
}

func TestGoogleStartStoresHashesAndPKCE(t *testing.T) {
	h := newGoogleHarness()
	first, err := h.flow.begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	state := h.store.states[googleHash(first.State)]
	if !googleRandomValue(first.State) || !googleRandomValue(state.Verifier) || state.StateHash == first.State ||
		state.NonceHash != googleHash(h.provider.nonce) || state.NonceHash == h.provider.nonce || state.ExpiresAt.Sub(state.CreatedAt) != GoogleStateTTL {
		t.Fatal("state, nonce or verifier storage is unsafe")
	}
	second, err := h.flow.begin(context.Background())
	if err != nil || first.State == second.State || state.Verifier == h.provider.verifier {
		t.Fatal("authorization requests must use fresh randomness")
	}
	provider := newGoogleProvider(config.Config{GoogleClientID: "test-client", GoogleCallbackURL: "https://app.example/api/v1/auth/google/callback"})
	parsed, err := url.Parse(provider.AuthorizationURL("state", "nonce", state.Verifier))
	if err != nil {
		t.Fatal(err)
	}
	query := parsed.Query()
	if parsed.Host != "accounts.google.com" || query.Get("code_challenge_method") != "S256" ||
		query.Get("code_challenge") != oauth2.S256ChallengeFromVerifier(state.Verifier) || query.Get("nonce") != "nonce" ||
		query.Get("scope") != "openid email profile" || query.Get("redirect_uri") != "https://app.example/api/v1/auth/google/callback" || query.Get("access_type") != "online" {
		t.Fatalf("incorrect Google authorization parameters: %s", parsed.Redacted())
	}
}

func TestGoogleFlowCookieBindingAndSingleUse(t *testing.T) {
	h := newGoogleHarness()
	start, _ := h.flow.begin(context.Background())
	for _, cookie := range []string{"", "invalid", oauth2.GenerateVerifier()} {
		_, err := h.flow.complete(context.Background(), start.State, cookie, "code", "")
		requireGoogleError(t, err, "oauth_state_invalid")
	}
	if h.provider.count.Load() != 0 || h.store.states[googleHash(start.State)].UsedAt != nil {
		t.Fatal("an unbound callback contacted Google or consumed another browser's state")
	}
	result, err := h.flow.complete(context.Background(), start.State, start.State, "code", "")
	if err != nil || result.User.ID != "account-id" || result.RefreshToken == "" || h.accounts.Load() != 1 || !h.provider.deadline {
		t.Fatal("verified login did not issue a bounded session", result, err)
	}
	_, err = h.flow.complete(context.Background(), start.State, start.State, "code", "")
	requireGoogleError(t, err, "oauth_state_invalid")
	if h.provider.count.Load() != 1 {
		t.Fatal("state replay contacted provider")
	}
}

func TestGoogleExpiredStateRejectedBeforeExchange(t *testing.T) {
	h := newGoogleHarness()
	start, _ := h.flow.begin(context.Background())
	h.now = h.now.Add(GoogleStateTTL)
	_, err := h.flow.complete(context.Background(), start.State, start.State, "code", "")
	requireGoogleError(t, err, "oauth_state_invalid")
	if h.provider.count.Load() != 0 || h.accounts.Load() != 0 {
		t.Fatal("expired state must not authenticate")
	}
}

func TestGoogleCancellationAndProviderFailureConsumeState(t *testing.T) {
	for _, test := range []struct{ providerError, code, want string }{
		{"access_denied", "", "oauth_cancelled"}, {"untrusted-provider-description", "", "oauth_failed"}, {"", "", "oauth_failed"}, {"", "code", "oauth_failed"},
	} {
		t.Run(test.providerError+test.code, func(t *testing.T) {
			h := newGoogleHarness()
			h.provider.err = errors.New("provider secret-token-and-authorization-code")
			start, _ := h.flow.begin(context.Background())
			_, err := h.flow.complete(context.Background(), start.State, start.State, test.code, test.providerError)
			requireGoogleError(t, err, test.want)
			if strings.Contains(err.Error(), "secret") || h.accounts.Load() != 0 {
				t.Fatal("provider failure exposed details or created account")
			}
			_, err = h.flow.complete(context.Background(), start.State, start.State, "code", "")
			requireGoogleError(t, err, "oauth_state_invalid")
		})
	}
}

func TestGoogleConcurrentCallbacksOnlyOneSucceeds(t *testing.T) {
	h := newGoogleHarness()
	start, _ := h.flow.begin(context.Background())
	var success atomic.Int32
	var wait sync.WaitGroup
	for range 8 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			if _, err := h.flow.complete(context.Background(), start.State, start.State, "code", ""); err == nil {
				success.Add(1)
			}
		}()
	}
	wait.Wait()
	if success.Load() != 1 || h.provider.count.Load() != 1 || h.accounts.Load() != 1 {
		t.Fatalf("single-use state violated: successes=%d, exchanges=%d", success.Load(), h.provider.count.Load())
	}
}

func TestGoogleNoUnsafeEmailLink(t *testing.T) {
	h := newGoogleHarness()
	h.flow.account = func(context.Context, string, string, string) (User, error) {
		return User{}, platform.Err(409, "account_link_required", "Use the existing account's sign-in method.")
	}
	h.flow.issue = func(context.Context, string) (SessionTokens, error) {
		t.Fatal("refused email linking must not issue a session")
		return SessionTokens{}, nil
	}
	start, _ := h.flow.begin(context.Background())
	_, err := h.flow.complete(context.Background(), start.State, start.State, "code", "")
	requireGoogleError(t, err, "account_link_required")
}

func TestGoogleClaimValidation(t *testing.T) {
	h := newGoogleHarness()
	_, _ = h.flow.begin(context.Background())
	valid := h.provider.claims
	for _, test := range []struct {
		name   string
		mutate func(*GoogleClaims)
	}{
		{"issuer", func(c *GoogleClaims) { c.Issuer = "https://attacker.example" }},
		{"audience", func(c *GoogleClaims) { c.Audience = "another-client" }},
		{"expiry", func(c *GoogleClaims) { c.Expires = h.now.Unix() }},
		{"future-issuance", func(c *GoogleClaims) { c.IssuedAt = h.now.Add(2 * time.Minute).Unix() }},
		{"missing-issuance", func(c *GoogleClaims) { c.IssuedAt = 0 }},
		{"authorized-party", func(c *GoogleClaims) { c.AuthorizedParty = "another-client" }},
		{"unverified-email", func(c *GoogleClaims) { c.EmailVerified = false }},
		{"missing-email", func(c *GoogleClaims) { c.Email = "" }},
		{"display-email", func(c *GoogleClaims) { c.Email = "Imposter <verified@example.com>" }},
		{"invalid-email", func(c *GoogleClaims) { c.Email = "bad-email" }},
		{"missing-subject", func(c *GoogleClaims) { c.Subject = "" }},
		{"oversize-subject", func(c *GoogleClaims) { c.Subject = strings.Repeat("a", 256) }},
		{"non-ascii-subject", func(c *GoogleClaims) { c.Subject = "شناسه" }},
		{"control-subject", func(c *GoogleClaims) { c.Subject = "subject\x00" }},
		{"missing-nonce", func(c *GoogleClaims) { c.Nonce = "" }},
		{"nonce-mismatch", func(c *GoogleClaims) { c.Nonce = oauth2.GenerateVerifier() }},
	} {
		t.Run(test.name, func(t *testing.T) {
			claims := valid
			test.mutate(&claims)
			_, _, err := validateGoogleClaims(claims, "test-client", googleHash(valid.Nonce), h.now)
			requireGoogleError(t, err, "oauth_identity_invalid")
		})
	}
	valid.Issuer = "accounts.google.com"
	email, name, err := validateGoogleClaims(valid, "test-client", googleHash(valid.Nonce), h.now)
	if err != nil || email != "verified@example.com" || name != "Verified User" {
		t.Fatal("valid claims rejected", err)
	}
}

func TestGoogleNonceMismatchNeverCreatesAccount(t *testing.T) {
	h := newGoogleHarness()
	start, _ := h.flow.begin(context.Background())
	h.provider.claims.Nonce = oauth2.GenerateVerifier()
	_, err := h.flow.complete(context.Background(), start.State, start.State, "code", "")
	requireGoogleError(t, err, "oauth_identity_invalid")
	if h.accounts.Load() != 0 {
		t.Fatal("unverified nonce created an account")
	}
}

func TestGoogleOptionalConfiguration(t *testing.T) {
	service := &Service{}
	if service.GoogleEnabled() {
		t.Fatal("Google must be unavailable without configuration")
	}
	_, err := service.BeginGoogle(context.Background())
	requireGoogleError(t, err, "google_unavailable")
	_, err = service.CompleteGoogle(context.Background(), "", "", "", "")
	requireGoogleError(t, err, "google_unavailable")
}

func TestGoogleOfficialProviderVerifiesSignatureAndPKCEOffline(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	verifier := oauth2.GenerateVerifier()
	claims := map[string]any{"iss": "https://accounts.google.com", "aud": "test-client", "sub": "google-subject",
		"iat": time.Now().Unix(), "exp": time.Now().Add(time.Hour).Unix(), "nonce": oauth2.GenerateVerifier(), "email": "user@example.com", "email_verified": true}
	identityToken := signedGoogleToken(t, key, claims)
	var tokenResponse atomic.Value
	var exchanges atomic.Int32
	mux := http.NewServeMux()
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		exchanges.Add(1)
		if r.Method != http.MethodPost || r.ParseForm() != nil || r.Form.Get("code") != "test-code" || r.Form.Get("code_verifier") != verifier ||
			r.Form.Get("grant_type") != "authorization_code" || r.Form.Get("redirect_uri") != "https://app.example/api/v1/auth/google/callback" {
			t.Error("OAuth exchange omitted bound PKCE or configured callback")
			w.WriteHeader(400)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"access_token":"provider-access","token_type":"Bearer","expires_in":3600,"id_token":%q}`, tokenResponse.Load().(string))
	})
	mux.HandleFunc("/certs", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "max-age=3600")
		_ = json.NewEncoder(w).Encode(map[string]any{"keys": []map[string]string{{"kid": "test-key", "alg": "RS256", "kty": "RSA", "use": "sig",
			"n": base64.RawURLEncoding.EncodeToString(key.N.Bytes()), "e": base64.RawURLEncoding.EncodeToString(big.NewInt(int64(key.E)).Bytes())}}})
	})
	server := httptest.NewServer(mux)
	defer server.Close()
	provider := newGoogleProvider(config.Config{GoogleClientID: "test-client", GoogleClientSecret: "test-client-secret", GoogleCallbackURL: "https://app.example/api/v1/auth/google/callback"}).(*googleProvider)
	provider.config.Endpoint.TokenURL = server.URL + "/token"
	provider.config.Endpoint.AuthStyle = oauth2.AuthStyleInParams
	provider.validator, err = idtoken.NewValidator(&idtoken.ValidatorOptions{Client: server.Client(), RS256CertsURL: server.URL + "/certs"})
	if err != nil {
		t.Fatal(err)
	}
	tokenResponse.Store(identityToken)
	verified, err := provider.ExchangeIdentity(context.Background(), "test-code", verifier)
	if err != nil || verified.Subject != "google-subject" || !verified.EmailVerified || verified.Nonce != claims["nonce"] {
		t.Fatal("official provider rejected valid offline signature", err)
	}
	parts := strings.Split(identityToken, ".")
	claims["sub"] = "forged-subject"
	encoded, _ := json.Marshal(claims)
	parts[1] = base64.RawURLEncoding.EncodeToString(encoded)
	tokenResponse.Store(strings.Join(parts, "."))
	if _, err = provider.ExchangeIdentity(context.Background(), "test-code", verifier); err == nil {
		t.Fatal("forged identity signature accepted")
	}
	claims["aud"] = "wrong-client"
	tokenResponse.Store(signedGoogleToken(t, key, claims))
	if _, err = provider.ExchangeIdentity(context.Background(), "test-code", verifier); err == nil {
		t.Fatal("wrong audience accepted by validator")
	}
	claims["aud"], claims["exp"] = "test-client", time.Now().Add(-time.Hour).Unix()
	tokenResponse.Store(signedGoogleToken(t, key, claims))
	if _, err = provider.ExchangeIdentity(context.Background(), "test-code", verifier); err == nil {
		t.Fatal("expired identity accepted by validator")
	}
	if exchanges.Load() != 4 {
		t.Fatal("unexpected exchange count", exchanges.Load())
	}
}

func signedGoogleToken(t *testing.T, key *rsa.PrivateKey, claims map[string]any) string {
	t.Helper()
	encoded, err := json.Marshal(claims)
	if err != nil {
		t.Fatal(err)
	}
	value := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"RS256","typ":"JWT","kid":"test-key"}`)) + "." + base64.RawURLEncoding.EncodeToString(encoded)
	hash := sha256.Sum256([]byte(value))
	signature, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, hash[:])
	if err != nil {
		t.Fatal(err)
	}
	return value + "." + base64.RawURLEncoding.EncodeToString(signature)
}

func TestGooglePostgresStateSingleUseAndCleanup(t *testing.T) {
	db := authTestDB(t)
	store := postgresGoogleStates{db: db}
	now := time.Now().UTC()
	state := database.OAuthState{StateHash: googleHash(oauth2.GenerateVerifier()), Verifier: oauth2.GenerateVerifier(),
		NonceHash: googleHash(oauth2.GenerateVerifier()), CreatedAt: now, ExpiresAt: now.Add(GoogleStateTTL)}
	if err := store.Create(context.Background(), state); err != nil {
		t.Fatal(err)
	}
	var success atomic.Int32
	var wait sync.WaitGroup
	for range 8 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			consumed, err := store.Consume(context.Background(), state.StateHash, now)
			if err == nil {
				if consumed.Verifier != state.Verifier || consumed.NonceHash != state.NonceHash || consumed.UsedAt == nil {
					t.Error("consumed state did not return its bound verifier/nonce")
				}
				success.Add(1)
			} else {
				requireGoogleError(t, err, "oauth_state_invalid")
			}
		}()
	}
	wait.Wait()
	if success.Load() != 1 {
		t.Fatal("database state was not atomically consumed", success.Load())
	}
	expired := state
	expired.StateHash = googleHash(oauth2.GenerateVerifier())
	expired.CreatedAt, expired.ExpiresAt = now.Add(-2*GoogleStateTTL), now
	if err := db.Create(&expired).Error; err != nil {
		t.Fatal(err)
	}
	_, err := store.Consume(context.Background(), expired.StateHash, now)
	requireGoogleError(t, err, "oauth_state_invalid")
	next := state
	next.StateHash = googleHash(oauth2.GenerateVerifier())
	if err = store.Create(context.Background(), next); err != nil {
		t.Fatal(err)
	}
	var expiredCount int64
	if err = db.Model(&database.OAuthState{}).Where("state_hash = ?", expired.StateHash).Count(&expiredCount).Error; err != nil || expiredCount != 0 {
		t.Fatal("expired OAuth state was not cleaned up", expiredCount, err)
	}
}

func TestGooglePostgresCompleteFlow(t *testing.T) {
	db := authTestDB(t)
	h := newGoogleHarness()
	cfg := authTestConfig()
	cfg.GoogleClientID, cfg.GoogleClientSecret = "test-client", "test-client-secret"
	cfg.GoogleCallbackURL = "https://app.example/api/v1/auth/google/callback"
	service := New(cfg, db)
	service.google, service.now = h.provider, h.flow.now
	start, err := service.BeginGoogle(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	first, err := service.CompleteGoogle(context.Background(), start.State, start.State, "code", "")
	if err != nil || first.AccessToken == "" || first.RefreshToken == "" || first.User.PasswordHash != "" || service.ProfileComplete(first.User) {
		t.Fatal("verified Google flow did not provision an incomplete-profile account/session", err)
	}
	var identity database.AuthIdentity
	if err = db.Where("provider = ? AND subject = ?", "google", h.provider.claims.Subject).First(&identity).Error; err != nil || identity.UserID != first.User.ID {
		t.Fatal("provider subject is not bound to provisioned user", err)
	}
	_, err = service.CompleteGoogle(context.Background(), start.State, start.State, "code", "")
	requireGoogleError(t, err, "oauth_state_invalid")
	h.provider.claims.Email = "changed-google-address@example.com"
	start, err = service.BeginGoogle(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	again, err := service.CompleteGoogle(context.Background(), start.State, start.State, "code", "")
	if err != nil || again.User.ID != first.User.ID || again.User.Email != first.User.Email {
		t.Fatal("changing Google email altered the subject's account binding", err)
	}
	start, err = service.BeginGoogle(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	h.provider.claims.Subject = "another-google-subject"
	h.provider.claims.Email = first.User.Email
	_, err = service.CompleteGoogle(context.Background(), start.State, start.State, "code", "")
	requireGoogleError(t, err, "account_link_required")
	var identities int64
	if err = db.Model(&database.AuthIdentity{}).Count(&identities).Error; err != nil || identities != 1 {
		t.Fatal("refused email link persisted a second identity", identities, err)
	}
}
