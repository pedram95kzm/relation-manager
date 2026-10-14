package analysis

import (
	"context"
	"time"

	"relations/internal/ai"
	"relations/internal/questionnaire"
	"relations/internal/scoring"
)

// Generator keeps paid network requests outside database transactions.
type Generator interface {
	Available(context.Context) (bool, error)
	Generate(context.Context, string, string, scoring.Result, questionnaire.Context) (ai.Generation, error)
}

// ExampleGenerator has no database or network side effects. Explicit local
// requests use this method even if provider configuration changes meanwhile.
type ExampleGenerator interface {
	GenerateExample(context.Context, string, string, scoring.Result, questionnaire.Context) (ai.Generation, error)
}

type CreateInput struct {
	Module               string                `json:"module"`
	Locale               string                `json:"locale"`
	QuestionnaireVersion string                `json:"questionnaire_version,omitempty"`
	Answers              questionnaire.Answers `json:"answers,omitempty"`
	Context              questionnaire.Context `json:"context,omitempty"`
}

// Updates replace complete answer/context snapshots. The expected revision
// prevents a second browser tab from silently overwriting newer responses.
type UpdateInput struct {
	Revision int64                  `json:"revision"`
	Locale   string                 `json:"locale,omitempty"`
	Answers  questionnaire.Answers  `json:"answers"`
	Context  *questionnaire.Context `json:"context"`
}

type SubmitInput struct {
	Revision       int64 `json:"revision"`
	Consent        bool  `json:"consent"`
	GenerateReport bool  `json:"generate_report"`
	AIConsent      bool  `json:"ai_consent"`
	LocalExample   bool  `json:"local_example"`
}

type Summary struct {
	ID                   string    `json:"id"`
	Module               string    `json:"module"`
	Locale               string    `json:"locale"`
	QuestionnaireVersion string    `json:"questionnaire_version"`
	ScoringVersion       string    `json:"scoring_version"`
	Status               string    `json:"status"`
	Revision             int64     `json:"revision"`
	FailureCode          string    `json:"failure_code,omitempty"`
	CreatedAt            time.Time `json:"created_at"`
	UpdatedAt            time.Time `json:"updated_at"`
}

type Usage struct {
	Day             string `json:"day"`
	UserUsed        int64  `json:"user_used"`
	UserLimit       int    `json:"user_limit"`
	GlobalUsed      int64  `json:"global_used"`
	GlobalLimit     int    `json:"global_limit"`
	ReportAvailable bool   `json:"report_available"`
	Provider        string `json:"provider"`
	IsExample       bool   `json:"is_example"`
}

type DeterministicReport struct {
	Kind          string     `json:"kind"`
	Source        string     `json:"source"`
	SchemaVersion string     `json:"schema_version"`
	Module        string     `json:"module"`
	Locale        string     `json:"locale"`
	Title         string     `json:"title"`
	Summary       string     `json:"summary"`
	Limitations   []string   `json:"limitations"`
	AIAvailable   bool       `json:"ai_available"`
	Safety        *ai.Safety `json:"safety,omitempty"`
}
