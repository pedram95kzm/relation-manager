package questionnaire

import (
	"embed"
	"encoding/json"
	"fmt"
	"net/http"

	"relations/internal/platform"
)

const Version = "1.0.0"
const ScoringVersion = "1.0.0"

//go:embed definitions/*.json
var definitions embed.FS

// Evidence IDs are an allowlist for internal keys and subsequent AI citations.
var evidenceIDs = map[string]bool{
	"bfi2": true, "attachment": true, "emotion_regulation": true,
	"emotion_awareness": true, "communication": true, "conflict_styles": true,
	"relationship_similarity": true,
}

func ValidLocale(locale string) bool { return locale == "fa" || locale == "en" }
func ValidModule(module string) bool {
	return module == "self" || module == "compatibility" || module == "conflict"
}

func Current(module string) (Definition, error) { return Get(module, Version) }

// Get decodes a new value each time so callers cannot mutate the embedded bank.
// Keeping previous files in the catalog supports retrieval of saved versions.
func Get(module, version string) (Definition, error) {
	if !ValidModule(module) || version != Version {
		return Definition{}, platform.Err(http.StatusNotFound, "questionnaire_not_found", "Questionnaire version was not found.")
	}
	data, err := definitions.ReadFile("definitions/" + module + "-" + version + ".json")
	if err != nil {
		return Definition{}, fmt.Errorf("read questionnaire definition: %w", err)
	}
	var q Definition
	if err := platform.StrictJSON(data, &q); err != nil {
		return Definition{}, fmt.Errorf("decode questionnaire definition: %w", err)
	}
	if err := validateDefinition(q); err != nil {
		return Definition{}, err
	}
	return q, nil
}

func validateDefinition(q Definition) error {
	if !ValidModule(q.Module) || q.Version != Version || q.ScoringVersion != ScoringVersion || q.Title.FA == "" || q.Title.EN == "" || len(q.Questions) == 0 {
		return fmt.Errorf("invalid questionnaire catalog metadata")
	}
	dims := make(map[string]int)
	for _, dimension := range q.Dimensions {
		if dimension.ID == "" || dimension.Label.FA == "" || dimension.Label.EN == "" {
			return fmt.Errorf("invalid questionnaire dimension")
		}
		if _, duplicate := dims[dimension.ID]; duplicate {
			return fmt.Errorf("duplicate questionnaire dimension")
		}
		dims[dimension.ID] = 0
		for _, ref := range dimension.References {
			if !evidenceIDs[ref] {
				return fmt.Errorf("unsupported evidence reference")
			}
		}
	}
	seen := make(map[string]bool)
	for _, question := range q.Questions {
		if question.ID == "" || seen[question.ID] || question.Text.FA == "" || question.Text.EN == "" || question.Rationale == "" {
			return fmt.Errorf("invalid questionnaire item")
		}
		if _, ok := dims[question.Dimension]; !ok {
			return fmt.Errorf("unsupported questionnaire dimension")
		}
		seen[question.ID] = true
		dims[question.Dimension]++
		for _, ref := range question.References {
			if !evidenceIDs[ref] {
				return fmt.Errorf("unsupported evidence reference")
			}
		}
	}
	for _, count := range dims {
		if count < 2 {
			return fmt.Errorf("questionnaire dimension needs at least two items")
		}
	}
	return nil
}

func Public(q Definition) PublicDefinition {
	options := []LikertOption{
		{1, Text{FA: "کاملاً مخالفم", EN: "Strongly disagree"}},
		{2, Text{FA: "مخالفم", EN: "Disagree"}},
		{3, Text{FA: "نه موافقم نه مخالفم", EN: "Neither agree nor disagree"}},
		{4, Text{FA: "موافقم", EN: "Agree"}},
		{5, Text{FA: "کاملاً موافقم", EN: "Strongly agree"}},
	}
	public := PublicDefinition{Module: q.Module, Version: q.Version, ScoringVersion: q.ScoringVersion, Title: q.Title,
		Description: q.Description, Questions: make([]PublicQuestion, 0, len(q.Questions)),
		Dimensions: make([]PublicDimension, 0, len(q.Dimensions)), ContextFields: q.ContextFields,
		UnknownLabel: Text{FA: "نمی‌دانم", EN: "I don't know"}}
	for _, question := range q.Questions {
		public.Questions = append(public.Questions, PublicQuestion{ID: question.ID, Group: question.Dimension, Text: question.Text,
			AnswerType: "likert", Options: options, AllowUnknown: true, Required: question.Required})
	}
	for _, dimension := range q.Dimensions {
		public.Dimensions = append(public.Dimensions, PublicDimension{ID: dimension.ID, Label: dimension.Label})
	}
	return public
}

func definitionJSON(q Definition) ([]byte, error) { return json.Marshal(q) }
