package chip

import (
	"encoding/json"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/collibra/chip/pkg/clients"
	"github.com/google/jsonschema-go/jsonschema"
)

// answerProbe mirrors what an assessment tool returns: an assessment whose
// content carries Answer, the tagged union the reflector cannot derive.
type answerProbe struct {
	Assessment *clients.Assessment `json:"assessment,omitempty"`
}

func answerNode(t *testing.T, s *jsonschema.Schema) *jsonschema.Schema {
	t.Helper()
	assessment, ok := s.Properties["assessment"]
	if !ok {
		t.Fatal("assessment property missing from generated schema")
	}
	content, ok := assessment.Properties["content"]
	if !ok {
		t.Fatal("content property missing from generated schema")
	}
	if content.Items == nil {
		t.Fatal("content has no item schema")
	}
	answer, ok := content.Items.Properties["answer"]
	if !ok {
		t.Fatal("answer property missing from generated schema")
	}
	return answer
}

func TestBuildSchemaReplacesUntypedAnswer(t *testing.T) {
	answer := answerNode(t, buildSchema[answerProbe](clients.AssessmentTypeSchemas()))

	if answer.Type != "object" {
		t.Errorf("answer: got Type=%q, want %q", answer.Type, "object")
	}
	if len(answer.Types) != 0 {
		t.Errorf("answer still carries a type union %v; the nullable strip should collapse it", answer.Types)
	}
	if len(answer.OneOf) != 10 {
		t.Errorf("answer: got %d variants, want one per answer type (10)", len(answer.OneOf))
	}

	// Without the override the reflector renders Value ("any") as the boolean
	// schema true, which strict MCP clients reject.
	raw, err := json.Marshal(buildSchema[answerProbe](clients.AssessmentTypeSchemas()))
	if err != nil {
		t.Fatalf("marshaling schema: %v", err)
	}
	if strings.Contains(string(raw), `"value":true`) {
		t.Errorf("schema still carries a boolean subschema for an answer value: %s", raw)
	}
}

// TestTypelessOverrideBecomesNullOnly pins the trap that answerSchema works
// around. The reflector prepends "null" to an override that carries no type,
// and the nullable strip keeps a lone "null", so the field accepts null only.
// If this stops being true, the Type in answerSchema can go.
func TestTypelessOverrideBecomesNullOnly(t *testing.T) {
	untyped := map[reflect.Type]*jsonschema.Schema{
		reflect.TypeFor[clients.Answer](): {OneOf: []*jsonschema.Schema{{Type: "object"}}},
	}
	answer := answerNode(t, buildSchema[answerProbe](untyped))
	if answer.Type != "null" && !slices.Equal(answer.Types, []string{"null"}) {
		t.Errorf("type-less override kept Type=%q Types=%v; expected null only", answer.Type, answer.Types)
	}
}

func TestAnswerSchemaAcceptsEveryAnswerType(t *testing.T) {
	resolved, err := buildSchema[answerProbe](clients.AssessmentTypeSchemas()).Resolve(nil)
	if err != nil {
		t.Fatalf("resolving schema: %v", err)
	}

	cases := []struct {
		name  string
		wire  string
		valid bool
	}{
		{"text", `{"type":"TEXT","value":"too many support tickets"}`, true},
		{"html", `{"type":"HTML","value":"<b>bold</b>"}`, true},
		{"expression", `{"type":"EXPRESSION","value":"Low Risk"}`, true},
		{"date", `{"type":"DATE","value":"2017-07-21"}`, true},
		{"number", `{"type":"NUMBER","value":1.3}`, true},
		{"boolean", `{"type":"BOOLEAN","value":true}`, true},
		// Value has "omitempty", so false marshals without the property.
		{"boolean false", `{"type":"BOOLEAN","value":false}`, true},
		{"empty text", `{"type":"TEXT","value":""}`, true},
		{"items", `{"type":"ITEMS","value":[{"id":"choice1","value":"My Choice"}]}`, true},
		{"assets", `{"type":"ASSETS","value":[{"id":"9e6ba6fa-ae24-41c8-9b42-08e7c4231689"}]}`, true},
		{"users or groups", `{"type":"USERORGROUPS","value":[{"id":"387b34f5-eac8-467c-89b6-5ee7c2e7368b","type":"GROUP"}]}`, true},
		{"attachments", `{"type":"ATTACHMENTS","value":[{"id":"85a9c637-e27e-4a7f-80f1-6db07e336d04","fileName":"evidence.pdf"}]}`, true},
		{"unknown property in an entry", `{"type":"ITEMS","value":[{"id":"choice1","label":"new field"}]}`, true},
		{"number as string", `{"type":"NUMBER","value":"not a number"}`, false},
		{"boolean as string", `{"type":"BOOLEAN","value":"true"}`, false},
		{"items as scalar", `{"type":"ITEMS","value":"choice1"}`, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := validateProbe(t, resolved, `{"id":"q1","answer":`+tc.wire+`}`)
			if tc.valid && got != nil {
				t.Errorf("answer %s: got error %v, want valid", tc.wire, got)
			}
			if !tc.valid && got == nil {
				t.Errorf("answer %s: got valid, want a validation error", tc.wire)
			}
		})
	}
}

func TestAnswerSchemaAcceptsUnansweredQuestion(t *testing.T) {
	resolved, err := buildSchema[answerProbe](clients.AssessmentTypeSchemas()).Resolve(nil)
	if err != nil {
		t.Fatalf("resolving schema: %v", err)
	}
	if got := validateProbe(t, resolved, `{"id":"q1","name":"Business problem"}`); got != nil {
		t.Errorf("question without an answer: got error %v, want valid", got)
	}
}

// validateProbe runs one question and answer through the path the SDK uses:
// decode the API response, normalize, marshal, then validate the JSON.
func validateProbe(t *testing.T, resolved *jsonschema.Resolved, questionAndAnswer string) error {
	t.Helper()
	var probe answerProbe
	wire := `{"assessment":{"id":"07ed39f5-be3b-4406-a09e-3c78a8734261","content":[` + questionAndAnswer + `]}}`
	if err := json.Unmarshal([]byte(wire), &probe); err != nil {
		t.Fatalf("decoding %s: %v", wire, err)
	}
	normalizeNilCollections(reflect.ValueOf(&probe).Elem())
	out, err := json.Marshal(probe)
	if err != nil {
		t.Fatalf("marshaling probe: %v", err)
	}
	var generic any
	if err := json.Unmarshal(out, &generic); err != nil {
		t.Fatalf("re-decoding output: %v", err)
	}
	return resolved.Validate(generic)
}
