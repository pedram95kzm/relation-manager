package analysis

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"relations/internal/ai"
	"relations/internal/config"
	"relations/internal/database"
	"relations/internal/platform"
	"relations/internal/questionnaire"
	"relations/internal/scoring"
)

type fakeGenerator struct {
	available bool
	err       error
	calls     atomic.Int64
	before    func()
}

func (g *fakeGenerator) Available(context.Context) (bool, error) { return g.available, nil }
func (g *fakeGenerator) Generate(_ context.Context, module, locale string, scores scoring.Result, _ questionnaire.Context) (ai.Generation, error) {
	g.calls.Add(1)
	if g.before != nil {
		g.before()
	}
	return ai.Generation{Report: ai.Report{Kind: "ai", SchemaVersion: "1.0.0", Module: module, Locale: locale, Title: "Educational report", Summary: "A bounded educational narrative."},
		Provider: "openai", Model: "fixture-model", PromptVersion: "1.0.0", ReportSchema: "1.0.0", InputTokens: 10, OutputTokens: 20, LatencyMS: 5}, g.err
}

func testConfig() config.Config {
	return config.Config{MinAge: 18, GlobalDailyLimit: 50, UserDailyLimit: 10, AITimeout: time.Minute, AIRetries: 1}
}

func testDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL required for isolated PostgreSQL analysis integration test")
	}
	u, err := url.Parse(dsn)
	if err != nil || (u.Scheme != "postgres" && u.Scheme != "postgresql") || !strings.Contains(strings.ToLower(u.Path), "test") {
		t.Fatal("refusing non-test database")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	t.Cleanup(cancel)
	manager, err := database.Open(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	managerSQL, _ := manager.DB()
	t.Cleanup(func() { _ = managerSQL.Close() })
	schema := "test_analysis_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if err = manager.Exec(`CREATE SCHEMA "` + schema + `"`).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = manager.Exec(`DROP SCHEMA "` + schema + `" CASCADE`).Error })
	query := u.Query()
	query.Set("search_path", schema)
	u.RawQuery = query.Encode()
	db, err := database.Open(ctx, u.String())
	if err != nil {
		t.Fatal(err)
	}
	sql, _ := db.DB()
	t.Cleanup(func() { _ = sql.Close() })
	if err = database.Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	if err = questionnaire.Seed(ctx, db); err != nil {
		t.Fatal(err)
	}
	return db
}

func testUser(t *testing.T, db *gorm.DB) database.User {
	t.Helper()
	age := 28
	user := database.User{ID: uuid.NewString(), Email: uuid.NewString() + "@example.test", Name: "Fixture account", Role: "user", Age: &age, Gender: "prefer_not_to_say", AuthVersion: 1}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	return user
}

func allAnswers(t *testing.T, module string, unknown bool) questionnaire.Answers {
	t.Helper()
	definition, err := questionnaire.Get(module, questionnaire.Version)
	if err != nil {
		t.Fatal(err)
	}
	answers := questionnaire.Answers{"self": {}}
	if module == "compatibility" {
		answers["partner"] = map[string]*int{}
	}
	for _, respondent := range answers {
		for _, question := range definition.Questions {
			var value *int
			if !unknown {
				number := 3
				value = &number
			}
			respondent[question.ID] = value
		}
	}
	return answers
}

func draft(t *testing.T, s *Service, user database.User, module string, ctx questionnaire.Context, unknown bool) database.Analysis {
	t.Helper()
	result, err := s.Create(context.Background(), user.ID, CreateInput{Module: module, Locale: "en", Answers: allAnswers(t, module, unknown), Context: ctx})
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func expectError(t *testing.T, err error, code string) {
	t.Helper()
	var safe *platform.Error
	if !errors.As(err, &safe) || safe.Code != code {
		t.Fatalf("expected %s, got %v", code, err)
	}
}

func submitAI(revision int64) SubmitInput {
	return SubmitInput{Revision: revision, Consent: true, GenerateReport: true, AIConsent: true}
}

func TestOwnerScopedDraftRevisionAndHistory(t *testing.T) {
	db := testDB(t)
	s := New(testConfig(), db, nil)
	owner, stranger := testUser(t, db), testUser(t, db)
	value := draft(t, s, owner, "self", questionnaire.Context{}, false)
	if value.Status != "draft" || value.Revision != 1 {
		t.Fatal(value)
	}
	for _, id := range []string{value.ID, "invalid-uuid", uuid.NewString()} {
		_, err := s.Get(context.Background(), stranger.ID, id)
		expectError(t, err, "analysis_not_found")
	}
	ctx := questionnaire.Context{}
	_, err := s.Update(context.Background(), stranger.ID, value.ID, UpdateInput{Revision: 1, Answers: questionnaire.Answers{}, Context: &ctx})
	expectError(t, err, "analysis_not_found")
	updated, err := s.Update(context.Background(), owner.ID, value.ID, UpdateInput{Revision: 1, Locale: "fa", Answers: questionnaire.Answers{}, Context: &ctx})
	if err != nil || updated.Revision != 2 || updated.Locale != "fa" || string(updated.Answers) != "{}" {
		t.Fatal(updated, err)
	}
	_, err = s.Update(context.Background(), owner.ID, value.ID, UpdateInput{Revision: 1, Answers: questionnaire.Answers{}, Context: &ctx})
	expectError(t, err, "revision_conflict")
	history, err := s.List(context.Background(), stranger.ID, 1, 20, "", "")
	if err != nil || history.Total != 0 || len(history.Items) != 0 {
		t.Fatal(history, err)
	}
	history, err = s.List(context.Background(), owner.ID, 1, 20, "self", "draft")
	if err != nil || history.Total != 1 || history.Items[0].ID != value.ID {
		t.Fatal(history, err)
	}
	if err := s.Delete(context.Background(), stranger.ID, value.ID); err == nil {
		t.Fatal("stranger deleted owner draft")
	}
	if err := s.Delete(context.Background(), owner.ID, value.ID); err != nil {
		t.Fatal(err)
	}
	_, err = s.Get(context.Background(), owner.ID, value.ID)
	expectError(t, err, "analysis_not_found")
}

func TestSubmissionFinalValidationAndSeparateAIConsent(t *testing.T) {
	db := testDB(t)
	generator := &fakeGenerator{available: true}
	s := New(testConfig(), db, generator)
	user := testUser(t, db)
	value, err := s.Create(context.Background(), user.ID, CreateInput{Module: "self", Locale: "en"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.Submit(context.Background(), user.ID, value.ID, SubmitInput{Revision: 1})
	expectError(t, err, "consent_required")
	_, err = s.Submit(context.Background(), user.ID, value.ID, SubmitInput{Revision: 1, Consent: true, GenerateReport: true})
	expectError(t, err, "ai_consent_required")
	_, err = s.Submit(context.Background(), user.ID, value.ID, submitAI(1))
	expectError(t, err, "answers_incomplete")
	if generator.calls.Load() != 0 {
		t.Fatal("incomplete or unconsented answers reached provider")
	}
	usage, err := s.Usage(context.Background(), user.ID)
	if err != nil || usage.GlobalUsed != 0 || usage.UserUsed != 0 {
		t.Fatal(usage, err)
	}
	if err = db.Model(&database.User{}).Where("id = ?", user.ID).Update("age", nil).Error; err != nil {
		t.Fatal(err)
	}
	_, err = s.Create(context.Background(), user.ID, CreateInput{Module: "self"})
	expectError(t, err, "profile_required")
}

func TestDeterministicSubmissionAndUnknownsConsumeNoQuota(t *testing.T) {
	for _, unknown := range []bool{false, true} {
		t.Run(map[bool]string{false: "available", true: "unknown"}[unknown], func(t *testing.T) {
			db := testDB(t)
			generator := &fakeGenerator{available: true}
			s := New(testConfig(), db, generator)
			user := testUser(t, db)
			value := draft(t, s, user, "self", questionnaire.Context{}, unknown)
			input := SubmitInput{Revision: 1, Consent: true}
			if unknown {
				input = submitAI(1)
			}
			result, err := s.Submit(context.Background(), user.ID, value.ID, input)
			want := "completed"
			if unknown {
				want = "insufficient"
			}
			if err != nil || result.Status != want || result.ConsentAt == nil || result.AIConsentAt != nil || generator.calls.Load() != 0 {
				t.Fatal(result, err)
			}
			var report DeterministicReport
			var scores scoring.Result
			if err := json.Unmarshal(result.Report, &report); err != nil || report.Kind != "deterministic" {
				t.Fatal(report, err)
			}
			if err := json.Unmarshal(result.Scores, &scores); err != nil {
				t.Fatal(err)
			}
			if unknown && (scores.Respondents["self"].Dimensions[0].Score != nil || scores.Respondents["self"].Unknown != 36) {
				t.Fatal("unknown data became fabricated numbers", scores)
			}
			usage, err := s.Usage(context.Background(), user.ID)
			if err != nil || usage.UserUsed != 0 || usage.GlobalUsed != 0 {
				t.Fatal(usage, err)
			}
		})
	}
}

func TestUnavailableProviderAndUnconsentedPartnerNeverTransmit(t *testing.T) {
	db := testDB(t)
	generator := &fakeGenerator{available: false}
	s := New(testConfig(), db, generator)
	user := testUser(t, db)
	value := draft(t, s, user, "self", questionnaire.Context{}, false)
	result, err := s.Submit(context.Background(), user.ID, value.ID, submitAI(1))
	if err != nil || result.Status != "completed" || result.AIConsentAt != nil {
		t.Fatal(result, err)
	}
	generator.available = true
	for _, ctx := range []questionnaire.Context{{PartnerResponseOrigin: "independent", PartnerConsent: false}, {PartnerResponseOrigin: "proxy", PartnerConsent: true}} {
		value := draft(t, s, user, "compatibility", ctx, false)
		result, err = s.Submit(context.Background(), user.ID, value.ID, submitAI(1))
		if err != nil || result.Status != "insufficient" || result.AIConsentAt != nil {
			t.Fatal(result, err)
		}
	}
	if generator.calls.Load() != 0 {
		t.Fatal("unavailable provider or unconsented/proxy partner data transmitted")
	}
}

func TestAtomicGlobalQuotaAcrossConcurrentUsers(t *testing.T) {
	db := testDB(t)
	cfg := testConfig()
	cfg.GlobalDailyLimit, cfg.UserDailyLimit = 1, 5
	generator := &fakeGenerator{available: true}
	s := New(cfg, db, generator)
	if err := s.InitializeSettings(context.Background()); err != nil {
		t.Fatal(err)
	}
	type attempt struct{ userID, id string }
	attempts := []attempt{}
	for range 4 {
		user := testUser(t, db)
		value := draft(t, s, user, "self", questionnaire.Context{}, false)
		attempts = append(attempts, attempt{user.ID, value.ID})
	}
	var group sync.WaitGroup
	results := make(chan error, len(attempts))
	for _, attempt := range attempts {
		group.Add(1)
		go func() {
			defer group.Done()
			_, err := s.Submit(context.Background(), attempt.userID, attempt.id, submitAI(1))
			results <- err
		}()
	}
	group.Wait()
	close(results)
	successes := 0
	for err := range results {
		if err == nil {
			successes++
		} else {
			expectError(t, err, "global_daily_limit")
		}
	}
	if successes != 1 || generator.calls.Load() != 1 {
		t.Fatal("concurrent calls bypassed atomic budget", successes, generator.calls.Load())
	}
	usage, err := s.Usage(context.Background(), attempts[0].userID)
	if err != nil || usage.GlobalUsed != 1 {
		t.Fatal(usage, err)
	}
}

func TestUserOverrideRollsBackGlobalReservationAndUTCReset(t *testing.T) {
	db := testDB(t)
	generator := &fakeGenerator{available: true}
	s := New(testConfig(), db, generator)
	now := time.Date(2026, 10, 11, 23, 59, 0, 0, time.UTC)
	s.now = func() time.Time { return now }
	user := testUser(t, db)
	if err := db.Model(&database.User{}).Where("id = ?", user.ID).Update("daily_limit", 1).Error; err != nil {
		t.Fatal(err)
	}
	first := draft(t, s, user, "self", questionnaire.Context{}, false)
	if _, err := s.Submit(context.Background(), user.ID, first.ID, submitAI(1)); err != nil {
		t.Fatal(err)
	}
	second := draft(t, s, user, "self", questionnaire.Context{}, false)
	_, err := s.Submit(context.Background(), user.ID, second.ID, submitAI(1))
	expectError(t, err, "user_daily_limit")
	usage, err := s.Usage(context.Background(), user.ID)
	if err != nil || usage.GlobalUsed != 1 || usage.UserUsed != 1 || usage.UserLimit != 1 || usage.Day != "2026-10-11" {
		t.Fatal("partial reservation survived rollback", usage, err)
	}
	now = now.Add(2 * time.Minute)
	if _, err := s.Submit(context.Background(), user.ID, second.ID, submitAI(1)); err != nil {
		t.Fatal("UTC day did not reset budget", err)
	}
	usage, err = s.Usage(context.Background(), user.ID)
	if err != nil || usage.GlobalUsed != 1 || usage.UserUsed != 1 || usage.Day != "2026-10-12" {
		t.Fatal(usage, err)
	}
}

func TestGenerationFailureKeepsScoresAndRetryConsumesAttempt(t *testing.T) {
	db := testDB(t)
	generator := &fakeGenerator{available: true, err: errors.New("secret API credential in raw provider error")}
	s := New(testConfig(), db, generator)
	user := testUser(t, db)
	value := draft(t, s, user, "self", questionnaire.Context{}, false)
	result, err := s.Submit(context.Background(), user.ID, value.ID, submitAI(1))
	if err != nil || result.Status != "failed" || result.FailureCode != "ai_provider_error" || result.AIConsentAt == nil || len(result.Scores) <= 2 {
		t.Fatal(result, err)
	}
	encoded, _ := json.Marshal(result)
	if strings.Contains(string(encoded), "secret") || strings.Contains(string(encoded), "credential") {
		t.Fatal("raw provider failure leaked")
	}
	var event database.UsageEvent
	if err := db.Where("analysis_id = ?", result.ID).First(&event).Error; err != nil || event.Status != "error" || event.ErrorCode != "ai_provider_error" || event.InputTokens != 10 {
		t.Fatal(event, err)
	}
	generator.err = nil
	retried, err := s.Submit(context.Background(), user.ID, result.ID, submitAI(result.Revision))
	if err != nil || retried.Status != "completed" || retried.Provider != "openai" || retried.ReportSchema != "1.0.0" {
		t.Fatal(retried, err)
	}
	usage, err := s.Usage(context.Background(), user.ID)
	if err != nil || usage.UserUsed != 2 || usage.GlobalUsed != 2 {
		t.Fatal("failed provider attempt did not count", usage, err)
	}
	ctx := questionnaire.Context{}
	_, err = s.Update(context.Background(), user.ID, result.ID, UpdateInput{Revision: retried.Revision, Answers: allAnswers(t, "self", false), Context: &ctx})
	expectError(t, err, "analysis_locked")
	if err := s.Delete(context.Background(), user.ID, result.ID); err != nil {
		t.Fatal(err)
	}
	if err := db.First(&event, "id = ?", event.ID).Error; err != nil || event.AnalysisID != nil || event.UserID == nil {
		t.Fatal("deleted analysis retained sensitive usage reference", event, err)
	}
}

func TestGenerationRechecksAccountStateBeforePersisting(t *testing.T) {
	for _, change := range []string{"disable", "auth_version", "delete"} {
		t.Run(change, func(t *testing.T) {
			db := testDB(t)
			user := testUser(t, db)
			generator := &fakeGenerator{available: true}
			generator.before = func() {
				var err error
				switch change {
				case "disable":
					err = db.Model(&database.User{}).Where("id = ?", user.ID).Update("disabled", true).Error
				case "auth_version":
					err = db.Model(&database.User{}).Where("id = ?", user.ID).Update("auth_version", 2).Error
				case "delete":
					err = db.Where("id = ?", user.ID).Delete(&database.User{}).Error
				}
				if err != nil {
					t.Fatal(err)
				}
			}
			s := New(testConfig(), db, generator)
			value := draft(t, s, user, "self", questionnaire.Context{}, false)
			_, err := s.Submit(context.Background(), user.ID, value.ID, submitAI(1))
			expectError(t, err, "invalid_credentials")
			if change == "delete" {
				var count int64
				if err := db.Model(&database.Analysis{}).Where("id = ?", value.ID).Count(&count).Error; err != nil || count != 0 {
					t.Fatal("deleted data resurrected", count, err)
				}
				return
			}
			var persisted database.Analysis
			if err := db.Where("id = ?", value.ID).First(&persisted).Error; err != nil || persisted.Status != "failed" || strings.Contains(string(persisted.Report), `"kind": "ai"`) {
				t.Fatal("new sensitive report persisted after revocation", persisted, err)
			}
		})
	}
}

func TestRecoverStaleProcessingExactlyOnce(t *testing.T) {
	db := testDB(t)
	s := New(testConfig(), db, nil)
	user := testUser(t, db)
	value := draft(t, s, user, "self", questionnaire.Context{}, false)
	old := time.Now().UTC().Add(-10 * time.Minute)
	if err := db.Model(&database.Analysis{}).Where("id = ?", value.ID).Updates(map[string]any{
		"status": "processing", "consent_at": old, "ai_consent_at": old, "processing_started_at": old,
		"scores": database.JSON(`{"preserved":true}`), "report": database.JSON(`{"kind":"deterministic"}`)}).Error; err != nil {
		t.Fatal(err)
	}
	count, err := s.RecoverStale(context.Background())
	if err != nil || count != 1 {
		t.Fatal(count, err)
	}
	result, err := s.Get(context.Background(), user.ID, value.ID)
	if err != nil || result.Status != "failed" || result.FailureCode != "processing_interrupted" || result.Revision != 2 || !strings.Contains(string(result.Scores), "preserved") {
		t.Fatal(result, err)
	}
	count, err = s.RecoverStale(context.Background())
	if err != nil || count != 0 {
		t.Fatal("recovery duplicated operational event", count, err)
	}
	var events int64
	if err := db.Model(&database.UsageEvent{}).Where("analysis_id = ?", value.ID).Count(&events).Error; err != nil || events != 1 {
		t.Fatal(events, err)
	}
}

func TestDeterministicConflictReportLocalizesAndFlagsUncertainSafety(t *testing.T) {
	db := testDB(t)
	s := New(testConfig(), db, nil)
	user := testUser(t, db)
	for _, locale := range []string{"en", "fa"} {
		value, err := s.Create(context.Background(), user.ID, CreateInput{Module: "conflict", Locale: locale,
			Answers: allAnswers(t, "conflict", true), Context: questionnaire.Context{RelationshipType: "friend", Safety: "unknown", Topic: "A scheduling disagreement"}})
		if err != nil {
			t.Fatal(err)
		}
		result, err := s.Submit(context.Background(), user.ID, value.ID, SubmitInput{Revision: 1, Consent: true})
		if err != nil || result.Status != "completed" {
			t.Fatal(result, err)
		}
		var report DeterministicReport
		if err := json.Unmarshal(result.Report, &report); err != nil || report.Safety == nil || report.Safety.Level != "caution" || len(report.Safety.Support) == 0 {
			t.Fatal("uncertain safety lacked a caution", report, err)
		}
		if locale == "fa" && !strings.Contains(report.Title, "نتایج") || locale == "en" && report.Title != "Deterministic reflection results" {
			t.Fatal("deterministic report did not match requested language", report)
		}
	}
}
