package analysis

import (
	"context"
	"encoding/json"
	"testing"

	"relations/internal/ai"
	"relations/internal/config"
	"relations/internal/questionnaire"
	"relations/internal/scoring"
)

type localGenerator struct{ fakeGenerator }

func (g *localGenerator) GenerateExample(ctx context.Context, module, locale string, result scoring.Result, input questionnaire.Context) (ai.Generation, error) {
	return ai.New(config.Config{}, nil).GenerateExample(ctx, module, locale, result, input)
}

func TestExplicitLocalExampleNeverUsesExternalGeneratorOrQuota(t *testing.T) {
	db := testDB(t)
	user := testUser(t, db)
	generator := &localGenerator{}
	service := New(testConfig(), db, generator)
	draft, err := service.Create(context.Background(), user.ID, CreateInput{Module: "self", Locale: "en", Answers: allAnswers(t, "self", false)})
	if err != nil {
		t.Fatal(err)
	}
	value, err := service.Submit(context.Background(), user.ID, draft.ID, SubmitInput{Revision: draft.Revision, Consent: true, GenerateReport: true, LocalExample: true})
	if err != nil {
		t.Fatal(err)
	}
	if value.Status != "completed" || value.Provider != "mock" || value.AIConsentAt != nil || generator.calls.Load() != 0 {
		t.Fatal("unsafe example lifecycle")
	}
	var report ai.Report
	if err = json.Unmarshal(value.Report, &report); err != nil || report.Kind != "example" {
		t.Fatal("example report", err)
	}
	for i, echo := range report.ScoreEcho {
		if echo.Score == nil || *echo.Score != 50 {
			t.Fatal("example changed score", i)
		}
	}
	usage, err := service.Usage(context.Background(), user.ID)
	if err != nil || usage.UserUsed != 0 || usage.GlobalUsed != 0 {
		t.Fatal("example used paid quota", err)
	}
	var events int64
	if err = db.Table("usage_events").Where("analysis_id = ? AND provider = 'mock' AND input_tokens = 0 AND output_tokens = 0", value.ID).Count(&events).Error; err != nil || events != 1 {
		t.Fatal("example metrics", err)
	}
}

func TestLocalExampleNeedsGenerationAndProcessingConsent(t *testing.T) {
	service := New(testConfig(), nil, &localGenerator{})
	for _, input := range []SubmitInput{{Revision: 1, Consent: true, LocalExample: true}, {Revision: 1, GenerateReport: true, LocalExample: true}} {
		if _, err := service.Submit(context.Background(), "", "", input); err == nil {
			t.Fatal("accepted invalid local example consent")
		}
	}
}
