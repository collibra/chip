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

// TestGetAssignmentForAssetType_InheritsParentChain mirrors the live KPI
// case: the subtype's own assignment carries only attributes, while its
// relation types live on the parent type's assignment (with empty
// domainTypes — Collibra's inherit sentinel). The lookup must walk the
// parent chain and merge both levels, keeping head (TO_TARGET) and tail
// (TO_SOURCE) directions and deduping anything repeated at a lower level.
func TestGetAssignmentForAssetType_InheritsParentChain(t *testing.T) {
	const (
		kpiTypeID    = "00000000-0000-0000-0000-000000031107"
		parentTypeID = "00000000-0000-0000-0000-000000031101"
		domTypeID    = "00000000-0000-0000-0000-000000000001"
		relTypeID    = "cd000000-0000-0000-0000-000000007002"
	)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /rest/2.0/assignments/assetType/"+kpiTypeID, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`[{
			"id": "assignment-kpi",
			"domainTypes": [{"id": "` + domTypeID + `", "name": "Business Glossary"}],
			"characteristicTypes": [{
				"id": "char-attr",
				"minimumOccurrences": 0,
				"assignedCharacteristicTypeDiscriminator": "AttributeType",
				"attributeType": {"id": "attr-1", "name": "Definition", "resourceType": "StringAttributeType"}
			}]
		}]`))
	})
	mux.HandleFunc("GET /rest/2.0/assetTypes/"+kpiTypeID, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"id": "` + kpiTypeID + `", "name": "KPI", "parent": {"id": "` + parentTypeID + `", "name": "Business Asset"}}`))
	})
	mux.HandleFunc("GET /rest/2.0/assignments/assetType/"+parentTypeID, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`[{
			"id": "assignment-parent",
			"domainTypes": [],
			"characteristicTypes": [{
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
			}, {
				"id": "char-attr-dup",
				"minimumOccurrences": 1,
				"assignedCharacteristicTypeDiscriminator": "AttributeType",
				"attributeType": {"id": "attr-1", "name": "Definition", "resourceType": "StringAttributeType"}
			}]
		}]`))
	})
	mux.HandleFunc("GET /rest/2.0/assetTypes/"+parentTypeID, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"id": "` + parentTypeID + `", "name": "Business Asset"}`))
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	client := testutil.NewClient(srv)

	got, err := GetAssignmentForAssetType(t.Context(), client, kpiTypeID, domTypeID)
	if err != nil {
		t.Fatalf("GetAssignmentForAssetType: %v", err)
	}

	if len(got.AttributeTypes) != 1 {
		t.Fatalf("expected 1 attribute type (deduped across levels), got %d: %+v", len(got.AttributeTypes), got.AttributeTypes)
	}
	if got.AttributeTypes[0].Required {
		t.Errorf("subtype's own (min:0) attribute entry should win over the parent's min:1 duplicate")
	}
	if len(got.RelationTypes) != 2 {
		t.Fatalf("expected forward+reversed relation entries from parent assignment, got %d: %+v", len(got.RelationTypes), got.RelationTypes)
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
