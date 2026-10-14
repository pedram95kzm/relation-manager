package questionnaire

import (
	"net/http"
	"strings"
	"unicode/utf8"

	"relations/internal/platform"
)

func invalidAnswers() error {
	return platform.Err(http.StatusBadRequest, "invalid_answers", "Questionnaire answers do not match the selected version.")
}
func invalidContext() error {
	return platform.Err(http.StatusBadRequest, "invalid_context", "Questionnaire context is incomplete or invalid.")
}

// Validate permits omitted answers in drafts, while final submission requires
// an explicit numeric or unknown choice for each required item.
func Validate(q Definition, answers Answers, ctx Context, final bool) error {
	if !ValidModule(q.Module) || q.Version != Version || q.ScoringVersion != ScoringVersion {
		return invalidAnswers()
	}
	questions := make(map[string]Question, len(q.Questions))
	for _, question := range q.Questions {
		questions[question.ID] = question
	}
	for respondent, values := range answers {
		if respondent != "self" && (respondent != "partner" || q.Module != "compatibility") || values == nil {
			return invalidAnswers()
		}
		for id, value := range values {
			if _, ok := questions[id]; !ok || value != nil && (*value < 1 || *value > 5) {
				return invalidAnswers()
			}
		}
	}
	if final {
		respondents := []string{"self"}
		if q.Module == "compatibility" {
			respondents = append(respondents, "partner")
		}
		for _, respondent := range respondents {
			for _, question := range q.Questions {
				if _, present := answers[respondent][question.ID]; question.Required && !present {
					return platform.Err(http.StatusBadRequest, "answers_incomplete", "Select an answer or I don't know for every required question.")
				}
			}
		}
	}
	return ValidateContext(q.Module, ctx, final)
}

func ValidateContext(module string, ctx Context, final bool) error {
	if !ValidModule(module) {
		return invalidContext()
	}
	for field := range ctx.presentFields {
		partnerField := field == "partner_response_origin" || field == "partner_consent"
		if partnerField && module != "compatibility" || !partnerField && module != "conflict" {
			return invalidContext()
		}
	}
	conflictFieldsPresent := ctx.RelationshipType != "" || ctx.Safety != "" || ctx.Topic != "" || ctx.Observations != "" ||
		ctx.Feelings != "" || ctx.DesiredOutcome != "" || ctx.ResolutionAttempts != "" || ctx.MissingPerspective != ""
	partnerFieldsPresent := ctx.PartnerResponseOrigin != "" || ctx.PartnerConsent
	if module != "conflict" && conflictFieldsPresent || module != "compatibility" && partnerFieldsPresent {
		return invalidContext()
	}
	if module == "compatibility" {
		if ctx.PartnerResponseOrigin != "" && ctx.PartnerResponseOrigin != "independent" && ctx.PartnerResponseOrigin != "proxy" {
			return invalidContext()
		}
		if final && ctx.PartnerResponseOrigin == "" {
			return invalidContext()
		}
		// Lack of partner sharing consent is preserved so the scoring layer can
		// explicitly suppress a dyadic index instead of fabricating one.
		return nil
	}
	if module != "conflict" {
		return nil
	}
	for _, value := range []string{ctx.Topic, ctx.Observations, ctx.Feelings, ctx.DesiredOutcome, ctx.ResolutionAttempts, ctx.MissingPerspective} {
		if !utf8.ValidString(value) || utf8.RuneCountInString(value) > 2000 || strings.ContainsRune(value, '\x00') {
			return invalidContext()
		}
	}
	if ctx.RelationshipType != "" {
		switch ctx.RelationshipType {
		case "colleague", "parent", "friend", "partner", "other":
		default:
			return invalidContext()
		}
	}
	if ctx.Safety != "" && ctx.Safety != "yes" && ctx.Safety != "no" && ctx.Safety != "unknown" {
		return invalidContext()
	}
	if final && (ctx.RelationshipType == "" || ctx.Safety == "" || strings.TrimSpace(ctx.Topic) == "") {
		return invalidContext()
	}
	return nil
}
