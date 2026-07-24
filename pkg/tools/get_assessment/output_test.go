package get_assessment

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/collibra/chip/pkg/clients"
	"github.com/google/jsonschema-go/jsonschema"
)

// Guards the template.version wire-type bug: the API sends version as a JSON
// number, but the generated output schema types it as a string, so a populated
// assessment failed output validation ("has type integer, want string").
// clients.VersionString fixes it by decoding either form and always marshaling
// as a string. This builds a populated Output, validates it against the tool's
// own schema, and asserts version serializes as a quoted string.
func TestOutputWithTemplateVersionValidates(t *testing.T) {
	schema, err := jsonschema.For[Output](nil)
	if err != nil {
		t.Fatalf("build schema: %v", err)
	}
	resolved, err := schema.Resolve(nil)
	if err != nil {
		t.Fatalf("resolve schema: %v", err)
	}

	out := Output{
		Found: true,
		Assessment: &clients.Assessment{
			ID:     "07ed39f5-be3b-4406-a09e-3c78a8734261",
			Name:   "Business Context for Churn Prediction Model",
			Status: "DRAFT",
			Template: &clients.AssessmentTemplate{
				ID:      "2e428d4d-595e-4a2d-9a2b-1e31f05e17c6",
				Name:    "Business Context",
				Version: "8",
				Status:  "PUBLISHED",
			},
			Content: []clients.QuestionAndAnswer{
				{ID: "business_case", Name: "Business problem", Answer: &clients.Answer{Type: "HTML", Value: "reduce churn"}},
			},
		},
	}

	raw, err := json.Marshal(out)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(raw), `"version":"8"`) {
		t.Fatalf("template version should marshal as a quoted string; got %s", raw)
	}

	var instance any
	if err := json.Unmarshal(raw, &instance); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if err := resolved.Validate(instance); err != nil {
		t.Fatalf("populated get_assessment output failed its own schema: %v\noutput: %s", err, raw)
	}
}

// Also lock the decode side: a numeric version from the API must round-trip to
// a quoted string.
func TestVersionStringDecodesNumberAndString(t *testing.T) {
	for _, in := range []string{`{"version":8}`, `{"version":"8"}`} {
		var tmpl clients.AssessmentTemplate
		if err := json.Unmarshal([]byte(in), &tmpl); err != nil {
			t.Fatalf("unmarshal %s: %v", in, err)
		}
		if tmpl.Version != "8" {
			t.Fatalf("input %s: got version %q, want \"8\"", in, tmpl.Version)
		}
	}
}
