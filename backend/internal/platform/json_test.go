package platform

import (
	"strings"
	"testing"
)

func TestDuplicateQuestionAndDepthRejected(t *testing.T) {
	var result map[string]any
	for _, input := range []string{`{"self":{"o1":2,"o1":5}}`, strings.Repeat("[", 70) + "null" + strings.Repeat("]", 70), `{} {}`} {
		if StrictJSON([]byte(input), &result) == nil {
			t.Fatal("accepted ambiguous JSON")
		}
	}
	if e := StrictJSON([]byte(`{"self":{"o1":2,"o2":null}}`), &result); e != nil {
		t.Fatal(e)
	}
}
