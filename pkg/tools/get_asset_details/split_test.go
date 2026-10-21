package get_asset_details

import (
	"reflect"
	"testing"
)

func TestSplitMultiValue(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want []string
	}{
		{"one value", "GDPR", []string{"GDPR"}},
		{"several values", "GDPR;HIPAA;SOX", []string{"GDPR", "HIPAA", "SOX"}},
		{"escaped semicolon stays inside the value", `A\;B;C`, []string{"A;B", "C"}},
		{"only an escaped semicolon", `A\;B`, []string{"A;B"}},
		{"backslash not before a semicolon is kept", `A\B;C`, []string{`A\B`, "C"}},
		{"empty text", "", []string{""}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := splitMultiValue(tt.in); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("splitMultiValue(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}
