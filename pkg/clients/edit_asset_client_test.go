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

// TestGetEffectiveAssignmentForAsset_ParsesResolvedAssignment checks parsing of
// the per-asset endpoint's single Assignment object in the references-primary
// orientation: attributes and required-ness come from
// assignedCharacteristicTypeReferences, while a relation's role / co-role /
// direction / target arrive via the deprecated characteristicTypes joined on the
// LINE id (the reference's own top-level id ↔ characteristicTypes[].id). Both
// relation directions (TO_TARGET / TO_SOURCE) resolve as distinct entries.
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
			"assignedCharacteristicTypeReferences": [{
				"id": "line-def",
				"minimumOccurrences": 1,
				"assignedResourceReference": {"id": "attr-def", "name": "Definition", "resourceDiscriminator": "StringAttributeType"}
			}, {
				"id": "line-note",
				"minimumOccurrences": 0,
				"assignedResourceReference": {"id": "attr-note", "name": "Note", "resourceDiscriminator": "StringAttributeType"}
			}, {
				"id": "line-rel-fwd",
				"minimumOccurrences": 0,
				"roleDirection": "TO_TARGET",
				"assignedResourceReference": {"id": "` + relTypeID + `", "name": "calculated using", "resourceDiscriminator": "RelationType"}
			}, {
				"id": "line-rel-rev",
				"minimumOccurrences": 0,
				"roleDirection": "TO_SOURCE",
				"assignedResourceReference": {"id": "` + relTypeID + `", "name": "calculated using", "resourceDiscriminator": "RelationType"}
			}],
			"characteristicTypes": [
				{"id": "line-rel-fwd", "role": "calculated using", "coRole": "used to calculate"},
				{"id": "line-rel-rev", "role": "calculated using", "coRole": "used to calculate"}
			]
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
		// Role / co-role are carried only by the joined characteristicTypes; if the
		// join key were wrong they'd be empty.
		if rt.ID != relTypeID || rt.Role != "calculated using" || rt.CoRole != "used to calculate" {
			t.Errorf("unexpected relation entry (role/coRole join failed?): %+v", rt)
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

// TestGetEffectiveAssignmentForAsset_ExcludesDerivedRelationType asserts a
// derived relation type (assignedResourceReference.resourceDiscriminator ==
// "DerivedRelationType") is not offered, while its explicit "RelationType"
// sibling is kept. The deprecated characteristicTypes list marks both as
// ordinary relations, so only the references-primary discriminator distinguishes
// them.
func TestGetEffectiveAssignmentForAsset_ExcludesDerivedRelationType(t *testing.T) {
	const (
		assetID       = "019e027f-25b9-728f-9ed8-77c315ac377f"
		explicitRelID = "cd000000-0000-0000-0000-000000007001"
		derivedRelID  = "cd000000-0000-0000-0000-000000007002"
	)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /rest/2.0/assignments/asset/"+assetID, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{
			"id": "assignment-effective",
			"assetType": {"id": "at-1", "name": "Acronym"},
			"assignedCharacteristicTypeReferences": [{
				"id": "line-explicit",
				"roleDirection": "TO_TARGET",
				"assignedResourceReference": {"id": "` + explicitRelID + `", "name": "relates to", "resourceDiscriminator": "RelationType"}
			}, {
				"id": "line-derived",
				"roleDirection": "TO_TARGET",
				"assignedResourceReference": {"id": "` + derivedRelID + `", "name": "derived from", "resourceDiscriminator": "DerivedRelationType"}
			}],
			"characteristicTypes": [
				{"id": "line-explicit", "role": "relates to", "coRole": "related from"},
				{"id": "line-derived", "role": "derived from", "coRole": "source of"}
			]
		}`))
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	client := testutil.NewClient(srv)

	got, err := GetEffectiveAssignmentForAsset(t.Context(), client, assetID)
	if err != nil {
		t.Fatalf("GetEffectiveAssignmentForAsset: %v", err)
	}

	if len(got.RelationTypes) != 1 {
		t.Fatalf("expected only the explicit relation type, got %d: %+v", len(got.RelationTypes), got.RelationTypes)
	}
	if got.RelationTypes[0].ID != explicitRelID {
		t.Errorf("expected explicit relation %q, got %+v", explicitRelID, got.RelationTypes[0])
	}
}

// TestGetEffectiveAssignmentForAsset_NormalizesDateTimeAttributeType asserts the
// platform-bug kind "DateTimeAttributeType" is surfaced to the model as
// "DateAttributeType".
func TestGetEffectiveAssignmentForAsset_NormalizesDateTimeAttributeType(t *testing.T) {
	const assetID = "019e027f-25b9-728f-9ed8-77c315ac377f"

	mux := http.NewServeMux()
	mux.HandleFunc("GET /rest/2.0/assignments/asset/"+assetID, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{
			"id": "assignment-effective",
			"assetType": {"id": "at-1", "name": "Acronym"},
			"assignedCharacteristicTypeReferences": [{
				"id": "line-dt",
				"minimumOccurrences": 0,
				"assignedResourceReference": {"id": "attr-dt", "name": "Last Reviewed", "resourceDiscriminator": "DateTimeAttributeType"}
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
	if len(got.AttributeTypes) != 1 {
		t.Fatalf("expected 1 attribute type, got %d: %+v", len(got.AttributeTypes), got.AttributeTypes)
	}
	if got.AttributeTypes[0].Kind != "DateAttributeType" {
		t.Errorf("expected DateTimeAttributeType normalized to DateAttributeType, got %q", got.AttributeTypes[0].Kind)
	}
}
