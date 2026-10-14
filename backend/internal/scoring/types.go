package scoring

import "relations/internal/questionnaire"

type DimensionScore struct {
	ID           string             `json:"id"`
	Label        questionnaire.Text `json:"label"`
	Score        *float64           `json:"score"`
	Status       string             `json:"status"`
	Band         *string            `json:"band"`
	Answered     int                `json:"answered"`
	Total        int                `json:"total"`
	Unknown      int                `json:"unknown"`
	Omitted      int                `json:"omitted"`
	Completeness float64            `json:"completeness"`
}

type RespondentScores struct {
	Dimensions          []DimensionScore `json:"dimensions"`
	Answered            int              `json:"answered"`
	Total               int              `json:"total"`
	Unknown             int              `json:"unknown"`
	Omitted             int              `json:"omitted"`
	NumericCompleteness float64          `json:"numeric_completeness"`
}

type SimilarityComponent struct {
	DimensionID string  `json:"dimension_id"`
	Similarity  float64 `json:"similarity"`
}

type Similarity struct {
	Index              *float64              `json:"index"`
	Status             string                `json:"status"`
	Components         []SimilarityComponent `json:"components"`
	IncludedDimensions []string              `json:"included_dimensions"`
	OmittedDimensions  []string              `json:"omitted_dimensions"`
	Coverage           float64               `json:"coverage"`
	Formula            string                `json:"formula"`
}

type Result struct {
	Module               string                      `json:"module"`
	QuestionnaireVersion string                      `json:"questionnaire_version"`
	ScoringVersion       string                      `json:"scoring_version"`
	Status               string                      `json:"status"`
	Respondents          map[string]RespondentScores `json:"respondents"`
	Compatibility        *Similarity                 `json:"compatibility,omitempty"`
}
