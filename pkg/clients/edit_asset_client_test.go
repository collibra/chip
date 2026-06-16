package clients

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/collibra/chip/pkg/tools/testutil"
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
// Regression guard: a numeric attribute on a freshly-created asset
// previously broke the entire attribute fetch.
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

// TestGetEffectiveAssignmentForAsset_ParsesResolvedAssignment mirrors the live
// per-asset endpoint: Collibra returns a single Assignment object whose
// characteristicTypes already include everything that applies to the asset —
// the asset type's own attributes plus characteristics inherited from parents
// (the KPI add_relation case). chip parses it as-is, with no client-side
// domain-type filtering, keeping required-ness from minimumOccurrences and both
// head (TO_TARGET) and tail (TO_SOURCE) relation directions.
func TestGetEffectiveAssignmentForAsset_ParsesResolvedAssignment(t *testing.T) {
	const (
		assetID   = "019e027f-25b9-728f-9ed8-77c315ac377f"
		relTypeID = "cd000000-0000-0000-0000-000000007002"
	)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /rest/2.0/assignments/asset/"+assetID, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{
			"id": "assignment-effective",
			"assetType": {"id": "00000000-0000-0000-0000-000000011003", "name": "Acronym"},
			"domainTypes": [{"id": "00000000-0000-0000-0000-000000010001", "name": "Glossary"}],
			"characteristicTypes": [{
				"id": "char-def",
				"minimumOccurrences": 1,
				"assignedCharacteristicTypeDiscriminator": "AttributeType",
				"attributeType": {"id": "attr-def", "name": "Definition", "resourceType": "StringAttributeType"}
			}, {
				"id": "char-note",
				"minimumOccurrences": 0,
				"assignedCharacteristicTypeDiscriminator": "AttributeType",
				"attributeType": {"id": "attr-note", "name": "Note", "resourceType": "StringAttributeType"}
			}, {
				"id": "char-rel-fwd",
				"minimumOccurrences": 0,
				"roleDirection": "TO_TARGET",
				"assignedCharacteristicTypeDiscriminator": "RelationType",
				"relationType": {"id": "` + relTypeID + `", "role": "calculated using", "coRole": "used to calculate"}
			}, {
				"id": "char-rel-rev",
				"minimumOccurrences": 0,
				"roleDirection": "TO_SOURCE",
				"assignedCharacteristicTypeDiscriminator": "RelationType",
				"relationType": {"id": "` + relTypeID + `", "role": "calculated using", "coRole": "used to calculate"}
			}]
		}`))
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	client := testutil.NewClient(srv)

	got, err := GetEffectiveAssignmentForAsset(t.Context(), client, assetID)
	if err != nil {
		t.Fatalf("GetEffectiveAssignmentForAsset: %v", err)
	}

	if len(got.AttributeTypes) != 2 {
		t.Fatalf("expected 2 attribute types, got %d: %+v", len(got.AttributeTypes), got.AttributeTypes)
	}
	var def *EditAssetAssignmentAttributeType
	for i := range got.AttributeTypes {
		if got.AttributeTypes[i].Name == "Definition" {
			def = &got.AttributeTypes[i]
		}
	}
	if def == nil {
		t.Fatalf("Definition attribute missing from effective assignment: %+v", got.AttributeTypes)
	}
	if !def.Required {
		t.Errorf("Definition (minimumOccurrences=1) should be required")
	}
	if len(got.RelationTypes) != 2 {
		t.Fatalf("expected forward+reversed relation entries, got %d: %+v", len(got.RelationTypes), got.RelationTypes)
	}
	var sawForward, sawReversed bool
	for _, rt := range got.RelationTypes {
		if rt.ID != relTypeID || rt.Role != "calculated using" {
			t.Errorf("unexpected relation entry: %+v", rt)
		}
		if rt.Reversed {
			sawReversed = true
		} else {
			sawForward = true
		}
	}
	if !sawForward || !sawReversed {
		t.Errorf("expected both directions, forward=%v reversed=%v", sawForward, sawReversed)
	}
}
