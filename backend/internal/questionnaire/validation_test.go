package questionnaire

import (
	"strings"
	"testing"

	"relations/internal/platform"
)

func number(value int) *int { return &value }

func complete(q Definition, value *int) Answers {
	answers := Answers{"self": {}}
	for _, question := range q.Questions {
		answers["self"][question.ID] = value
	}
	return answers
}

func TestRequiredUnknownAndMissingDraft(t *testing.T) {
	q, _ := Current("self")
	answers := complete(q, nil)
	if err := Validate(q, answers, Context{}, true); err != nil {
		t.Fatal("explicit unknown should be valid required selection", err)
	}
	delete(answers["self"], q.Questions[0].ID)
	if err := Validate(q, answers, Context{}, false); err != nil {
		t.Fatal("partial draft rejected", err)
	}
	if err := Validate(q, answers, Context{}, true); err == nil {
		t.Fatal("final omitted required item accepted")
	}
}

func TestAnswerValidationAndAmbiguousJSON(t *testing.T) {
	q, _ := Current("self")
	for name, answers := range map[string]Answers{
		"out of range": {"self": {"o1": number(6)}},
		"zero":         {"self": {"o1": number(0)}},
		"unknown key":  {"self": {"unsupported": number(3)}},
		"partner":      {"partner": {"o1": number(3)}},
		"respondent":   {"someone": {}},
		"null map":     {"self": nil},
	} {
		t.Run(name, func(t *testing.T) {
			if err := Validate(q, answers, Context{}, false); err == nil {
				t.Fatal("malformed answer accepted")
			}
		})
	}
	for _, data := range []string{`{"self":{"o1":1.5}}`, `{"self":{"o1":1,"o1":5}}`, `{"self":{"o1":"unknown"}}`, `{"self":{"o1":3},"self":{"o1":4}}`} {
		var answers Answers
		if err := platform.StrictJSON([]byte(data), &answers); err == nil {
			t.Fatal("fraction/string/duplicate JSON accepted", data)
		}
	}
	var valid Answers
	if err := platform.StrictJSON([]byte(`{"self":{"o1":null,"o2":3}}`), &valid); err != nil || valid["self"]["o1"] != nil || *valid["self"]["o2"] != 3 {
		t.Fatal("nullable integer answer contract", err)
	}
	q.Version = "unsupported"
	if err := Validate(q, valid, Context{}, false); err == nil {
		t.Fatal("version mismatch accepted")
	}
}

func TestContextBoundsRequirementsAndModules(t *testing.T) {
	valid := Context{RelationshipType: "friend", Safety: "unknown", Topic: "A disagreement about plans"}
	if err := ValidateContext("conflict", valid, true); err != nil {
		t.Fatal("unknown safety should retain context", err)
	}
	for _, ctx := range []Context{{}, {RelationshipType: "stranger", Safety: "no", Topic: "Issue"},
		{RelationshipType: "friend", Safety: "maybe", Topic: "Issue"}, {RelationshipType: "friend", Safety: "no", Topic: "  "},
		{RelationshipType: "friend", Safety: "no", Topic: strings.Repeat("الف", 1000)},
		{RelationshipType: "friend", Safety: "no", Topic: "bad\x00text"},
		{RelationshipType: "friend", Safety: "no", Topic: "Issue", PartnerConsent: true}} {
		if err := ValidateContext("conflict", ctx, true); err == nil {
			t.Fatal("invalid context accepted")
		}
	}
	if err := ValidateContext("conflict", Context{}, false); err != nil {
		t.Fatal("partial context rejected", err)
	}
	if err := ValidateContext("self", valid, false); err == nil {
		t.Fatal("incompatible conflict fields accepted")
	}
	if err := ValidateContext("compatibility", Context{}, true); err == nil {
		t.Fatal("missing explicit origin accepted")
	}
	for _, origin := range []string{"independent", "proxy"} {
		if err := ValidateContext("compatibility", Context{PartnerResponseOrigin: origin}, true); err != nil {
			t.Fatal("valid origin assertion rejected", err)
		}
	}
}

func TestTypedContextRetainsPresence(t *testing.T) {
	for _, data := range []string{`null`, `{"topic":null}`, `{"unknown":"x"}`, `{"topic":"x","topic":"y"}`, `{"partner_consent":"yes"}`} {
		var ctx Context
		if err := platform.StrictJSON([]byte(data), &ctx); err == nil {
			t.Fatal("invalid context JSON accepted", data)
		}
	}
	for _, data := range []string{`{"partner_consent":false}`, `{"topic":""}`} {
		var ctx Context
		if err := platform.StrictJSON([]byte(data), &ctx); err != nil {
			t.Fatal(err)
		}
		if err := ValidateContext("self", ctx, false); err == nil {
			t.Fatal("module-incompatible present empty field accepted")
		}
	}
}
