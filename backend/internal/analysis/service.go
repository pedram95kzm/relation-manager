package analysis

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"relations/internal/ai"
	"relations/internal/config"
	"relations/internal/database"
	"relations/internal/platform"
	"relations/internal/questionnaire"
	"relations/internal/scoring"
)

type Service struct {
	db        *gorm.DB
	cfg       config.Config
	generator Generator
	now       func() time.Time
}

func New(cfg config.Config, db *gorm.DB, generator Generator) *Service {
	return &Service{db: db, cfg: cfg, generator: generator, now: time.Now}
}

func notFound() error {
	return platform.Err(http.StatusNotFound, "analysis_not_found", "Analysis not found.")
}

func revisionConflict() error {
	return platform.Err(http.StatusConflict, "revision_conflict", "The analysis changed. Reload it before saving or submitting.")
}

func jsonValue(value any) (database.JSON, error) {
	data, err := json.Marshal(value)
	return database.JSON(data), err
}

func (s *Service) lockedUser(tx *gorm.DB, userID string, requireProfile bool) (database.User, error) {
	var user database.User
	if _, err := uuid.Parse(userID); err != nil {
		return user, platform.Err(401, "invalid_credentials", "Authentication is required.")
	}
	err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", userID).First(&user).Error
	if errors.Is(err, gorm.ErrRecordNotFound) || err == nil && user.Disabled {
		return user, platform.Err(401, "invalid_credentials", "Authentication is required.")
	}
	if err != nil {
		return user, err
	}
	minimumAge := max(18, s.cfg.MinAge)
	if requireProfile && (user.Age == nil || *user.Age < minimumAge || *user.Age > 120 || user.Gender == "") {
		return user, platform.Err(403, "profile_required", "Complete the required eligible age and gender profile first.")
	}
	return user, nil
}

func owned(tx *gorm.DB, userID, id string, lock bool) (database.Analysis, error) {
	var value database.Analysis
	if _, err := uuid.Parse(id); err != nil {
		return value, notFound()
	}
	query := tx.Where("id = ? AND user_id = ?", id, userID)
	if lock {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	err := query.First(&value).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return value, notFound()
	}
	return value, err
}

func (s *Service) Create(ctx context.Context, userID string, input CreateInput) (database.Analysis, error) {
	var value database.Analysis
	if input.Locale == "" {
		input.Locale = "fa"
	}
	if !questionnaire.ValidLocale(input.Locale) {
		return value, platform.Err(400, "invalid_locale", "Select Persian or English.")
	}
	if input.QuestionnaireVersion == "" {
		input.QuestionnaireVersion = questionnaire.Version
	}
	definition, err := questionnaire.Get(input.Module, input.QuestionnaireVersion)
	if err != nil {
		return value, err
	}
	if input.Answers == nil {
		input.Answers = questionnaire.Answers{}
	}
	if err = questionnaire.Validate(definition, input.Answers, input.Context, false); err != nil {
		return value, err
	}
	answers, err := jsonValue(input.Answers)
	if err != nil {
		return value, err
	}
	contextData, err := jsonValue(input.Context)
	if err != nil {
		return value, err
	}
	now := s.now().UTC()
	value = database.Analysis{ID: uuid.NewString(), UserID: userID, Module: definition.Module, Locale: input.Locale,
		QuestionnaireVersion: definition.Version, ScoringVersion: definition.ScoringVersion, Status: "draft", Revision: 1,
		Answers: answers, Context: contextData, Scores: database.JSON(`{}`), CreatedAt: now, UpdatedAt: now}
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if _, err := s.lockedUser(tx, userID, true); err != nil {
			return err
		}
		var drafts int64
		if err := tx.Model(&database.Analysis{}).Where("user_id = ? AND status = 'draft'", userID).Count(&drafts).Error; err != nil {
			return err
		}
		if drafts >= 100 {
			return platform.Err(409, "draft_limit", "Delete or submit an existing draft before creating another.")
		}
		return tx.Create(&value).Error
	})
	return value, err
}

func (s *Service) Get(ctx context.Context, userID, id string) (database.Analysis, error) {
	return owned(s.db.WithContext(ctx), userID, id, false)
}

func (s *Service) Update(ctx context.Context, userID, id string, input UpdateInput) (database.Analysis, error) {
	var value database.Analysis
	if input.Revision < 1 || input.Answers == nil || input.Context == nil {
		return value, platform.Err(400, "invalid_draft", "Supply the current revision, answers object and context object.")
	}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if _, err := s.lockedUser(tx, userID, true); err != nil {
			return err
		}
		var err error
		value, err = owned(tx, userID, id, true)
		if err != nil {
			return err
		}
		if value.Revision != input.Revision {
			return revisionConflict()
		}
		if value.Status != "draft" && value.Status != "failed" && value.Status != "insufficient" {
			return platform.Err(409, "analysis_locked", "Processing and completed analyses cannot be edited.")
		}
		locale := value.Locale
		if input.Locale != "" {
			locale = input.Locale
		}
		if !questionnaire.ValidLocale(locale) {
			return platform.Err(400, "invalid_locale", "Select Persian or English.")
		}
		definition, err := questionnaire.Get(value.Module, value.QuestionnaireVersion)
		if err != nil {
			return err
		}
		if err = questionnaire.Validate(definition, input.Answers, *input.Context, false); err != nil {
			return err
		}
		answers, err := jsonValue(input.Answers)
		if err != nil {
			return err
		}
		contextData, err := jsonValue(input.Context)
		if err != nil {
			return err
		}
		updates := map[string]any{"answers": answers, "context": contextData, "locale": locale, "revision": value.Revision + 1,
			"status": "draft", "scores": database.JSON(`{}`), "report": nil, "consent_at": nil, "ai_consent_at": nil, "processing_started_at": nil,
			"failure_code": "", "provider": "", "model": "", "prompt_version": "", "report_schema": "", "updated_at": s.now().UTC()}
		if err = tx.Model(&database.Analysis{}).Where("id = ? AND user_id = ?", id, userID).Updates(updates).Error; err != nil {
			return err
		}
		value, err = owned(tx, userID, id, false)
		return err
	})
	return value, err
}

func (s *Service) Delete(ctx context.Context, userID, id string) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if _, err := s.lockedUser(tx, userID, false); err != nil {
			return err
		}
		value, err := owned(tx, userID, id, true)
		if err != nil {
			return err
		}
		if value.Status == "processing" {
			return platform.Err(409, "analysis_processing", "Wait for processing to finish before deleting this analysis.")
		}
		return tx.Where("id = ? AND user_id = ?", id, userID).Delete(&database.Analysis{}).Error
	})
}

func (s *Service) List(ctx context.Context, userID string, page, size int, module, status string) (platform.Page[Summary], error) {
	result := platform.Page[Summary]{Items: []Summary{}, Page: page, PageSize: size}
	if page < 1 || page > 100000 || size < 1 || size > 100 || module != "" && !questionnaire.ValidModule(module) || status != "" && !validStatus(status) {
		return result, platform.Err(400, "invalid_filter", "Select a supported analysis filter or page.")
	}
	query := s.db.WithContext(ctx).Model(&database.Analysis{}).Where("user_id = ?", userID)
	if module != "" {
		query = query.Where("module = ?", module)
	}
	if status != "" {
		query = query.Where("status = ?", status)
	}
	if err := query.Count(&result.Total).Error; err != nil {
		return result, err
	}
	err := query.Select("id", "module", "locale", "questionnaire_version", "scoring_version", "status", "revision", "failure_code", "created_at", "updated_at").
		Order("created_at DESC, id DESC").Offset((page - 1) * size).Limit(size).Scan(&result.Items).Error
	return result, err
}

func validStatus(status string) bool {
	return status == "draft" || status == "processing" || status == "completed" || status == "failed" || status == "insufficient"
}

func deterministic(value database.Analysis, input questionnaire.Context, available bool) (database.JSON, error) {
	report := DeterministicReport{Kind: "deterministic", Source: "deterministic", SchemaVersion: "1.0.0",
		Module: value.Module, Locale: value.Locale, Title: "Deterministic reflection results",
		Summary: "Scores are computed directly from your numeric answers. No AI narrative was generated. Review the dimension scores, completeness and unknown responses together.",
		Limitations: []string{"For educational reflection only; this is not a diagnosis or treatment recommendation.",
			"Original questions are not clinically validated instruments. Completeness is not statistical confidence.",
			"Unknown responses remain unknown. Similarity is not a prediction or probability of relationship success."}, AIAvailable: available}
	if value.Module == "conflict" && (input.Safety == "no" || input.Safety == "unknown") {
		report.Safety = &ai.Safety{Level: "caution", Message: "You indicated that safety is uncertain or absent. This questionnaire cannot assess danger.",
			Support: []string{"Prioritize your personal safety before discussing the conflict.", "Consider support from a trusted person or local professional."}}
	}
	if value.Locale == "fa" {
		report.Title = "نتایج محاسبه‌شده برای خوداندیشی"
		report.Summary = "امتیازها مستقیماً از پاسخ‌های عددی شما محاسبه شده‌اند. متن هوش مصنوعی تولید نشده است. امتیاز هر بُعد را همراه با میزان تکمیل و پاسخ‌های نامشخص بررسی کنید."
		report.Limitations = []string{"این نتایج فقط برای تأمل و آموزش هستند و توصیهٔ تشخیصی یا درمانی نیستند.",
			"پرسش‌های تألیفی ابزار بالینی اعتبارسنجی‌شده نیستند. میزان تکمیل، اطمینان آماری نیست.",
			"پاسخ‌های نامشخص، نامشخص باقی می‌مانند. شباهت، پیش‌بینی یا احتمال موفقیت رابطه نیست."}
		if report.Safety != nil {
			report.Safety.Message = "شما امنیت را نامشخص یا برقرارنبوده اعلام کرده‌اید. این پرسش‌نامه نمی‌تواند خطر را ارزیابی کند."
			report.Safety.Support = []string{"پیش از گفت‌وگو دربارهٔ اختلاف، امنیت شخصی خود را در اولویت قرار دهید.", "کمک گرفتن از فردی مورد اعتماد یا متخصص محلی را در نظر بگیرید."}
		}
	}
	return jsonValue(report)
}

func (s *Service) Submit(ctx context.Context, userID, id string, input SubmitInput) (database.Analysis, error) {
	var value database.Analysis
	if input.Revision < 1 {
		return value, platform.Err(400, "invalid_revision", "Supply the current analysis revision.")
	}
	if !input.Consent {
		return value, platform.Err(400, "consent_required", "Confirm consent to process this educational analysis.")
	}
	if input.LocalExample && !input.GenerateReport {
		return value, platform.Err(400, "invalid_request", "Local examples require generate_report to be enabled.")
	}
	if input.GenerateReport && !input.LocalExample && !input.AIConsent {
		return value, platform.Err(400, "ai_consent_required", "Confirm sharing scores and supplied context with the configured AI provider.")
	}
	// Establish ownership before checking provider state so strangers cannot use
	// someone else's ID to probe quota or configuration behavior.
	if _, err := s.Get(ctx, userID, id); err != nil {
		return value, err
	}
	available := false
	var err error
	exampleGenerator, canGenerateExample := s.generator.(ExampleGenerator)
	if input.LocalExample {
		if !canGenerateExample {
			return value, platform.Err(503, "example_unavailable", "The local example generator is unavailable.")
		}
		available = true
	} else if s.generator != nil {
		available, err = s.generator.Available(ctx)
		if err != nil && input.GenerateReport {
			return value, err
		}
		// A provider-config problem must not prevent local deterministic scoring.
		if err != nil {
			available = false
		}
	}
	var scores scoring.Result
	var contextInput questionnaire.Context
	generate := false
	var authVersion int64
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		user, err := s.lockedUser(tx, userID, true)
		if err != nil {
			return err
		}
		authVersion = user.AuthVersion
		value, err = owned(tx, userID, id, true)
		if err != nil {
			return err
		}
		if value.Revision != input.Revision {
			return revisionConflict()
		}
		if value.Status != "draft" && value.Status != "failed" && value.Status != "insufficient" {
			return platform.Err(409, "analysis_locked", "This analysis has already been submitted.")
		}
		definition, err := questionnaire.Get(value.Module, value.QuestionnaireVersion)
		if err != nil {
			return err
		}
		var answers questionnaire.Answers
		if err = json.Unmarshal(value.Answers, &answers); err != nil {
			return err
		}
		if err = json.Unmarshal(value.Context, &contextInput); err != nil {
			return err
		}
		if err = questionnaire.Validate(definition, answers, contextInput, true); err != nil {
			return err
		}
		scores, err = scoring.Compute(definition, answers, contextInput)
		if err != nil {
			return err
		}
		value.Scores, err = jsonValue(scores)
		if err != nil {
			return err
		}
		value.Report, err = deterministic(value, contextInput, available)
		if err != nil {
			return err
		}
		now := s.now().UTC()
		value.ConsentAt = &now
		value.AIConsentAt = nil
		value.Status, value.FailureCode = "completed", ""
		value.Provider, value.Model, value.PromptVersion, value.ReportSchema = "", "", "", "deterministic-1.0.0"
		value.ProcessingStartedAt = nil
		value.Revision++
		value.UpdatedAt = now
		if scores.Status != "available" {
			value.Status = "insufficient"
		} else if input.GenerateReport && input.LocalExample {
			// Pure generation inside this transaction never calls a configured
			// external provider, requires sharing consent, or reserves AI quota.
			generation, err := exampleGenerator.GenerateExample(ctx, value.Module, value.Locale, scores, contextInput)
			if err != nil {
				return err
			}
			value.Report, err = jsonValue(generation.Report)
			if err != nil {
				return err
			}
			value.Provider, value.Model = generation.Provider, generation.Model
			value.PromptVersion, value.ReportSchema = generation.PromptVersion, generation.ReportSchema
			if err = tx.Create(&database.UsageEvent{ID: uuid.NewString(), UserID: &value.UserID, AnalysisID: &value.ID,
				Provider: "mock", Model: generation.Model, Status: "success", CreatedAt: now}).Error; err != nil {
				return err
			}
		} else if input.GenerateReport && available {
			if err = s.reserve(tx, user, now); err != nil {
				return err
			}
			value.Status, value.ProcessingStartedAt = "processing", &now
			value.AIConsentAt = &now
			generate = true
		}
		return tx.Model(&database.Analysis{}).Where("id = ? AND user_id = ?", id, userID).Updates(map[string]any{
			"scores": value.Scores, "report": value.Report, "consent_at": value.ConsentAt, "ai_consent_at": value.AIConsentAt, "status": value.Status,
			"revision": value.Revision, "failure_code": value.FailureCode, "processing_started_at": value.ProcessingStartedAt,
			"provider": value.Provider, "model": value.Model, "prompt_version": value.PromptVersion,
			"report_schema": value.ReportSchema, "updated_at": value.UpdatedAt}).Error
	})
	if err != nil || !generate {
		return value, err
	}
	generation, generationErr := s.generator.Generate(ctx, value.Module, value.Locale, scores, contextInput)
	// A disconnected HTTP client must not strand a reserved attempt forever.
	finishCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return s.finish(finishCtx, value, authVersion, generation, generationErr)
}

func (s *Service) finish(ctx context.Context, started database.Analysis, authVersion int64, generation ai.Generation, generationErr error) (database.Analysis, error) {
	var value database.Analysis
	authorizationLost := false
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var user database.User
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", started.UserID).First(&user).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return platform.Err(401, "invalid_credentials", "Authentication is required.")
			}
			return err
		}
		var err error
		value, err = owned(tx, started.UserID, started.ID, true)
		if err != nil {
			return err
		}
		if value.Status != "processing" || value.Revision != started.Revision {
			return platform.Err(409, "analysis_state_changed", "Analysis processing state changed.")
		}
		if user.Disabled {
			authorizationLost = true
			generationErr = platform.Err(401, "account_unavailable", "Authentication is required.")
		} else if user.AuthVersion != authVersion {
			authorizationLost = true
			generationErr = platform.Err(401, "session_changed", "Sign in again to access this analysis.")
		}
		now := s.now().UTC()
		value.Provider, value.Model = generation.Provider, generation.Model
		value.PromptVersion = generation.PromptVersion
		value.Status = "completed"
		value.FailureCode = ""
		eventStatus := "success"
		if generationErr == nil {
			value.Report, err = jsonValue(generation.Report)
			if err != nil {
				return err
			}
			value.ReportSchema = generation.ReportSchema
		} else {
			value.Status, value.FailureCode, eventStatus = "failed", "ai_provider_error", "error"
			var safe *platform.Error
			if errors.As(generationErr, &safe) {
				switch safe.Code {
				case "ai_unavailable", "ai_timeout", "ai_rate_limited", "ai_provider_error", "ai_invalid_report", "session_changed", "account_unavailable":
					value.FailureCode = safe.Code
				}
			}
		}
		value.ProcessingStartedAt, value.UpdatedAt = nil, now
		value.Revision++
		if err = tx.Model(&database.Analysis{}).Where("id = ? AND user_id = ?", value.ID, value.UserID).Updates(map[string]any{
			"status": value.Status, "report": value.Report, "provider": value.Provider, "model": value.Model,
			"prompt_version": value.PromptVersion, "report_schema": value.ReportSchema, "failure_code": value.FailureCode,
			"processing_started_at": nil, "revision": value.Revision, "updated_at": now}).Error; err != nil {
			return err
		}
		return tx.Create(&database.UsageEvent{ID: uuid.NewString(), UserID: &value.UserID, AnalysisID: &value.ID,
			Provider: generation.Provider, Model: generation.Model, Status: eventStatus, ErrorCode: value.FailureCode,
			LatencyMS: max(0, generation.LatencyMS), InputTokens: max(0, generation.InputTokens), OutputTokens: max(0, generation.OutputTokens), CreatedAt: now}).Error
	})
	if authorizationLost && err == nil {
		return database.Analysis{}, platform.Err(401, "invalid_credentials", "Authentication is required.")
	}
	return value, err
}
