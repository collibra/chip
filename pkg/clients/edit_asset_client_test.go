package clients

import (
	"encoding/json"
	"testing"
)

// TestEditAssetAttributeInstance_ValueAcceptsAnyScalar covers Collibra's
// behavior of returning attribute values typed by their attribute kind:
// strings come back quoted, numbers as numeric literals, booleans as bare
// true/false, and unset values as null. The unmarshaler must accept all of
// these and present them as a single printable string to consumers.
func TestEditAssetAttributeInstance_ValueAcceptsAnyScalar(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want string
	}{
		{"string value", `{"id":"a","value":"hello"}`, "hello"},
		{"empty string value", `{"id":"a","value":""}`, ""},
		{"numeric value (int)", `{"id":"a","value":42}`, "42"},
		{"numeric value (float)", `{"id":"a","value":3.14}`, "3.14"},
		{"boolean value true", `{"id":"a","value":true}`, "true"},
		{"boolean value false", `{"id":"a","value":false}`, "false"},
		{"null value", `{"id":"a","value":null}`, ""},
		{"missing value field", `{"id":"a"}`, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var got EditAssetAttributeInstance
			if err := json.Unmarshal([]byte(tc.raw), &got); err != nil {
				t.Fatalf("Unmarshal: %v", err)
			}
			if got.Value != tc.want {
				t.Fatalf("Value = %q, want %q", got.Value, tc.want)
			}
		})
	}
}

// TestEditAssetAttributeInstance_ListPageDecodesMixedKinds covers the path
// the edit_asset tool actually hits: a paginated /rest/2.0/attributes?assetId=
// response with attribute values of mixed kinds in the same payload.
// Reproduces the bug reported in DEV-177761 where a numeric attribute on a
// freshly-created asset broke the entire attribute fetch.
func TestEditAssetAttributeInstance_ListPageDecodesMixedKinds(t *testing.T) {
	raw := `{
		"total": 3,
		"offset": 0,
		"limit": 100,
		"results": [
			{"id":"a1","type":{"id":"t1","name":"Definition"},"asset":{"id":"x"},"value":"Some text"},
			{"id":"a2","type":{"id":"t2","name":"Row Count"},"asset":{"id":"x"},"value":12345},
			{"id":"a3","type":{"id":"t3","name":"Is Public"},"asset":{"id":"x"},"value":true}
		]
	}`
	var page editAssetAttributesList
	if err := json.Unmarshal([]byte(raw), &page); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if len(page.Results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(page.Results))
	}
	wantValues := []string{"Some text", "12345", "true"}
	for i, w := range wantValues {
		if page.Results[i].Value != w {
			t.Fatalf("results[%d].Value = %q, want %q", i, page.Results[i].Value, w)
		}
	}
}
