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

// hasAttr reports whether the resolved assignment offers an attribute type with
// the given resource id.
func hasAttr(a *EditAssetAssignment, id string) bool {
	for _, at := range a.AttributeTypes {
		if at.ID == id {
			return true
		}
	}
	return false
}

// findRel returns the resolved relation entry with the given resource id, or nil.
func findRel(a *EditAssetAssignment, id string) *EditAssetAssignmentRelationType {
	for i := range a.RelationTypes {
		if a.RelationTypes[i].ID == id {
			return &a.RelationTypes[i]
		}
	}
	return nil
}

// TestGetEffectiveAssignmentForAsset_IncludesDirectTraitInheritances asserts that
// characteristics contributed by a Trait applied directly to the asset type
// (traitAssignmentInheritances) — both attributes AND relations — are folded into
// the resolved assignment, each read the same references-primary way as the
// top-level: membership from the reference, relation metadata joined from the
// entry's OWN characteristicTypes on the line id.
func TestGetEffectiveAssignmentForAsset_IncludesDirectTraitInheritances(t *testing.T) {
	const (
		assetID     = "019e027f-25b9-728f-9ed8-77c315ac377f"
		traitAttrID = "attr-trait-tag"
		traitRelID  = "cd000000-0000-0000-0000-0000000070aa"
	)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /rest/2.0/assignments/asset/"+assetID, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{
			"id": "assignment-effective",
			"assetType": {"id": "at-1", "name": "Acronym"},
			"assignedCharacteristicTypeReferences": [{
				"id": "line-own",
				"minimumOccurrences": 1,
				"assignedResourceReference": {"id": "attr-own", "name": "Definition", "resourceDiscriminator": "StringAttributeType"}
			}],
			"traitAssignmentInheritances": [{
				"assignedCharacteristicTypeReferences": [{
					"id": "line-trait-attr",
					"minimumOccurrences": 0,
					"assignedResourceReference": {"id": "` + traitAttrID + `", "name": "Tag", "resourceDiscriminator": "StringAttributeType"}
				}, {
					"id": "line-trait-rel",
					"roleDirection": "TO_TARGET",
					"assignedResourceReference": {"id": "` + traitRelID + `", "name": "governed by", "resourceDiscriminator": "RelationType"}
				}],
				"characteristicTypes": [
					{"id": "line-trait-rel", "role": "governed by", "coRole": "governs"}
				]
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

	if !hasAttr(got, "attr-own") {
		t.Errorf("top-level attribute dropped: %+v", got.AttributeTypes)
	}
	if !hasAttr(got, traitAttrID) {
		t.Errorf("direct-trait attribute not merged: %+v", got.AttributeTypes)
	}
	rel := findRel(got, traitRelID)
	if rel == nil {
		t.Fatalf("direct-trait relation not merged: %+v", got.RelationTypes)
	}
	// Role/coRole come only from the trait entry's own characteristicTypes; empty
	// would mean the per-entry join key was wrong.
	if rel.Role != "governed by" || rel.CoRole != "governs" {
		t.Errorf("direct-trait relation metadata join failed: %+v", rel)
	}
}

// TestGetEffectiveAssignmentForAsset_IncludesAncestorTraitInheritances asserts
// characteristics contributed by a Trait on an ANCESTOR asset type
// (assignmentInheritances) — whose characteristics nest one level deeper under
// each entry's own traitAssignmentInheritances — are folded in, attributes AND
// relations alike, with relation metadata joined from the nested entry's own
// characteristicTypes.
func TestGetEffectiveAssignmentForAsset_IncludesAncestorTraitInheritances(t *testing.T) {
	const (
		assetID   = "019e027f-25b9-728f-9ed8-77c315ac377f"
		ancAttrID = "attr-anc-source"
		ancRelID  = "cd000000-0000-0000-0000-0000000070bb"
	)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /rest/2.0/assignments/asset/"+assetID, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{
			"id": "assignment-effective",
			"assetType": {"id": "at-1", "name": "Acronym"},
			"assignedCharacteristicTypeReferences": [{
				"id": "line-own",
				"minimumOccurrences": 1,
				"assignedResourceReference": {"id": "attr-own", "name": "Definition", "resourceDiscriminator": "StringAttributeType"}
			}],
			"assignmentInheritances": [{
				"traitAssignmentInheritances": [{
					"assignedCharacteristicTypeReferences": [{
						"id": "line-anc-attr",
						"minimumOccurrences": 0,
						"assignedResourceReference": {"id": "` + ancAttrID + `", "name": "Source System", "resourceDiscriminator": "StringAttributeType"}
					}, {
						"id": "line-anc-rel",
						"roleDirection": "TO_SOURCE",
						"assignedResourceReference": {"id": "` + ancRelID + `", "name": "classified by", "resourceDiscriminator": "RelationType"}
					}],
					"characteristicTypes": [
						{"id": "line-anc-rel", "role": "classified by", "coRole": "classifies"}
					]
				}]
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

	if !hasAttr(got, ancAttrID) {
		t.Errorf("ancestor-trait attribute not merged: %+v", got.AttributeTypes)
	}
	rel := findRel(got, ancRelID)
	if rel == nil {
		t.Fatalf("ancestor-trait relation not merged: %+v", got.RelationTypes)
	}
	if rel.Role != "classified by" || rel.CoRole != "classifies" {
		t.Errorf("ancestor-trait relation metadata join failed: %+v", rel)
	}
	if !rel.Reversed {
		t.Errorf("TO_SOURCE ancestor-trait relation should be reversed: %+v", rel)
	}
}

// TestGetEffectiveAssignmentForAsset_ShadowsClosestSource asserts closest-wins
// precedence: a characteristic present in more than one source appears once,
// carrying the closest source's values (no field-level blending). It covers both
// ranks — top-level over direct-trait, and direct-trait over ancestor-trait —
// using the observable required flag (minimumOccurrences) as the differing field.
func TestGetEffectiveAssignmentForAsset_ShadowsClosestSource(t *testing.T) {
	const (
		assetID = "019e027f-25b9-728f-9ed8-77c315ac377f"
		attrTop = "attr-shared-top" // in top-level (required) + direct-trait (optional)
		attrMid = "attr-shared-mid" // in direct-trait (required) + ancestor-trait (optional)
	)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /rest/2.0/assignments/asset/"+assetID, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{
			"id": "assignment-effective",
			"assetType": {"id": "at-1", "name": "Acronym"},
			"assignedCharacteristicTypeReferences": [{
				"id": "line-top",
				"minimumOccurrences": 1,
				"assignedResourceReference": {"id": "` + attrTop + `", "name": "Shared Top", "resourceDiscriminator": "StringAttributeType"}
			}],
			"traitAssignmentInheritances": [{
				"assignedCharacteristicTypeReferences": [{
					"id": "line-top-trait",
					"minimumOccurrences": 0,
					"assignedResourceReference": {"id": "` + attrTop + `", "name": "Shared Top", "resourceDiscriminator": "StringAttributeType"}
				}, {
					"id": "line-mid-trait",
					"minimumOccurrences": 1,
					"assignedResourceReference": {"id": "` + attrMid + `", "name": "Shared Mid", "resourceDiscriminator": "StringAttributeType"}
				}]
			}],
			"assignmentInheritances": [{
				"traitAssignmentInheritances": [{
					"assignedCharacteristicTypeReferences": [{
						"id": "line-mid-anc",
						"minimumOccurrences": 0,
						"assignedResourceReference": {"id": "` + attrMid + `", "name": "Shared Mid", "resourceDiscriminator": "StringAttributeType"}
					}]
				}]
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

	var top, mid []EditAssetAssignmentAttributeType
	for _, at := range got.AttributeTypes {
		switch at.ID {
		case attrTop:
			top = append(top, at)
		case attrMid:
			mid = append(mid, at)
		}
	}
	if len(top) != 1 {
		t.Fatalf("expected shared-top attribute once, got %d: %+v", len(top), top)
	}
	if !top[0].Required {
		t.Errorf("top-level should shadow direct-trait: expected required (top-level minOccurs=1), got %+v", top[0])
	}
	if len(mid) != 1 {
		t.Fatalf("expected shared-mid attribute once, got %d: %+v", len(mid), mid)
	}
	if !mid[0].Required {
		t.Errorf("direct-trait should shadow ancestor-trait: expected required (direct-trait minOccurs=1), got %+v", mid[0])
	}
}

// TestGetEffectiveAssignmentForAsset_ExcludesDerivedRelationTypeFromTrait asserts
// the DRT guard applies on the trait-inheritance fields too: a derived relation
// type contributed by a Trait is not offered, while its explicit sibling is.
func TestGetEffectiveAssignmentForAsset_ExcludesDerivedRelationTypeFromTrait(t *testing.T) {
	const (
		assetID       = "019e027f-25b9-728f-9ed8-77c315ac377f"
		explicitRelID = "cd000000-0000-0000-0000-0000000070c1"
		derivedRelID  = "cd000000-0000-0000-0000-0000000070c2"
	)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /rest/2.0/assignments/asset/"+assetID, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{
			"id": "assignment-effective",
			"assetType": {"id": "at-1", "name": "Acronym"},
			"traitAssignmentInheritances": [{
				"assignedCharacteristicTypeReferences": [{
					"id": "line-trait-explicit",
					"roleDirection": "TO_TARGET",
					"assignedResourceReference": {"id": "` + explicitRelID + `", "name": "relates to", "resourceDiscriminator": "RelationType"}
				}, {
					"id": "line-trait-derived",
					"roleDirection": "TO_TARGET",
					"assignedResourceReference": {"id": "` + derivedRelID + `", "name": "derived from", "resourceDiscriminator": "DerivedRelationType"}
				}],
				"characteristicTypes": [
					{"id": "line-trait-explicit", "role": "relates to", "coRole": "related from"},
					{"id": "line-trait-derived", "role": "derived from", "coRole": "source of"}
				]
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
	if findRel(got, derivedRelID) != nil {
		t.Errorf("derived relation from trait inheritance should be excluded: %+v", got.RelationTypes)
	}
	if findRel(got, explicitRelID) == nil {
		t.Errorf("explicit relation from trait inheritance should be kept: %+v", got.RelationTypes)
	}
}
