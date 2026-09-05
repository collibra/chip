package clients

import (
	"encoding/json"
	"testing"
)

// The API sends an answer value in a shape that depends on the answer type.
// Each shape must decode onto a concrete field: a scalar onto value, a choice
// onto items. The fields must stay typed, or the generated tool schema carries
// a property with no type and strict MCP clients reject the whole tool import.
func TestAnswerUnmarshalNarrowsValue(t *testing.T) {
	tests := []struct {
		name      string
		raw       string
		wantValue string
		wantItems []AnswerItem
	}{
		{name: "text", raw: `{"type":"HTML","value":"reduce churn"}`, wantValue: "reduce churn"},
		{name: "number keeps integer form", raw: `{"type":"NUMBER","value":42}`, wantValue: "42"},
		{name: "number keeps decimals", raw: `{"type":"NUMBER","value":3.5}`, wantValue: "3.5"},
		{name: "boolean", raw: `{"type":"BOOLEAN","value":true}`, wantValue: "true"},
		{name: "absent value", raw: `{"type":"TEXT"}`, wantValue: ""},
		{
			name:      "items become a list of options",
			raw:       `{"type":"ITEMS","value":[{"id":"yes","value":"Yes"}]}`,
			wantItems: []AnswerItem{{ID: "yes", Value: "Yes"}},
		},
		{
			name:      "unrecognised list falls back to json",
			raw:       `{"type":"ASSETS","value":["a","b"]}`,
			wantValue: `["a","b"]`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var got Answer
			if err := json.Unmarshal([]byte(tc.raw), &got); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if got.Value != tc.wantValue {
				t.Errorf("value = %q, want %q", got.Value, tc.wantValue)
			}
			if len(got.Items) != len(tc.wantItems) {
				t.Fatalf("items = %v, want %v", got.Items, tc.wantItems)
			}
			for i, want := range tc.wantItems {
				if got.Items[i] != want {
					t.Errorf("items[%d] = %v, want %v", i, got.Items[i], want)
				}
			}
		})
	}
}

// An unanswered question carries no answer at all.
func TestAssessmentUnansweredQuestionHasNoAnswer(t *testing.T) {
	var a Assessment
	raw := `{"id":"1","content":[{"id":"q1","name":"Business problem"}]}`
	if err := json.Unmarshal([]byte(raw), &a); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(a.Content) != 1 {
		t.Fatalf("content = %v, want one question", a.Content)
	}
	if a.Content[0].Answer != nil {
		t.Errorf("answer = %v, want nil", a.Content[0].Answer)
	}
}
