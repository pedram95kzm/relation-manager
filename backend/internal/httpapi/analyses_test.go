package httpapi

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"strings"
	"testing"

	"github.com/google/uuid"
	"relations/internal/auth"
	"relations/internal/database"
	"relations/internal/questionnaire"
	"relations/internal/scoring"
)

func analysisHTTPAccount(t *testing.T, accounts *auth.Service) auth.SessionTokens {
	t.Helper()
	result, err := accounts.Register(context.Background(), auth.RegisterInput{Email: uuid.NewString() + "@example.test", Name: "HTTP fixture", Password: "strong-password-123"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = accounts.UpdateProfile(context.Background(), result.User.ID, auth.ProfileInput{Name: result.User.Name, Age: 25, Gender: "prefer_not_to_say"}); err != nil {
		t.Fatal(err)
	}
	return result
}

func TestAnalysisHTTPStrictInputsOwnershipAndDeterministicJourney(t *testing.T) {
	db := httpAuthDB(t)
	if err := questionnaire.Seed(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	cfg := httpAuthConfig()
	router := New(cfg, db, slog.New(slog.NewJSONHandler(io.Discard, nil)))
	accounts := auth.New(cfg, db)
	owner, stranger := analysisHTTPAccount(t, accounts), analysisHTTPAccount(t, accounts)
	for _, body := range []string{
		`{"module":"self","user_id":"` + stranger.User.ID + `"}`,
		`{"module":"self","answers":{"self":{"o1":1,"o1":2}}}`,
		`{"module":"self","answers":{"self":{"o1":1.5}}}`,
		`{"module":"self","context":{"partner_consent":false}}`,
		`{"module":"self","context":null}`,
	} {
		response := authRequest(router, "POST", "/api/v1/analyses", body, cfg.PublicURL, owner.AccessToken, nil)
		if response.Code != 400 {
			t.Fatalf("accepted invalid analysis input: %d %s", response.Code, response.Body.String())
		}
	}
	response := authRequest(router, "POST", "/api/v1/analyses", `{"module":"self","locale":"en"}`, cfg.PublicURL, owner.AccessToken, nil)
	if response.Code != 201 {
		t.Fatal(response.Code, response.Body.String())
	}
	var envelope struct {
		Data database.Analysis `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	id := envelope.Data.ID
	for _, path := range []string{"/api/v1/analyses/" + id, "/api/v1/analyses/" + id + "/report"} {
		response = authRequest(router, "GET", path, "", cfg.PublicURL, stranger.AccessToken, nil)
		if response.Code != 404 || !strings.Contains(response.Body.String(), "analysis_not_found") {
			t.Fatal("other owner accessed analysis", response.Code, response.Body.String())
		}
	}
	response = authRequest(router, "GET", "/api/v1/analyses/"+id+"/report", "", cfg.PublicURL, owner.AccessToken, nil)
	if response.Code != 409 {
		t.Fatal("draft report reported ready", response.Code)
	}
	response = authRequest(router, "PATCH", "/api/v1/analyses/"+id, `{"revision":1,"answers":{}}`, cfg.PublicURL, owner.AccessToken, nil)
	if response.Code != 400 {
		t.Fatal("missing context silently replaced draft context", response.Code)
	}
	definition, err := questionnaire.Get("self", questionnaire.Version)
	if err != nil {
		t.Fatal(err)
	}
	answers := questionnaire.Answers{"self": {}}
	for _, question := range definition.Questions {
		value := 4
		answers["self"][question.ID] = &value
	}
	body, _ := json.Marshal(map[string]any{"revision": 1, "answers": answers, "context": map[string]any{}})
	response = authRequest(router, "PATCH", "/api/v1/analyses/"+id, string(body), cfg.PublicURL, owner.AccessToken, nil)
	if response.Code != 200 {
		t.Fatal(response.Code, response.Body.String())
	}
	response = authRequest(router, "PATCH", "/api/v1/analyses/"+id, string(body), cfg.PublicURL, owner.AccessToken, nil)
	if response.Code != 409 || !strings.Contains(response.Body.String(), "revision_conflict") {
		t.Fatal("stale draft overwrote saved answers", response.Code, response.Body.String())
	}
	response = authRequest(router, "POST", "/api/v1/analyses/"+id+"/submit", `{"revision":2,"consent":true,"generate_report":true}`, cfg.PublicURL, owner.AccessToken, nil)
	if response.Code != 400 || !strings.Contains(response.Body.String(), "ai_consent_required") {
		t.Fatal("AI sharing checkbox was not required", response.Code, response.Body.String())
	}
	response = authRequest(router, "POST", "/api/v1/analyses/"+id+"/submit", `{"revision":2,"consent":true,"generate_report":false}`, cfg.PublicURL, owner.AccessToken, nil)
	if response.Code != 200 {
		t.Fatal(response.Code, response.Body.String())
	}
	if err = json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.Data.Status != "completed" || envelope.Data.Revision != 3 || envelope.Data.AIConsentAt != nil || !strings.Contains(string(envelope.Data.Report), "deterministic") {
		t.Fatal(envelope.Data)
	}
	var scores scoring.Result
	if err := json.Unmarshal(envelope.Data.Scores, &scores); err != nil || scores.Status != "available" || scores.Respondents["self"].Answered != 36 {
		t.Fatal("deterministic scores were not persisted", scores, err)
	}
	response = authRequest(router, "GET", "/api/v1/analyses?module=self&status=completed&page_size=5", "", cfg.PublicURL, owner.AccessToken, nil)
	if response.Code != 200 || strings.Contains(response.Body.String(), "answers") || strings.Contains(response.Body.String(), "report\"") || !strings.Contains(response.Body.String(), `"total":1`) {
		t.Fatal("history exposed sensitive answer or report payload", response.Code, response.Body.String())
	}
	response = authRequest(router, "GET", "/api/v1/analyses?page_size=101", "", cfg.PublicURL, owner.AccessToken, nil)
	if response.Code != 400 {
		t.Fatal("unbounded pagination accepted", response.Code)
	}
	response = authRequest(router, "GET", "/api/v1/analyses/usage", "", cfg.PublicURL, owner.AccessToken, nil)
	if response.Code != 200 || !strings.Contains(response.Body.String(), `"user_used":0`) || !strings.Contains(response.Body.String(), `"global_used":0`) {
		t.Fatal(response.Code, response.Body.String())
	}
	response = authRequest(router, "DELETE", "/api/v1/analyses/"+id, "", cfg.PublicURL, stranger.AccessToken, nil)
	if response.Code != 404 {
		t.Fatal("other owner deleted completed report", response.Code)
	}
	response = authRequest(router, "DELETE", "/api/v1/analyses/"+id, "", cfg.PublicURL, owner.AccessToken, nil)
	if response.Code != 204 || response.Body.Len() != 0 {
		t.Fatal(response.Code, response.Body.String())
	}
}

func TestAnalysisHTTPRequiresProfileForWritesAndAuthenticationForReads(t *testing.T) {
	db := httpAuthDB(t)
	if err := questionnaire.Seed(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	cfg := httpAuthConfig()
	router := New(cfg, db, slog.New(slog.NewJSONHandler(io.Discard, nil)))
	accounts := auth.New(cfg, db)
	user, err := accounts.Register(context.Background(), auth.RegisterInput{Email: uuid.NewString() + "@example.test", Name: "Incomplete profile", Password: "strong-password-123"})
	if err != nil {
		t.Fatal(err)
	}
	response := authRequest(router, "POST", "/api/v1/analyses", `{"module":"self"}`, cfg.PublicURL, user.AccessToken, nil)
	if response.Code != 403 || !strings.Contains(response.Body.String(), "profile_required") {
		t.Fatal(response.Code, response.Body.String())
	}
	response = authRequest(router, "GET", "/api/v1/analyses", "", cfg.PublicURL, user.AccessToken, nil)
	if response.Code != 200 {
		t.Fatal("unfinished profile could not inspect own history", response.Code)
	}
	response = authRequest(router, "GET", "/api/v1/analyses", "", cfg.PublicURL, "", nil)
	if response.Code != 401 {
		t.Fatal("history was publicly available", response.Code)
	}
}
