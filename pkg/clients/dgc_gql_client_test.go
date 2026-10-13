package clients

import (
	"encoding/json"
	"slices"
	"testing"
)

func TestMultiValueAttributeKeepsEscapedSemicolonInsideValue(t *testing.T) {
	body := []byte(`{"stringValues": "a\\;a;b\\b;c\"c", "type": {"name": "Tags"}}`)

	var attr MultiValueAttribute
	if err := json.Unmarshal(body, &attr); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := []string{`a;a`, `b\b`, `c"c`}
	if !slices.Equal(attr.Values, want) {
		t.Errorf("got values %q, want %q", attr.Values, want)
	}
	if attr.Type == nil || attr.Type.Name != "Tags" {
		t.Errorf("got type %+v, want Tags", attr.Type)
	}
}

func TestMultiValueAttributeRoundTripsThroughJSON(t *testing.T) {
	original := MultiValueAttribute{Values: []string{"PII", "a;b"}, Type: &AttributeType{Name: "Confidentiality"}}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var decoded MultiValueAttribute
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !slices.Equal(decoded.Values, original.Values) {
		t.Errorf("got values %q, want %q", decoded.Values, original.Values)
	}
}
