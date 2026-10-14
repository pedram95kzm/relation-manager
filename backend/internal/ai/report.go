package ai

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"
	"unicode/utf8"

	"relations/internal/platform"
	"relations/internal/questionnaire"
	"relations/internal/scoring"
)

func object(properties map[string]any) map[string]any {
	keys := make([]string, 0, len(properties))
	for k := range properties {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return map[string]any{"type": "object", "properties": properties, "required": keys, "additionalProperties": false}
}
func textSchema() map[string]any         { return map[string]any{"type": "string"} }
func listSchema(item any) map[string]any { return map[string]any{"type": "array", "items": item} }

func schema() map[string]any {
	return schemaForKind("ai")
}

func schemaForKind(kind string) map[string]any {
	return object(map[string]any{
		"kind":           map[string]any{"type": "string", "enum": []string{kind}},
		"schema_version": map[string]any{"type": "string", "enum": []string{ReportSchema}},
		"module":         map[string]any{"type": "string", "enum": []string{"self", "compatibility", "conflict"}},
		"locale":         map[string]any{"type": "string", "enum": []string{"en", "fa"}},
		"title":          textSchema(), "summary": textSchema(),
		"sections":        listSchema(object(map[string]any{"id": textSchema(), "title": textSchema(), "body": textSchema()})),
		"recommendations": listSchema(textSchema()), "limitations": listSchema(textSchema()), "references": listSchema(textSchema()),
		"score_echo": listSchema(object(map[string]any{"respondent": textSchema(), "dimension_id": textSchema(), "score": map[string]any{"type": []string{"number", "null"}}})),
		"safety":     object(map[string]any{"level": map[string]any{"type": "string", "enum": []string{"routine", "caution"}}, "message": textSchema(), "support": listSchema(textSchema())}),
	})
}

// Local schema validation is always applied, including providers with JSON-only mode.
func validateShape(value any, rule map[string]any) error {
	if types, ok := rule["type"].([]string); ok {
		for _, typ := range types {
			if validateShape(value, map[string]any{"type": typ}) == nil {
				return nil
			}
		}
		return fmt.Errorf("invalid nullable type")
	}
	switch rule["type"] {
	case "object":
		obj, ok := value.(map[string]any)
		if !ok {
			return fmt.Errorf("expected object")
		}
		props := rule["properties"].(map[string]any)
		if len(obj) != len(props) {
			return fmt.Errorf("object fields mismatch")
		}
		for key, sub := range props {
			v, exists := obj[key]
			if !exists {
				return fmt.Errorf("missing field")
			}
			if err := validateShape(v, sub.(map[string]any)); err != nil {
				return err
			}
		}
	case "array":
		arr, ok := value.([]any)
		if !ok {
			return fmt.Errorf("expected array")
		}
		for _, v := range arr {
			if err := validateShape(v, rule["items"].(map[string]any)); err != nil {
				return err
			}
		}
	case "string":
		if _, ok := value.(string); !ok {
			return fmt.Errorf("expected string")
		}
	case "number":
		if _, ok := value.(float64); !ok {
			return fmt.Errorf("expected number")
		}
	case "null":
		if value != nil {
			return fmt.Errorf("expected null")
		}
	}
	if enums, ok := rule["enum"].([]string); ok {
		for _, v := range enums {
			if value == v {
				return nil
			}
		}
		return fmt.Errorf("invalid enum")
	}
	return nil
}

func validText(s string, max int) bool {
	return utf8.ValidString(s) && strings.TrimSpace(s) != "" && utf8.RuneCountInString(s) <= max && !strings.ContainsRune(s, '\x00')
}

func ValidateReport(data []byte, module, locale string, scores scoring.Result, input questionnaire.Context) (Report, error) {
	return validateReportKind(data, module, locale, scores, input, "ai")
}

// Only the trusted local generator may opt into example reports. External
// provider output always goes through ValidateReport and its fixed ai kind.
func validateReportKind(data []byte, module, locale string, scores scoring.Result, input questionnaire.Context, kind string) (Report, error) {
	invalid := func() (Report, error) {
		return Report{}, platform.Err(502, "ai_invalid_report", "The provider returned an invalid report. Your deterministic scores remain available.")
	}
	if len(data) > 65536 {
		return invalid()
	}
	var report Report
	if platform.StrictJSON(data, &report) != nil {
		return invalid()
	}
	var raw any
	if json.Unmarshal(data, &raw) != nil || validateShape(raw, schemaForKind(kind)) != nil {
		return invalid()
	}
	if report.Module != module || report.Locale != locale || scores.Module != module || !validText(report.Title, 160) || !validText(report.Summary, 3000) {
		return invalid()
	}
	if len(report.Sections) < 1 || len(report.Sections) > 8 || len(report.Recommendations) < 1 || len(report.Recommendations) > 10 || len(report.Limitations) < 1 || len(report.Limitations) > 8 || len(report.References) > 7 {
		return invalid()
	}
	ids := map[string]bool{}
	for _, section := range report.Sections {
		if !validText(section.ID, 64) || ids[section.ID] || !validText(section.Title, 160) || !validText(section.Body, 5000) {
			return invalid()
		}
		ids[section.ID] = true
	}
	for _, values := range [][]string{report.Recommendations, report.Limitations, report.Safety.Support} {
		for _, v := range values {
			if !validText(v, 1500) {
				return invalid()
			}
		}
	}
	def, err := questionnaire.Get(module, scores.QuestionnaireVersion)
	if err != nil {
		return invalid()
	}
	allowed := map[string]bool{}
	for _, d := range def.Dimensions {
		for _, ref := range d.References {
			allowed[ref] = true
		}
	}
	seen := map[string]bool{}
	for _, ref := range report.References {
		if !allowed[ref] || seen[ref] {
			return invalid()
		}
		seen[ref] = true
	}
	expected := echoes(scores)
	if len(report.ScoreEcho) != len(expected) {
		return invalid()
	}
	byID := map[string]*float64{}
	for _, e := range expected {
		byID[e.Respondent+":"+e.DimensionID] = e.Score
	}
	seen = map[string]bool{}
	for _, e := range report.ScoreEcho {
		key := e.Respondent + ":" + e.DimensionID
		want, ok := byID[key]
		if !ok || seen[key] || (want == nil) != (e.Score == nil) {
			return invalid()
		}
		if want != nil && (math.IsNaN(*e.Score) || math.IsInf(*e.Score, 0) || *e.Score != *want) {
			return invalid()
		}
		seen[key] = true
	}
	if !validText(report.Safety.Message, 2000) || len(report.Safety.Support) > 5 {
		return invalid()
	}
	if module == "conflict" && input.Safety != "yes" && (report.Safety.Level != "caution" || len(report.Safety.Support) == 0) {
		return invalid()
	}
	return report, nil
}
