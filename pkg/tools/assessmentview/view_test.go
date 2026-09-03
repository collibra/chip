package assessmentview

import (
	"encoding/json"
	"testing"

	"github.com/collibra/chip/pkg/clients"
)

// The API sends an answer value as an untyped JSON value. Each shape must
// project onto a concrete field: a scalar onto value, a choice onto items.
func TestAnswerValueProjection(t *testing.T) {
	tests := []struct {
		name      string
		value     any
		wantValue string
		wantItems []Item
	}{
		{name: "text", value: "reduce churn", wantValue: "reduce churn"},
		{name: "number keeps integer form", value: float64(42), wantValue: "42"},
		{name: "number keeps decimals", value: 3.5, wantValue: "3.5"},
		{name: "boolean", value: true, wantValue: "true"},
		{name: "json number", value: json.Number("8"), wantValue: "8"},
		{name: "absent value", value: nil, wantValue: ""},
		{
			name:      "items become a list of options",
			value:     []any{map[string]any{"id": "yes", "value": "Yes"}},
			wantItems: []Item{{ID: "yes", Value: "Yes"}},
		},
		{
			name:      "unrecognised list falls back to json",
			value:     []any{"a", "b"},
			wantValue: `["a","b"]`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := newAnswer(&clients.Answer{Type: "TEXT", Value: tc.value})
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

func TestNewPtrKeepsNil(t *testing.T) {
	if got := NewPtr(nil); got != nil {
		t.Errorf("NewPtr(nil) = %v, want nil", got)
	}
}

// An unanswered question carries no answer at all.
func TestUnansweredQuestionHasNoAnswer(t *testing.T) {
	got := New(clients.Assessment{
		Content: []clients.QuestionAndAnswer{{ID: "q1", Name: "Business problem"}},
	})
	if len(got.Content) != 1 {
		t.Fatalf("content = %v, want one question", got.Content)
	}
	if got.Content[0].Answer != nil {
		t.Errorf("answer = %v, want nil", got.Content[0].Answer)
	}
}
