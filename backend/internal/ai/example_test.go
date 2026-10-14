package ai

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"

	"relations/internal/database"
	"relations/internal/platform"
	"relations/internal/questionnaire"
)

func denyNetwork(t *testing.T, s *Service) {
	t.Helper()
	s.client.Transport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		t.Error("local example attempted an external request")
		return nil, errors.New("external requests forbidden in local example tests")
	})
}

func TestPureExamplesAreLocalizedAndNeverCallNetwork(t *testing.T) {
	service := New(aiTestConfig(), nil)
	denyNetwork(t, service)
	for _, module := range []string{"self", "compatibility", "conflict"} {
		for _, locale := range []string{"en", "fa"} {
			sample, err := preview(PreviewInput{Module: module, Locale: locale})
			if err != nil {
				t.Fatal(module, locale, err)
			}
			input := questionnaire.Context{}
			if module == "conflict" {
				input = questionnaire.Context{RelationshipType: "friend", Safety: "unknown", Topic: "sample"}
			}
			gen, err := service.GenerateExample(context.Background(), module, locale, sample.Scores, input)
			if err != nil || gen.Provider != MockProvider || gen.Model != MockModel || gen.Report.Kind != "example" || gen.InputTokens != 0 || gen.OutputTokens != 0 || gen.LatencyMS != 0 {
				t.Fatal(module, locale, gen, err)
			}
			if locale == "en" && !strings.Contains(gen.Report.Summary, "No prompt") || locale == "fa" && !strings.Contains(gen.Report.Title, "محلی") {
				t.Fatal("example not visibly labeled and localized", gen.Report)
			}
			data, _ := json.Marshal(gen.Report)
			if _, err := validateReportKind(data, module, locale, sample.Scores, input, "example"); err != nil {
				t.Fatal("example failed strict output validation", err)
			}
			if _, err := ValidateReport(data, module, locale, sample.Scores, input); err == nil {
				t.Fatal("external provider validator accepted an example kind")
			}
			if module == "conflict" && (gen.Report.Safety.Level != "caution" || len(gen.Report.Safety.Support) == 0) {
				t.Fatal("example lost safety caution")
			}
			changed := gen.Report
			changed.ScoreEcho = append([]ScoreEcho(nil), gen.Report.ScoreEcho...)
			invalid := 80.0
			changed.ScoreEcho[0].Score = &invalid
			data, _ = json.Marshal(changed)
			if _, err := validateReportKind(data, module, locale, sample.Scores, input, "example"); err == nil {
				t.Fatal("local validation accepted a changed score")
			}
		}
	}
}

func TestPreviewPreservesSuppliedNullAndOmittedAnswers(t *testing.T) {
	input := PreviewInput{Module: "self", Locale: "en", Answers: questionnaire.Answers{"self": {}}, Context: &questionnaire.Context{}}
	def, err := questionnaire.Get("self", questionnaire.Version)
	if err != nil {
		t.Fatal(err)
	}
	input.Answers["self"][def.Questions[0].ID] = nil
	result, err := preview(input)
	if err != nil || result.UsedSampleAnswers || result.UsedSampleContext {
		t.Fatal(result, err)
	}
	respondent := result.Scores.Respondents["self"]
	if respondent.Unknown != 1 || respondent.Omitted != len(def.Questions)-1 || respondent.Answered != 0 || result.Scores.Status != "insufficient_data" {
		t.Fatal("preview imputed supplied unknown/omitted data", respondent)
	}
	for _, echo := range result.Report.ScoreEcho {
		if echo.Score != nil {
			t.Fatal("preview invented missing scores")
		}
	}
	for _, body := range []string{`null`, `{"module":"self","answers":null}`, `{"module":"self","context":null}`, `{"module":"self","unexpected":true}`} {
		var decoded PreviewInput
		if platform.StrictJSON([]byte(body), &decoded) == nil {
			t.Fatal("accepted invalid preview input", body)
		}
	}
	if _, err := preview(PreviewInput{Module: "self", Locale: "de"}); err == nil {
		t.Fatal("unsupported preview language")
	}
}

func TestPreviewNeverUsesLiveProviderAndDoesNotAuditPersonalData(t *testing.T) {
	db := aiTestDB(t)
	service := New(aiTestConfig(), db)
	actor := aiAdmin(t, db)
	denyNetwork(t, service)
	if _, err := service.SetProvider(context.Background(), actor.ID, ProviderInput{Provider: "openai", Model: "sample-model", APIKey: "private-live-key-123", MaxTokens: 4096, TimeoutSeconds: 60, Active: true}); err != nil {
		t.Fatal(err)
	}
	input := PreviewInput{Module: "conflict", Locale: "en", Context: &questionnaire.Context{RelationshipType: "friend", Safety: "no", Topic: "private-preview-topic", Observations: "private-preview-observation"}}
	result, err := service.Preview(context.Background(), actor.ID, input)
	if err != nil || result.Provider != MockProvider || result.ExternalRequest || !result.IsExample || result.Report.Kind != "example" || result.Report.Safety.Level != "caution" {
		t.Fatal(result, err)
	}
	var saved database.ProviderConfig
	if err := db.First(&saved, "id = 1").Error; err != nil || saved.Provider != "openai" {
		t.Fatal("preview changed live provider", saved, err)
	}
	var event database.AuditEvent
	if err := db.Where("action = ?", "provider.previewed").First(&event).Error; err != nil {
		t.Fatal(err)
	}
	for _, sensitive := range []string{"private-live-key", "private-preview-topic", "private-preview-observation"} {
		if strings.Contains(string(event.Metadata), sensitive) {
			t.Fatal("sensitive preview content persisted in audit")
		}
	}
	var metadata map[string]any
	if err := json.Unmarshal(event.Metadata, &metadata); err != nil {
		t.Fatal(err)
	}
	if _, ok := metadata["answers"]; ok {
		t.Fatal("raw answers in audit")
	}
	if _, ok := metadata["context"]; ok {
		t.Fatal("raw context in audit")
	}
	var count int64
	if err := db.Model(&database.Analysis{}).Count(&count).Error; err != nil || count != 0 {
		t.Fatal("preview created analysis", count, err)
	}
	if err := db.Model(&database.DailyUsage{}).Count(&count).Error; err != nil || count != 0 {
		t.Fatal("preview consumed quota", count, err)
	}
}

func TestMockSettingsNeedNoKeyClearOldKeysAndStayLocal(t *testing.T) {
	db := aiTestDB(t)
	service := New(aiTestConfig(), db)
	actor := aiAdmin(t, db)
	denyNetwork(t, service)
	ctx := context.Background()
	if _, err := service.SetProvider(ctx, actor.ID, ProviderInput{Provider: "openai", Model: "sample-model", APIKey: "private-live-key-123", MaxTokens: 4096, TimeoutSeconds: 60, Active: true}); err != nil {
		t.Fatal(err)
	}
	view, err := service.SetProvider(ctx, actor.ID, ProviderInput{Provider: MockProvider, Active: true})
	if err != nil || !view.Active || !view.IsExample || view.HasKey || view.Model != MockModel {
		t.Fatal(view, err)
	}
	var saved database.ProviderConfig
	if err := db.First(&saved, "id = 1").Error; err != nil || len(saved.EncryptedKey) != 0 {
		t.Fatal("mock retained previous live credential", err)
	}
	available, err := service.Available(ctx)
	if err != nil || !available {
		t.Fatal("active mock unavailable without key", available, err)
	}
	_, scores, input := reportFixture(t, "self")
	gen, err := service.Generate(ctx, "self", "en", scores, input)
	if err != nil || gen.Report.Kind != "example" || gen.Provider != MockProvider {
		t.Fatal(gen, err)
	}
	if _, err := service.TestProvider(ctx, actor.ID); err == nil {
		t.Fatal("mock advertised a live connection test")
	}
	if _, err := service.SetProvider(ctx, actor.ID, ProviderInput{Provider: MockProvider, APIKey: "an-unneeded-key", Active: true}); err == nil {
		t.Fatal("mock accepted credentials")
	}
	if _, err := service.SetProvider(ctx, actor.ID, ProviderInput{Provider: "openai", Model: "sample-model", MaxTokens: 4096, TimeoutSeconds: 60, Active: true}); err == nil {
		t.Fatal("switch from mock to live reused absent credentials")
	}
	if _, err := service.SetProvider(ctx, actor.ID, ProviderInput{Provider: MockProvider, Active: false}); err != nil {
		t.Fatal(err)
	}
	if available, err := service.Available(ctx); err != nil || available {
		t.Fatal("inactive mock available")
	}
	if _, err := service.Generate(ctx, "self", "en", scores, input); err == nil {
		t.Fatal("inactive configured mock generated regular report")
	}
}

func TestMockMigrationRollbackPreservesExistingMockRecords(t *testing.T) {
	db := aiTestDB(t)
	service := New(aiTestConfig(), db)
	actor := aiAdmin(t, db)
	ctx := context.Background()
	if _, err := service.SetProvider(ctx, actor.ID, ProviderInput{Provider: MockProvider, Active: true}); err != nil {
		t.Fatal(err)
	}
	if err := database.RollbackLast(ctx, db); err == nil {
		t.Fatal("rollback silently relabeled or removed mock settings")
	}
	view, err := service.GetProvider(ctx)
	if err != nil || view.Provider != MockProvider || !view.Active {
		t.Fatal("failed rollback damaged mock configuration", view, err)
	}
	var count int64
	if err := db.Raw("SELECT count(*) FROM schema_migrations WHERE version = ?", "0004_ai_mode.up.sql").Scan(&count).Error; err != nil || count != 1 {
		t.Fatal("failed rollback removed migration ledger", count, err)
	}
}
