package database

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"time"
)

// JSON keeps JSONB values as JSON in both database operations and API encoding.
// Scan copies driver buffers because they may be reused after the row is read.
type JSON []byte

func (j *JSON) Scan(value any) error {
	if value == nil {
		*j = nil
		return nil
	}
	var data []byte
	switch v := value.(type) {
	case []byte:
		data = v
	case string:
		data = []byte(v)
	default:
		return fmt.Errorf("unsupported JSON database value %T", value)
	}
	if !json.Valid(data) {
		return fmt.Errorf("invalid JSON database value")
	}
	*j = append((*j)[:0], data...)
	return nil
}

func (j JSON) Value() (driver.Value, error) {
	if len(j) == 0 {
		return nil, nil
	}
	if !json.Valid(j) {
		return nil, fmt.Errorf("invalid JSON value")
	}
	return string(j), nil
}

func (j JSON) MarshalJSON() ([]byte, error) {
	if len(j) == 0 {
		return []byte("null"), nil
	}
	if !json.Valid(j) {
		return nil, fmt.Errorf("invalid JSON value")
	}
	return j, nil
}

func (j *JSON) UnmarshalJSON(data []byte) error {
	if !json.Valid(data) {
		return fmt.Errorf("invalid JSON value")
	}
	*j = append((*j)[:0], data...)
	return nil
}

func (JSON) GormDataType() string { return "jsonb" }

type User struct {
	ID           string    `gorm:"type:uuid;primaryKey" json:"id"`
	Email        string    `gorm:"not null" json:"email"`
	Name         string    `gorm:"not null" json:"name"`
	PasswordHash string    `gorm:"not null;default:''" json:"-"`
	Role         string    `gorm:"not null;default:user" json:"role"`
	Age          *int      `json:"age"`
	Gender       string    `gorm:"not null;default:''" json:"gender"`
	Disabled     bool      `gorm:"not null;default:false" json:"disabled"`
	DailyLimit   *int      `json:"daily_limit,omitempty"`
	AuthVersion  int64     `gorm:"not null;default:1" json:"-"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type AuthIdentity struct {
	ID        string    `gorm:"type:uuid;primaryKey" json:"-"`
	UserID    string    `gorm:"type:uuid;not null" json:"-"`
	Provider  string    `gorm:"not null" json:"-"`
	Subject   string    `gorm:"not null" json:"-"`
	CreatedAt time.Time `json:"-"`
}

type RefreshSession struct {
	ID        string     `gorm:"type:uuid;primaryKey" json:"-"`
	UserID    string     `gorm:"type:uuid;not null" json:"-"`
	FamilyID  string     `gorm:"type:uuid;not null" json:"-"`
	TokenHash string     `gorm:"not null" json:"-"`
	ExpiresAt time.Time  `gorm:"not null" json:"-"`
	UsedAt    *time.Time `json:"-"`
	RevokedAt *time.Time `json:"-"`
	CreatedAt time.Time  `json:"-"`
}

// OAuthState stores only a state hash. The short-lived PKCE verifier is needed
// for the server-side token exchange and is never exposed through JSON or logs.
type OAuthState struct {
	StateHash string     `gorm:"primaryKey" json:"-"`
	Verifier  string     `gorm:"not null" json:"-"`
	NonceHash string     `gorm:"not null" json:"-"`
	ExpiresAt time.Time  `gorm:"not null" json:"-"`
	UsedAt    *time.Time `json:"-"`
	CreatedAt time.Time  `json:"-"`
}

func (OAuthState) TableName() string { return "oauth_states" }

type Questionnaire struct {
	Module         string    `gorm:"primaryKey" json:"module"`
	Version        string    `gorm:"primaryKey" json:"version"`
	ScoringVersion string    `gorm:"not null" json:"scoring_version"`
	Definition     JSON      `gorm:"type:jsonb;not null" json:"definition"`
	CreatedAt      time.Time `json:"created_at"`
}

type Analysis struct {
	ID                   string     `gorm:"type:uuid;primaryKey" json:"id"`
	UserID               string     `gorm:"type:uuid;not null" json:"-"`
	Module               string     `gorm:"not null" json:"module"`
	Locale               string     `gorm:"not null" json:"locale"`
	QuestionnaireVersion string     `gorm:"not null" json:"questionnaire_version"`
	ScoringVersion       string     `gorm:"not null" json:"scoring_version"`
	Status               string     `gorm:"not null;default:draft" json:"status"`
	Revision             int64      `gorm:"not null;default:1" json:"revision"`
	Answers              JSON       `gorm:"type:jsonb;not null;default:'{}'" json:"answers"`
	Context              JSON       `gorm:"type:jsonb;not null;default:'{}'" json:"context"`
	Scores               JSON       `gorm:"type:jsonb;not null;default:'{}'" json:"scores"`
	Report               JSON       `gorm:"type:jsonb" json:"report"`
	ConsentAt            *time.Time `json:"consent_at"`
	AIConsentAt          *time.Time `json:"ai_consent_at"`
	PromptVersion        string     `gorm:"not null;default:''" json:"prompt_version"`
	Provider             string     `gorm:"not null;default:''" json:"provider"`
	Model                string     `gorm:"not null;default:''" json:"model"`
	ReportSchema         string     `gorm:"not null;default:''" json:"report_schema"`
	FailureCode          string     `gorm:"not null;default:''" json:"failure_code,omitempty"`
	ProcessingStartedAt  *time.Time `json:"processing_started_at"`
	CreatedAt            time.Time  `json:"created_at"`
	UpdatedAt            time.Time  `json:"updated_at"`
}

type UsageEvent struct {
	ID           string    `gorm:"type:uuid;primaryKey" json:"id"`
	UserID       *string   `gorm:"type:uuid" json:"user_id,omitempty"`
	AnalysisID   *string   `gorm:"type:uuid" json:"analysis_id,omitempty"`
	Provider     string    `gorm:"not null;default:''" json:"provider"`
	Model        string    `gorm:"not null;default:''" json:"model"`
	Status       string    `gorm:"not null" json:"status"`
	ErrorCode    string    `gorm:"not null;default:''" json:"error_code,omitempty"`
	LatencyMS    int64     `gorm:"not null;default:0" json:"latency_ms"`
	InputTokens  int64     `gorm:"not null;default:0" json:"input_tokens"`
	OutputTokens int64     `gorm:"not null;default:0" json:"output_tokens"`
	CreatedAt    time.Time `json:"created_at"`
}

type DailyUsage struct {
	Day       time.Time `gorm:"type:date;primaryKey" json:"day"`
	ScopeKey  string    `gorm:"primaryKey" json:"scope_key"`
	Counter   int64     `gorm:"not null;default:0" json:"counter"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (DailyUsage) TableName() string { return "daily_usage" }

type ProviderConfig struct {
	ID             int       `gorm:"primaryKey" json:"id"`
	Provider       string    `gorm:"not null" json:"provider"`
	Model          string    `gorm:"not null" json:"model"`
	EncryptedKey   []byte    `gorm:"type:bytea" json:"-"`
	MaxTokens      int       `gorm:"not null;default:4096" json:"max_tokens"`
	TimeoutSeconds int       `gorm:"not null;default:60" json:"timeout_seconds"`
	Retries        int       `gorm:"not null;default:1" json:"retries"`
	Active         bool      `gorm:"not null;default:false" json:"active"`
	UpdatedBy      *string   `gorm:"type:uuid" json:"-"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

func (ProviderConfig) TableName() string { return "provider_config" }

type AppSettings struct {
	ID          int       `gorm:"primaryKey" json:"id"`
	GlobalDaily int       `gorm:"not null" json:"global_daily"`
	UserDaily   int       `gorm:"not null" json:"user_daily"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type AuditEvent struct {
	ID           string    `gorm:"type:uuid;primaryKey" json:"id"`
	ActorID      *string   `gorm:"type:uuid" json:"actor_id,omitempty"`
	TargetUserID *string   `gorm:"type:uuid" json:"target_user_id,omitempty"`
	Action       string    `gorm:"not null" json:"action"`
	Metadata     JSON      `gorm:"type:jsonb;not null;default:'{}'" json:"metadata"`
	CreatedAt    time.Time `json:"created_at"`
}
