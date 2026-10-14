package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"relations/internal/database"
	"relations/internal/platform"
	"relations/internal/questionnaire"
	"relations/internal/scoring"
)

const MockProvider = "mock"
const MockModel = "local-example-v1"
const MockPromptVersion = "local-example-1.0.0"

// GenerateExample is pure and stays local even if provider settings change.
// It is safe to call while reserving an analysis transaction: no database,
// credential lookup, quota reservation or HTTP request occurs here.
func (s *Service) GenerateExample(ctx context.Context, module, locale string, scores scoring.Result, input questionnaire.Context) (Generation, error) {
	gen := Generation{Provider: MockProvider, Model: MockModel, PromptVersion: MockPromptVersion, ReportSchema: ReportSchema}
	if err := ctx.Err(); err != nil {
		return gen, err
	}
	if !questionnaire.ValidModule(module) || !questionnaire.ValidLocale(locale) || scores.Module != module || scores.QuestionnaireVersion != questionnaire.Version || scores.ScoringVersion != questionnaire.ScoringVersion {
		return gen, platform.Err(400, "invalid_analysis", "The module, report language or deterministic scores are invalid.")
	}
	var err error
	gen.Report, err = exampleReport(module, locale, scores, input)
	return gen, err
}

type PreviewInput struct {
	Module  string                 `json:"module"`
	Locale  string                 `json:"locale"`
	Answers questionnaire.Answers  `json:"answers,omitempty"`
	Context *questionnaire.Context `json:"context,omitempty"`
}

func (input *PreviewInput) UnmarshalJSON(data []byte) error {
	if bytes.Equal(bytes.TrimSpace(data), []byte("null")) {
		return fmt.Errorf("preview must be an object")
	}
	type plain PreviewInput
	var value plain
	if err := platform.StrictJSON(data, &value); err != nil {
		return err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	for _, key := range []string{"answers", "context"} {
		if raw, ok := fields[key]; ok && bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
			return fmt.Errorf("preview data must be objects")
		}
	}
	*input = PreviewInput(value)
	return nil
}

type PreviewResult struct {
	Provider          string         `json:"provider"`
	Model             string         `json:"model"`
	IsExample         bool           `json:"is_example"`
	ExternalRequest   bool           `json:"external_request"`
	UsedSampleAnswers bool           `json:"used_sample_answers"`
	UsedSampleContext bool           `json:"used_sample_context"`
	Scores            scoring.Result `json:"scores"`
	Report            Report         `json:"report"`
	PromptVersion     string         `json:"prompt_version"`
	ReportSchema      string         `json:"report_schema"`
	LatencyMS         int64          `json:"latency_ms"`
}

func preview(input PreviewInput) (PreviewResult, error) {
	result := PreviewResult{Provider: MockProvider, Model: MockModel, IsExample: true, PromptVersion: MockPromptVersion, ReportSchema: ReportSchema}
	if input.Locale == "" {
		input.Locale = "fa"
	}
	if !questionnaire.ValidLocale(input.Locale) {
		return result, platform.Err(400, "invalid_locale", "Select Persian or English.")
	}
	def, err := questionnaire.Get(input.Module, questionnaire.Version)
	if err != nil {
		return result, err
	}
	answers := input.Answers
	if answers == nil {
		result.UsedSampleAnswers = true
		answers = questionnaire.Answers{"self": {}}
		if input.Module == "compatibility" {
			answers["partner"] = map[string]*int{}
		}
		for _, respondent := range []string{"self", "partner"} {
			if values, ok := answers[respondent]; ok {
				for _, question := range def.Questions {
					value := 3
					values[question.ID] = &value
				}
			}
		}
	}
	inputContext := questionnaire.Context{}
	if input.Context != nil {
		inputContext = *input.Context
	} else {
		result.UsedSampleContext = true
		switch input.Module {
		case "compatibility":
			inputContext = questionnaire.Context{PartnerResponseOrigin: "independent", PartnerConsent: true}
		case "conflict":
			inputContext = questionnaire.Context{RelationshipType: "friend", Safety: "unknown", Topic: "A sample scheduling disagreement"}
		}
	}
	if err = questionnaire.Validate(def, answers, inputContext, false); err != nil {
		return result, err
	}
	result.Scores, err = scoring.Compute(def, answers, inputContext)
	if err != nil {
		return result, err
	}
	result.Report, err = exampleReport(input.Module, input.Locale, result.Scores, inputContext)
	return result, err
}

// Preview always uses the local path. It never reads provider settings or
// decrypts credentials, and supplied answers/context never enter audit metadata.
func (s *Service) Preview(ctx context.Context, actor string, input PreviewInput) (PreviewResult, error) {
	if err := ctx.Err(); err != nil {
		return PreviewResult{}, err
	}
	result, err := preview(input)
	if err != nil {
		return result, err
	}
	metadata, _ := json.Marshal(map[string]any{"module": result.Report.Module, "locale": result.Report.Locale, "provider": MockProvider, "is_example": true, "used_sample_answers": result.UsedSampleAnswers, "used_sample_context": result.UsedSampleContext})
	err = s.db.WithContext(ctx).Create(&database.AuditEvent{ID: uuid.NewString(), ActorID: &actor, Action: "provider.previewed", Metadata: metadata}).Error
	if err != nil {
		return PreviewResult{}, err
	}
	return result, nil
}

func exampleReport(module, locale string, scores scoring.Result, input questionnaire.Context) (Report, error) {
	report := Report{Kind: "example", SchemaVersion: ReportSchema, Module: module, Locale: locale, ScoreEcho: echoes(scores), References: []string{},
		Title: "Local example report", Summary: "This is a locally generated example, not an AI interpretation. No prompt or personal data was sent to an external provider. The score echoes copy the actual deterministic calculation; the narrative below is illustrative.",
		Recommendations: []string{"Review numeric completeness and unknown answers before interpreting a dimension.", "Choose one observation to reflect on; this example cannot assess your circumstances."},
		Limitations:     []string{"Example narrative generated by local code; no AI model evaluated your answers.", "Original questions are not validated psychological instruments; scores are descriptive, not diagnostic.", "Completeness is not confidence; similarity is not a prediction or probability of relationship success.", "Persian and English measurement equivalence has not been established."},
		Safety:          Safety{Level: "routine", Message: "This example does not evaluate danger or provide clinical advice.", Support: []string{}}}
	if locale == "fa" {
		report.Title = "گزارش نمونهٔ محلی"
		report.Summary = "این یک نمونهٔ تولیدشده در همین سامانه است و تفسیر هوش مصنوعی نیست. هیچ دستور یا دادهٔ شخصی به ارائه‌دهندهٔ بیرونی فرستاده نشده است. امتیازهای بازتاب‌یافته از محاسبهٔ واقعی گرفته شده‌اند؛ متن زیر صرفاً نمونه است."
		report.Recommendations = []string{"پیش از تفسیر هر بُعد، میزان تکمیل و پاسخ‌های نامشخص را بررسی کنید.", "یک مشاهده را برای تأمل انتخاب کنید؛ این نمونه نمی‌تواند شرایط شما را ارزیابی کند."}
		report.Limitations = []string{"متن نمونه با کد محلی ساخته شده است؛ هیچ مدل هوش مصنوعی پاسخ‌های شما را ارزیابی نکرده است.", "پرسش‌های تألیفی ابزار روان‌شناختی اعتبارسنجی‌شده نیستند؛ امتیازها توصیفی‌اند و تشخیص نیستند.", "تکمیل پاسخ‌ها به معنای اطمینان نیست؛ شباهت پیش‌بینی یا احتمال موفقیت رابطه نیست.", "هم‌ارزی سنجش در زبان فارسی و انگلیسی اثبات نشده است."}
		report.Safety.Message = "این نمونه خطر را ارزیابی نمی‌کند و توصیهٔ بالینی نمی‌دهد."
	}
	dimensions := []string{}
	for _, respondent := range []string{"self", "partner"} {
		for _, dimension := range scores.Respondents[respondent].Dimensions {
			label := dimension.Label.EN
			status := "numeric score available"
			if dimension.Score == nil {
				status = "unknown or insufficient numeric answers"
			}
			if locale == "fa" {
				label = dimension.Label.FA
				status = "امتیاز عددی موجود است"
				if dimension.Score == nil {
					status = "پاسخ‌های عددی نامشخص یا ناکافی‌اند"
				}
			}
			dimensions = append(dimensions, label+": "+status)
		}
	}
	patternTitle := "Observed data availability"
	if locale == "fa" {
		patternTitle = "وضعیت داده‌های موجود"
	}
	report.Sections = []Section{{ID: "data", Title: patternTitle, Body: strings.Join(dimensions, "\n")}}
	switch module {
	case "self":
		section := Section{ID: "reflection", Title: "Example reflection", Body: "An example next step is to notice how your communication changes across situations. This text is a general demonstration, not an assessment based on your answers."}
		if locale == "fa" {
			section.Title = "نمونهٔ تأمل"
			section.Body = "یک گام نمونه، توجه به تغییر شیوهٔ ارتباط در موقعیت‌های مختلف است. این متن نمایش عمومی است و ارزیابی مبتنی بر پاسخ‌های شما نیست."
		}
		report.Sections = append(report.Sections, section)
	case "compatibility":
		body := "When independent responses and sharing consent are asserted, the displayed index describes arithmetic similarity across sufficient shared dimensions. This example cannot establish relationship quality or verified partner identity."
		if scores.Compatibility == nil || scores.Compatibility.Index == nil {
			body = "The similarity index is withheld because its consent, response-origin, or sufficient-data requirements were not met. Individual dimensions remain available without an invented dyadic index."
		}
		title := "Example comparison"
		if locale == "fa" {
			title = "نمونهٔ مقایسه"
			body = "اگر پاسخ‌های مستقل و رضایت اعلام شده باشند، شاخص نمایش‌داده‌شده شباهت عددی ابعاد دارای دادهٔ کافی را توصیف می‌کند. این نمونه کیفیت رابطه یا هویت طرف مقابل را اثبات نمی‌کند."
			if scores.Compatibility == nil || scores.Compatibility.Index == nil {
				body = "شاخص شباهت به دلیل برقرارنبودن شرط رضایت، منشأ پاسخ یا دادهٔ کافی نمایش داده نمی‌شود. ابعاد فردی بدون ساختن شاخص دونفره باقی می‌مانند."
			}
		}
		report.Sections = append(report.Sections, Section{ID: "comparison", Title: title, Body: body})
	case "conflict":
		title := "Example next step"
		body := "If you feel safe, consider separating an observation from your interpretation and choosing a small, optional next step. This sample does not determine motives, blame, or the other person's perspective."
		if locale == "fa" {
			title = "نمونهٔ گام بعدی"
			body = "اگر احساس امنیت دارید، مشاهده را از برداشت خود جدا کنید و یک گام کوچک و اختیاری انتخاب کنید. این نمونه دربارهٔ انگیزه، مقصر یا دیدگاه طرف دیگر داوری نمی‌کند."
		}
		if input.Safety != "yes" {
			body = "Safety is absent or uncertain. Prioritize your safety and consider trusted local support; this example does not recommend confrontation or a conversation script."
			report.Safety = Safety{Level: "caution", Message: "You reported absent or uncertain safety. This example cannot evaluate danger.", Support: []string{"Prioritize personal safety before discussing the conflict.", "Consider support from a trusted person or local professional."}}
			if locale == "fa" {
				body = "امنیت برقرار نیست یا نامشخص است. امنیت خود را در اولویت بگذارید و کمک مورداعتماد محلی را در نظر بگیرید؛ این نمونه رویارویی یا متن گفت‌وگو پیشنهاد نمی‌کند."
				report.Safety = Safety{Level: "caution", Message: "شما امنیت را نامشخص یا برقرارنبود‌ه اعلام کرده‌اید. این نمونه نمی‌تواند خطر را ارزیابی کند.", Support: []string{"پیش از گفت‌وگو دربارهٔ اختلاف، امنیت شخصی را در اولویت قرار دهید.", "کمک گرفتن از فردی مورداعتماد یا متخصص محلی را در نظر بگیرید."}}
			}
		}
		report.Sections = append(report.Sections, Section{ID: "next_steps", Title: title, Body: body})
	}
	data, err := json.Marshal(report)
	if err != nil {
		return Report{}, err
	}
	return validateReportKind(data, module, locale, scores, input, "example")
}
