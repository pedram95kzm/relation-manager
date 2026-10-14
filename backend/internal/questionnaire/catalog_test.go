package questionnaire

import (
	"encoding/json"
	"strings"
	"testing"

	"relations/internal/platform"
)

func TestCatalogBilingualCountsAndPublicKeys(t *testing.T) {
	for module, count := range map[string]int{"self": 36, "compatibility": 32, "conflict": 15} {
		t.Run(module, func(t *testing.T) {
			q, err := Current(module)
			if err != nil || len(q.Questions) != count {
				t.Fatal(q.Module, len(q.Questions), err)
			}
			for _, question := range q.Questions {
				if question.Text.FA == "" || question.Text.EN == "" || question.Rationale == "" {
					t.Fatal("missing bilingual wording or rationale")
				}
			}
			data, err := json.Marshal(Public(q))
			if err != nil {
				t.Fatal(err)
			}
			for _, hidden := range []string{"\"reverse\"", "\"rationale\"", "\"references\"", "\"weights\""} {
				if strings.Contains(string(data), hidden) {
					t.Fatalf("public definition leaks %s", hidden)
				}
			}
			var public PublicDefinition
			if err := platform.StrictJSON(data, &public); err != nil || len(public.Questions[0].Options) != 5 || !public.Questions[0].AllowUnknown {
				t.Fatal("invalid public answer contract", err)
			}
		})
	}
}

func TestCatalogImmutableCopiesAndAllowlist(t *testing.T) {
	first, _ := Current("self")
	first.Questions[0].Reverse = !first.Questions[0].Reverse
	first.Questions[0].Text.FA = "changed"
	first.Dimensions[0].References[0] = "unverified"
	if err := validateDefinition(first); err == nil {
		t.Fatal("unverified evidence accepted")
	}
	second, err := Current("self")
	if err != nil || second.Questions[0].Text.FA == "changed" || second.Dimensions[0].References[0] != "bfi2" {
		t.Fatal("caller altered embedded catalog", err)
	}
	for _, input := range [][2]string{{"self", "../../anything"}, {"unknown", Version}, {"self", "2.0.0"}} {
		if _, err := Get(input[0], input[1]); err == nil {
			t.Fatal("unsupported catalog accepted", input)
		}
	}
	if !ValidLocale("fa") || !ValidLocale("en") || ValidLocale("FA") || ValidLocale("de") {
		t.Fatal("locale allowlist changed")
	}
}

func TestCanonicalDigestUsesJSONMeaning(t *testing.T) {
	first, err := canonicalDigest([]byte(`{"a":1,"b":{"x":true,"y":null}}`))
	if err != nil {
		t.Fatal(err)
	}
	second, err := canonicalDigest([]byte(`{ "b": { "y": null, "x": true }, "a": 1.0 }`))
	if err != nil || first != second {
		t.Fatal("JSONB normalization causes false drift", err)
	}
	third, _ := canonicalDigest([]byte(`{"a":2,"b":{"x":true,"y":null}}`))
	if first == third {
		t.Fatal("changed definition has identical digest")
	}
}
