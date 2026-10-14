package questionnaire

// Text holds the authoritative wording for both supported locales.
type Text struct {
	FA string `json:"fa"`
	EN string `json:"en"`
}

type Dimension struct {
	ID         string   `json:"id"`
	Label      Text     `json:"label"`
	References []string `json:"references"`
}

// Question is the full immutable backend definition, including scoring keys.
type Question struct {
	ID         string   `json:"id"`
	Dimension  string   `json:"dimension"`
	Text       Text     `json:"text"`
	Reverse    bool     `json:"reverse"`
	Required   bool     `json:"required"`
	Rationale  string   `json:"rationale"`
	References []string `json:"references"`
}

type ContextField struct {
	ID           string   `json:"id"`
	Text         Text     `json:"text"`
	Type         string   `json:"type"`
	Required     bool     `json:"required"`
	AllowUnknown bool     `json:"allow_unknown"`
	Options      []Option `json:"options,omitempty"`
	MaxLength    int      `json:"max_length,omitempty"`
}

type Definition struct {
	Module         string         `json:"module"`
	Version        string         `json:"version"`
	ScoringVersion string         `json:"scoring_version"`
	Title          Text           `json:"title"`
	Description    Text           `json:"description"`
	Questions      []Question     `json:"questions"`
	Dimensions     []Dimension    `json:"dimensions"`
	ContextFields  []ContextField `json:"context_fields"`
}

// A present nil value is an explicit "I don't know" response. An absent key is
// omitted. Pointer integers also reject fractional JSON numbers at decoding.
type Answers map[string]map[string]*int

// Context stays separate from scored Likert answers. Consent/origin records are
// respondent assertions, not proof of partner identity or independent login.
type Context struct {
	RelationshipType      string `json:"relationship_type,omitempty"`
	Safety                string `json:"safety,omitempty"`
	Topic                 string `json:"topic,omitempty"`
	Observations          string `json:"observations,omitempty"`
	Feelings              string `json:"feelings,omitempty"`
	DesiredOutcome        string `json:"desired_outcome,omitempty"`
	ResolutionAttempts    string `json:"resolution_attempts,omitempty"`
	MissingPerspective    string `json:"missing_perspective,omitempty"`
	PartnerResponseOrigin string `json:"partner_response_origin,omitempty"`
	PartnerConsent        bool   `json:"partner_consent,omitempty"`
	presentFields         map[string]bool
}

type Option struct {
	Value string `json:"value"`
	Label Text   `json:"label"`
}

type LikertOption struct {
	Value int  `json:"value"`
	Label Text `json:"label"`
}

type PublicQuestion struct {
	ID           string         `json:"id"`
	Group        string         `json:"group"`
	Text         Text           `json:"text"`
	AnswerType   string         `json:"answer_type"`
	Options      []LikertOption `json:"options"`
	AllowUnknown bool           `json:"allow_unknown"`
	Required     bool           `json:"required"`
}

type PublicDimension struct {
	ID    string `json:"id"`
	Label Text   `json:"label"`
}

type PublicDefinition struct {
	Module         string            `json:"module"`
	Version        string            `json:"version"`
	ScoringVersion string            `json:"scoring_version"`
	Title          Text              `json:"title"`
	Description    Text              `json:"description"`
	Questions      []PublicQuestion  `json:"questions"`
	Dimensions     []PublicDimension `json:"dimensions"`
	ContextFields  []ContextField    `json:"context_fields"`
	UnknownLabel   Text              `json:"unknown_label"`
}
