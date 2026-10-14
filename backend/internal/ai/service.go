package ai

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"relations/internal/config"
	"relations/internal/database"
	"relations/internal/platform"
	"relations/internal/questionnaire"
	"relations/internal/scoring"
)

type Service struct {
	cfg       config.Config
	db        *gorm.DB
	client    *http.Client
	endpoints map[string]string
}

func New(cfg config.Config, db *gorm.DB) *Service {
	urls := map[string]string{}
	for k, v := range endpoints {
		urls[k] = v
	}
	return &Service{cfg: cfg, db: db, endpoints: urls, client: &http.Client{CheckRedirect: func(req *http.Request, via []*http.Request) error { return http.ErrUseLastResponse }}}
}

type ProviderInput struct {
	Provider       string `json:"provider"`
	Model          string `json:"model"`
	APIKey         string `json:"api_key,omitempty"`
	MaxTokens      int    `json:"max_tokens"`
	TimeoutSeconds int    `json:"timeout_seconds"`
	Retries        int    `json:"retries"`
	Active         bool   `json:"active"`
}
type ProviderView struct {
	Configured     bool       `json:"configured"`
	Provider       string     `json:"provider"`
	Model          string     `json:"model"`
	HasKey         bool       `json:"has_key"`
	MaxTokens      int        `json:"max_tokens"`
	TimeoutSeconds int        `json:"timeout_seconds"`
	Retries        int        `json:"retries"`
	Active         bool       `json:"active"`
	IsExample      bool       `json:"is_example"`
	UpdatedAt      *time.Time `json:"updated_at"`
}
type TestResult struct {
	Success   bool   `json:"success"`
	Provider  string `json:"provider"`
	Model     string `json:"model"`
	LatencyMS int64  `json:"latency_ms"`
}

func view(cfg database.ProviderConfig) ProviderView {
	return ProviderView{Configured: true, Provider: cfg.Provider, Model: cfg.Model, HasKey: len(cfg.EncryptedKey) > 0, MaxTokens: cfg.MaxTokens, TimeoutSeconds: cfg.TimeoutSeconds, Retries: cfg.Retries, Active: cfg.Active, IsExample: cfg.Provider == MockProvider, UpdatedAt: &cfg.UpdatedAt}
}
func (s *Service) GetProvider(ctx context.Context) (ProviderView, error) {
	var cfg database.ProviderConfig
	err := s.db.WithContext(ctx).First(&cfg, "id = 1").Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ProviderView{MaxTokens: s.cfg.AIMaxTokens, TimeoutSeconds: int(s.cfg.AITimeout.Seconds()), Retries: s.cfg.AIRetries}, nil
	}
	if err != nil {
		return ProviderView{}, err
	}
	return view(cfg), nil
}

var modelPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$`)

func validKey(value string) bool {
	if len(value) < 10 || len(value) > 2048 {
		return false
	}
	for _, r := range value {
		if r < 33 || r > 126 {
			return false
		}
	}
	return true
}
func (s *Service) SetProvider(ctx context.Context, actor string, input ProviderInput) (ProviderView, error) {
	if input.Provider == MockProvider {
		if input.APIKey != "" || input.Model != "" && input.Model != MockModel {
			return ProviderView{}, platform.Err(400, "invalid_provider", "Local example mode uses local-example-v1 and does not accept an API key.")
		}
		input.Model = MockModel
		if input.MaxTokens == 0 {
			input.MaxTokens = s.cfg.AIMaxTokens
			if input.MaxTokens == 0 {
				input.MaxTokens = 4096
			}
		}
		if input.TimeoutSeconds == 0 {
			input.TimeoutSeconds = int(s.cfg.AITimeout.Seconds())
			if input.TimeoutSeconds == 0 {
				input.TimeoutSeconds = 60
			}
		}
	}
	if (endpoints[input.Provider] == "" && input.Provider != MockProvider) || !modelPattern.MatchString(input.Model) || input.MaxTokens < 512 || input.MaxTokens > 16000 || input.TimeoutSeconds < 1 || input.TimeoutSeconds > 120 || input.Retries < 0 || input.Retries > 3 || (input.APIKey != "" && !validKey(input.APIKey)) {
		return ProviderView{}, platform.Err(400, "invalid_provider", "Use an allowed provider, valid model identifier, and supported generation settings.")
	}
	var encrypted []byte
	var err error
	if input.APIKey != "" {
		encrypted, err = encryptKey(s.cfg.EncryptionKey, input.APIKey)
		if err != nil {
			return ProviderView{}, platform.Err(500, "encryption_unavailable", "Provider credentials could not be stored.")
		}
	}
	var cfg database.ProviderConfig
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Covers both initial insertion and replacement without a missing-row race.
		if err := tx.Exec("SELECT pg_advisory_xact_lock(?)", int64(77182439)).Error; err != nil {
			return err
		}
		err := tx.First(&cfg, "id = 1").Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if cfg.Provider != "" && cfg.Provider != input.Provider && input.APIKey == "" && input.Provider != MockProvider {
			return platform.Err(400, "provider_key_required", "Supply a new API key when switching providers.")
		}
		if input.APIKey != "" {
			cfg.EncryptedKey = encrypted
		}
		if input.Provider == MockProvider {
			cfg.EncryptedKey = nil
		}
		if input.Active && len(cfg.EncryptedKey) == 0 && input.Provider != MockProvider {
			return platform.Err(400, "provider_key_required", "Supply an API key before activating the provider.")
		}
		cfg.ID = 1
		cfg.Provider = input.Provider
		cfg.Model = input.Model
		cfg.MaxTokens = input.MaxTokens
		cfg.TimeoutSeconds = input.TimeoutSeconds
		cfg.Retries = input.Retries
		cfg.Active = input.Active
		cfg.UpdatedBy = &actor
		if err := tx.Save(&cfg).Error; err != nil {
			return err
		}
		metadata, _ := json.Marshal(map[string]any{"provider": cfg.Provider, "model": cfg.Model, "active": cfg.Active, "key_updated": input.APIKey != "", "max_tokens": cfg.MaxTokens, "timeout_seconds": cfg.TimeoutSeconds, "retries": cfg.Retries})
		return tx.Create(&database.AuditEvent{ID: uuid.NewString(), ActorID: &actor, Action: "provider.updated", Metadata: metadata}).Error
	})
	if err != nil {
		return ProviderView{}, err
	}
	return view(cfg), nil
}

func (s *Service) Available(ctx context.Context) (bool, error) {
	var cfg database.ProviderConfig
	err := s.db.WithContext(ctx).First(&cfg, "id = 1").Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return cfg.Active && (cfg.Provider == MockProvider || len(cfg.EncryptedKey) > 0), nil
}

func (s *Service) credential(ctx context.Context, requireActive bool) (database.ProviderConfig, string, error) {
	var cfg database.ProviderConfig
	err := s.db.WithContext(ctx).First(&cfg, "id = 1").Error
	if errors.Is(err, gorm.ErrRecordNotFound) || (err == nil && (cfg.Provider != MockProvider && len(cfg.EncryptedKey) == 0 || (requireActive && !cfg.Active))) {
		return cfg, "", platform.Err(503, "ai_unavailable", "An AI provider is not configured and active.")
	}
	if err != nil {
		return cfg, "", err
	}
	if cfg.Provider == MockProvider {
		return cfg, "", nil
	}
	key, err := decryptKey(s.cfg.EncryptionKey, cfg.EncryptedKey)
	if err != nil {
		return cfg, "", platform.Err(503, "ai_unavailable", "Provider credentials are unavailable. An administrator must replace them.")
	}
	return cfg, key, nil
}

func (s *Service) Generate(ctx context.Context, module, locale string, scores scoring.Result, input questionnaire.Context) (Generation, error) {
	started := time.Now()
	gen := Generation{PromptVersion: PromptVersion, ReportSchema: ReportSchema}
	modulePrompt, err := prompt(module, locale)
	if err != nil {
		return gen, platform.Err(400, "invalid_analysis", "The module or report language is invalid.")
	}
	def, err := questionnaire.Get(module, scores.QuestionnaireVersion)
	if err != nil || scores.Module != module {
		return gen, platform.Err(400, "invalid_analysis", "The questionnaire or deterministic scores are invalid.")
	}
	cfg, key, err := s.credential(ctx, true)
	gen.Provider = cfg.Provider
	gen.Model = cfg.Model
	if err != nil {
		return gen, err
	}
	if cfg.Provider == MockProvider {
		return s.GenerateExample(ctx, module, locale, scores, input)
	}
	refs := []string{}
	seen := map[string]bool{}
	for _, d := range def.Dimensions {
		for _, ref := range d.References {
			if !seen[ref] {
				refs = append(refs, ref)
				seen[ref] = true
			}
		}
	}
	envelope, err := json.Marshal(map[string]any{"module": module, "locale": locale, "scores": scores, "context": input, "expected_score_echo": echoes(scores), "allowed_references": refs, "output_schema": schema()})
	if err != nil {
		return gen, err
	}
	result, err := s.requestWithRetry(ctx, cfg, key, commonPrompt+modulePrompt, string(envelope), schema())
	gen.LatencyMS = time.Since(started).Milliseconds()
	gen.InputTokens = result.InputTokens
	gen.OutputTokens = result.OutputTokens
	if err != nil {
		return gen, err
	}
	gen.Report, err = ValidateReport(result.Content, module, locale, scores, input)
	return gen, err
}

func (s *Service) TestProvider(ctx context.Context, actor string) (TestResult, error) {
	cfg, key, err := s.credential(ctx, false)
	if err != nil {
		return TestResult{}, err
	}
	if cfg.Provider == MockProvider {
		return TestResult{}, platform.Err(400, "local_provider", "Local example mode has no external connection. Use the local example preview.")
	}
	started := time.Now()
	cfg.MaxTokens = 512
	cfg.Retries = 0
	testSchema := object(map[string]any{"ok": map[string]any{"type": "boolean"}})
	result, err := s.requestWithRetry(ctx, cfg, key, "Reply with exactly one JSON object: {\"ok\":true}. This is a connection check with no personal data.", "Return JSON with ok true.", testSchema)
	if err == nil {
		var response struct {
			OK bool `json:"ok"`
		}
		if platform.StrictJSON(result.Content, &response) != nil || !response.OK || strings.TrimSpace(string(result.Content)) == "" {
			err = platform.Err(502, "ai_invalid_report", "The provider returned an invalid connection-check response.")
		}
	}
	latency := time.Since(started).Milliseconds()
	code := ""
	var appErr *platform.Error
	if errors.As(err, &appErr) {
		code = appErr.Code
	}
	metadata, _ := json.Marshal(map[string]any{"provider": cfg.Provider, "model": cfg.Model, "success": err == nil, "error_code": code, "latency_ms": latency})
	if auditErr := s.db.WithContext(ctx).Create(&database.AuditEvent{ID: uuid.NewString(), ActorID: &actor, Action: "provider.tested", Metadata: metadata}).Error; err == nil {
		err = auditErr
	}
	if err != nil {
		return TestResult{}, err
	}
	return TestResult{true, cfg.Provider, cfg.Model, latency}, nil
}
