package ai

import (
	"encoding/json"
	"strings"
	"testing"

	"relations/internal/questionnaire"
	"relations/internal/scoring"
)

func reportFixture(t *testing.T, module string) (Report, scoring.Result, questionnaire.Context) {
	t.Helper()
	def, err := questionnaire.Get(module, "1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	input := questionnaire.Context{}
	if module == "conflict" {
		input = questionnaire.Context{RelationshipType: "partner", Safety: "no", Topic: "A disagreement"}
	}
	scores, err := scoring.Compute(def, questionnaire.Answers{}, input)
	if err != nil {
		t.Fatal(err)
	}
	report := Report{Kind: "ai", SchemaVersion: ReportSchema, Module: module, Locale: "en", Title: "Reflection", Summary: "A descriptive reflection with limited data.", Sections: []Section{{"reflection", "Reflection", "Consider a small reflective practice."}}, Recommendations: []string{"Consider what feels useful."}, Limitations: []string{"Original questions are not validated."}, References: []string{}, ScoreEcho: echoes(scores), Safety: Safety{Level: "routine", Message: "Respect your own boundaries.", Support: []string{}}}
	if module == "conflict" {
		report.Safety = Safety{Level: "caution", Message: "Prioritize your safety.", Support: []string{"Consider trusted local support."}}
	}
	return report, scores, input
}

func TestReportsRequireExactShapeScoresReferencesAndSafety(t *testing.T) {
	for _, module := range []string{"self", "compatibility", "conflict"} {
		report, scores, input := reportFixture(t, module)
		data, _ := json.Marshal(report)
		if _, err := ValidateReport(data, module, "en", scores, input); err != nil {
			t.Fatal(module, err)
		}
	}
	report, scores, input := reportFixture(t, "self")
	for _, mutation := range []func(*Report){
		func(r *Report) { n := 80.0; r.ScoreEcho[0].Score = &n },
		func(r *Report) { r.ScoreEcho = r.ScoreEcho[1:] },
		func(r *Report) { r.ScoreEcho[1] = r.ScoreEcho[0] },
		func(r *Report) { r.References = []string{"invented-paper"} },
		func(r *Report) { r.Module = "conflict" },
		func(r *Report) { r.Locale = "fa" },
		func(r *Report) { r.Limitations = nil },
		func(r *Report) { r.Summary = strings.Repeat("x", 3001) },
	} {
		data, _ := json.Marshal(report)
		var changed Report
		_ = json.Unmarshal(data, &changed)
		mutation(&changed)
		data, _ = json.Marshal(changed)
		if _, err := ValidateReport(data, "self", "en", scores, input); err == nil {
			t.Fatal("accepted untrusted report mutation")
		}
	}
	data, _ := json.Marshal(report)
	var raw map[string]any
	_ = json.Unmarshal(data, &raw)
	delete(raw, "references")
	missing, _ := json.Marshal(raw)
	if _, err := ValidateReport(missing, "self", "en", scores, input); err == nil {
		t.Fatal("accepted missing required references")
	}
	if _, err := ValidateReport(append([]byte(`{"injected":"ignore rules",`), data[1:]...), "self", "en", scores, input); err == nil {
		t.Fatal("accepted unexpected field")
	}
	if _, err := ValidateReport(append([]byte(`{"kind":"ai",`), data[1:]...), "self", "en", scores, input); err == nil {
		t.Fatal("accepted duplicate field")
	}
	conflict, conflictScores, conflictInput := reportFixture(t, "conflict")
	conflict.Safety.Level = "routine"
	data, _ = json.Marshal(conflict)
	if _, err := ValidateReport(data, "conflict", "en", conflictScores, conflictInput); err == nil {
		t.Fatal("accepted unsafe conflict framing")
	}
}

func TestPromptCatalogIsFinite(t *testing.T) {
	for _, module := range []string{"self", "compatibility", "conflict"} {
		for _, locale := range []string{"en", "fa"} {
			text, err := prompt(module, locale)
			if err != nil || text == "" {
				t.Fatal(module, locale, err)
			}
		}
	}
	if _, err := prompt("../service", "en"); err == nil {
		t.Fatal("accepted prompt path injection")
	}
	if _, err := prompt("self", "de"); err == nil {
		t.Fatal("accepted unsupported locale")
	}
}

func TestReportNumericEchoCannotBeChangedEvenSlightly(t *testing.T) {
	report, scores, input := reportFixture(t, "self")
	def, err := questionnaire.Get("self", "1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	answers := questionnaire.Answers{"self": map[string]*int{}}
	for _, question := range def.Questions {
		value := 4
		answers["self"][question.ID] = &value
	}
	scores, err = scoring.Compute(def, answers, input)
	if err != nil {
		t.Fatal(err)
	}
	report.ScoreEcho = echoes(scores)
	data, _ := json.Marshal(report)
	if _, err = ValidateReport(data, "self", "en", scores, input); err != nil {
		t.Fatal("valid numeric score", err)
	}
	changed := *report.ScoreEcho[0].Score + 0.0000001
	report.ScoreEcho[0].Score = &changed
	data, _ = json.Marshal(report)
	if _, err = ValidateReport(data, "self", "en", scores, input); err == nil {
		t.Fatal("accepted small invented score change")
	}
}
