package ai

import "relations/internal/scoring"

const PromptVersion = "1.0.0"
const ReportSchema = "1.0.0"

type Section struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	Body  string `json:"body"`
}
type ScoreEcho struct {
	Respondent  string   `json:"respondent"`
	DimensionID string   `json:"dimension_id"`
	Score       *float64 `json:"score"`
}
type Safety struct {
	Level   string   `json:"level"`
	Message string   `json:"message"`
	Support []string `json:"support"`
}
type Report struct {
	Kind            string      `json:"kind"`
	SchemaVersion   string      `json:"schema_version"`
	Module          string      `json:"module"`
	Locale          string      `json:"locale"`
	Title           string      `json:"title"`
	Summary         string      `json:"summary"`
	Sections        []Section   `json:"sections"`
	Recommendations []string    `json:"recommendations"`
	Limitations     []string    `json:"limitations"`
	References      []string    `json:"references"`
	ScoreEcho       []ScoreEcho `json:"score_echo"`
	Safety          Safety      `json:"safety"`
}
type Generation struct {
	Report                                       Report
	Provider, Model, PromptVersion, ReportSchema string
	InputTokens, OutputTokens, LatencyMS         int64
}

func echoes(scores scoring.Result) []ScoreEcho {
	result := []ScoreEcho{}
	// Finite respondent order keeps prompts and provider tests reproducible.
	for _, name := range []string{"self", "partner"} {
		for _, d := range scores.Respondents[name].Dimensions {
			result = append(result, ScoreEcho{name, d.ID, d.Score})
		}
	}
	return result
}
