package scoring

import (
	"encoding/json"
	"math"
	"reflect"
	"strings"
	"testing"

	"relations/internal/questionnaire"
)

func value(number int) *int { return &number }

func TestExactTwoThirds(t *testing.T) {
	for total, minimum := range map[int]int{2: 2, 3: 2, 4: 3, 5: 4, 6: 4} {
		if Required(total) != minimum {
			t.Fatalf("required(%d)=%d, want %d", total, Required(total), minimum)
		}
	}
}

func TestReverseUnknownAndMissing(t *testing.T) {
	q, _ := questionnaire.Current("self")
	answers := questionnaire.Answers{"self": {"o1": value(1), "o2": value(2), "o3": value(4), "o4": value(5), "ax1": value(5), "ax2": value(5), "ax3": nil,
		"c1": value(5), "c2": nil, "c3": value(5)}}
	result, err := Compute(q, answers, questionnaire.Context{})
	if err != nil {
		t.Fatal(err)
	}
	dims := result.Respondents["self"].Dimensions
	if dims[0].Score == nil || *dims[0].Score != 37.5 || *dims[0].Band != "lower" || dims[0].Completeness != 1 {
		t.Fatal("reverse-scored example differs", dims[0])
	}
	if dims[1].Score != nil || dims[1].Band != nil || dims[1].Status != "insufficient_data" || dims[1].Unknown != 1 || dims[1].Omitted != 1 || dims[1].Answered != 2 {
		t.Fatal("missing/unknown silently imputed", dims[1])
	}
	if dims[5].Score == nil || *dims[5].Score != 100 || dims[5].Completeness != 2.0/3.0 || dims[5].Unknown != 1 {
		t.Fatal("exact two-thirds three-item score differs", dims[5])
	}
	if result.Status != "insufficient_data" {
		t.Fatal("fewer than three meaningful dimensions should not authorize analysis")
	}
	encoded, _ := json.Marshal(result)
	if !strings.Contains(string(encoded), `"score":null`) || !strings.Contains(string(encoded), `"band":null`) {
		t.Fatal("missing charts must receive explicit null")
	}
	second, _ := Compute(q, answers, questionnaire.Context{})
	if !reflect.DeepEqual(result, second) {
		t.Fatal("same answers/version produced different scores")
	}
}

func TestNormalizationBoundsAndCompleteness(t *testing.T) {
	q, _ := questionnaire.Current("self")
	for _, endpoint := range []int{1, 5} {
		answers := questionnaire.Answers{"self": {}}
		for _, question := range q.Questions {
			answer := endpoint
			if question.Reverse {
				answer = 6 - answer
			}
			answers["self"][question.ID] = value(answer)
		}
		result, err := Compute(q, answers, questionnaire.Context{})
		if err != nil || result.Status != "available" || result.Respondents["self"].NumericCompleteness != 1 {
			t.Fatal(result, err)
		}
		for _, dimension := range result.Respondents["self"].Dimensions {
			if dimension.Score == nil || *dimension.Score != float64(25*(endpoint-1)) {
				t.Fatal("normalization endpoint", dimension)
			}
		}
	}
	answers := questionnaire.Answers{"self": {}}
	for _, question := range q.Questions {
		answers["self"][question.ID] = nil
	}
	result, err := Compute(q, answers, questionnaire.Context{})
	if err != nil || result.Status != "insufficient_data" || result.Respondents["self"].Unknown != 36 || result.Respondents["self"].Omitted != 0 || result.Respondents["self"].NumericCompleteness != 0 {
		t.Fatal("explicit unknowns count only as selections", result, err)
	}
}

func TestSimilarityArithmeticAndConsent(t *testing.T) {
	q, _ := questionnaire.Current("compatibility")
	answers := questionnaire.Answers{"self": {"o1": value(2), "o2": value(4), "c1": value(3), "c2": value(3), "e1": value(1), "e2": value(5)},
		"partner": {"o1": value(4), "o2": value(2), "c1": value(3), "c2": value(3), "e1": value(2), "e2": value(4)}}
	ctx := questionnaire.Context{PartnerResponseOrigin: "independent", PartnerConsent: true}
	result, err := Compute(q, answers, ctx)
	if err != nil || result.Status != "available" || result.Compatibility.Index == nil || *result.Compatibility.Index != 75 || result.Compatibility.Coverage != 3.0/15.0 || len(result.Compatibility.OmittedDimensions) != 12 {
		t.Fatal("expected component similarities 50,100,75 and mean75", result, err)
	}
	for _, ctx := range []questionnaire.Context{{PartnerResponseOrigin: "proxy", PartnerConsent: true}, {PartnerResponseOrigin: "independent"}, {}} {
		result, err := Compute(q, answers, ctx)
		if err != nil || result.Status != "insufficient_data" || result.Compatibility.Index != nil || result.Compatibility.Status != "requires_independent_responses" || len(result.Compatibility.Components) != 0 {
			t.Fatal("proxy/unconsented shared information was scored", result, err)
		}
		if result.Respondents["self"].Dimensions[0].Score == nil || result.Respondents["partner"].Dimensions[0].Score == nil {
			t.Fatal("individual deterministic scores should be preserved")
		}
	}
	delete(answers["partner"], "e2")
	result, err = Compute(q, answers, questionnaire.Context{PartnerResponseOrigin: "independent", PartnerConsent: true})
	if err != nil || result.Compatibility.Index != nil || result.Status != "insufficient_data" || len(result.Compatibility.Components) != 2 {
		t.Fatal("fewer than three common sufficient dimensions", result, err)
	}
}

func TestConflictNarrativeWithNullStyles(t *testing.T) {
	q, _ := questionnaire.Current("conflict")
	answers := questionnaire.Answers{"self": {}}
	for _, question := range q.Questions {
		answers["self"][question.ID] = nil
	}
	ctx := questionnaire.Context{RelationshipType: "friend", Safety: "unknown", Topic: "Missed plans"}
	result, err := Compute(q, answers, ctx)
	if err != nil || result.Status != "available" {
		t.Fatal("valid reported context permits cautious narrative", result, err)
	}
	for _, dimension := range result.Respondents["self"].Dimensions {
		if dimension.Score != nil || math.IsNaN(dimension.Completeness) {
			t.Fatal("invented style score", dimension)
		}
	}
	result, err = Compute(q, answers, questionnaire.Context{})
	if err != nil || result.Status != "insufficient_data" {
		t.Fatal("missing required context must not authorize narrative", result, err)
	}
}
