package analysis

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"relations/internal/ai"
	"relations/internal/database"
	"relations/internal/questionnaire"
	"relations/internal/scoring"
)

type denyExampleTransport struct{ calls *atomic.Int64 }

func (transport denyExampleTransport) RoundTrip(*http.Request) (*http.Response, error) {
	transport.calls.Add(1)
	return nil, errors.New("network is forbidden for this local-example integration test")
}

func TestActualAIServiceLocalSubmissionIgnoresActiveLiveProviderAndExhaustedQuota(t *testing.T) {
	db := testDB(t)
	cfg := testConfig()
	cfg.EncryptionKey = []byte(strings.Repeat("k", 32))
	ctx := context.Background()
	var calls atomic.Int64
	previousTransport := http.DefaultTransport
	http.DefaultTransport = denyExampleTransport{calls: &calls}
	t.Cleanup(func() { http.DefaultTransport = previousTransport })
	generator := ai.New(cfg, db)
	administrator := testUser(t, db)
	if err := db.Model(&administrator).Update("role", "admin").Error; err != nil {
		t.Fatal(err)
	}
	if _, err := generator.SetProvider(ctx, administrator.ID, ai.ProviderInput{Provider: "openai", Model: "sample-live-model", APIKey: "never-send-this-provider-key", MaxTokens: 512, TimeoutSeconds: 1, Active: true}); err != nil {
		t.Fatal(err)
	}
	service := New(cfg, db, generator)
	now := time.Now().UTC()
	service.now = func() time.Time { return now }
	if err := service.InitializeSettings(ctx); err != nil {
		t.Fatal(err)
	}
	owner := testUser(t, db)
	for _, counter := range []database.DailyUsage{
		{Day: usageDay(now), ScopeKey: "global", Counter: int64(cfg.GlobalDailyLimit)},
		{Day: usageDay(now), ScopeKey: "user:" + owner.ID, Counter: int64(cfg.UserDailyLimit)},
	} {
		if err := db.Create(&counter).Error; err != nil {
			t.Fatal(err)
		}
	}
	for _, test := range []struct {
		module, locale string
		input          questionnaire.Context
	}{
		{"self", "en", questionnaire.Context{}},
		{"conflict", "fa", questionnaire.Context{RelationshipType: "friend", Safety: "unknown", Topic: "Private scheduling concern"}},
	} {
		answers := allAnswers(t, test.module, false)
		draft, err := service.Create(ctx, owner.ID, CreateInput{Module: test.module, Locale: test.locale, Answers: answers, Context: test.input})
		if err != nil {
			t.Fatal(err)
		}
		result, err := service.Submit(ctx, owner.ID, draft.ID, SubmitInput{Revision: draft.Revision, Consent: true, GenerateReport: true, LocalExample: true, AIConsent: false})
		if err != nil || result.Status != "completed" || result.Provider != ai.MockProvider || result.Model != ai.MockModel || result.PromptVersion != ai.MockPromptVersion || result.ReportSchema != ai.ReportSchema || result.AIConsentAt != nil || result.ConsentAt == nil || result.ProcessingStartedAt != nil {
			t.Fatal("explicit local submission used live-provider lifecycle", result, err)
		}
		var report ai.Report
		if err := json.Unmarshal(result.Report, &report); err != nil || report.Kind != "example" || report.Locale != test.locale {
			t.Fatal("wrong example metadata", report, err)
		}
		def, err := questionnaire.Get(test.module, questionnaire.Version)
		if err != nil {
			t.Fatal(err)
		}
		expected, err := scoring.Compute(def, answers, test.input)
		if err != nil {
			t.Fatal(err)
		}
		if len(report.ScoreEcho) != len(expected.Respondents["self"].Dimensions) {
			t.Fatal("missing deterministic score echoes")
		}
		for _, echo := range report.ScoreEcho {
			found := false
			for _, dimension := range expected.Respondents[echo.Respondent].Dimensions {
				if dimension.ID == echo.DimensionID {
					found = true
					if (dimension.Score == nil) != (echo.Score == nil) || dimension.Score != nil && *dimension.Score != *echo.Score {
						t.Fatal("local output changed actual scores")
					}
				}
			}
			if !found {
				t.Fatal("unknown local score echo")
			}
		}
		if test.module == "conflict" && (report.Safety.Level != "caution" || len(report.Safety.Support) == 0) {
			t.Fatal("local conflict report omitted safety caution")
		}
		var event database.UsageEvent
		if err := db.Where("analysis_id = ?", result.ID).First(&event).Error; err != nil || event.Provider != ai.MockProvider || event.Status != "success" || event.InputTokens != 0 || event.OutputTokens != 0 || event.LatencyMS != 0 {
			t.Fatal("local submission recorded live usage", event, err)
		}
	}
	if calls.Load() != 0 {
		t.Fatal("local submission attempted external requests", calls.Load())
	}
	var counters []database.DailyUsage
	if err := db.Find(&counters).Error; err != nil || len(counters) != 2 {
		t.Fatal("local submission added quota rows", counters, err)
	}
	for _, counter := range counters {
		expected := int64(cfg.UserDailyLimit)
		if counter.ScopeKey == "global" {
			expected = int64(cfg.GlobalDailyLimit)
		}
		if counter.Counter != expected {
			t.Fatal("local submission consumed quota", counter)
		}
	}
	provider, err := generator.GetProvider(ctx)
	if err != nil || provider.Provider != "openai" || !provider.Active || !provider.HasKey {
		t.Fatal("local submission changed live provider configuration", provider, err)
	}
}
