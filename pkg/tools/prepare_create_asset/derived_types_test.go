package prepare_create_asset_test

import (
	"strings"
	"testing"

	"github.com/collibra/chip/pkg/tools/prepare_create_asset"
)

// TestPrepare_DerivedTypes_AreNeverResolvedByUUID answers one question: does
// chip ever resolve a derived relation type or derived attribute type by uuid?
// The platform is dropping derived types from the by-uuid lookups, so any such
// call turns into a 404.
//
// The mock server stands in for a post-change instance: it serves an assignment
// carrying one derived relation type and one derived attribute type alongside
// the explicit ones, 404s every id it doesn't know (which is what a derived id
// becomes), and records every id chip looks up. Green means chip is unaffected.
func TestPrepare_DerivedTypes_AreNeverResolvedByUUID(t *testing.T) {
	m := &mockDGC{t: t, derivedTypes: true}

	out, err := prepare_create_asset.NewTool(client(t, m)).Handler(t.Context(), prepare_create_asset.Input{
		AssetType:         btTypeName,
		Domain:            glossaryDomain,
		IncludeStringType: true,
	})
	if err != nil {
		t.Fatalf("Handler: %v", err)
	}
	if out.Status != prepare_create_asset.StatusReady {
		t.Fatalf("want status ready, got %q (%s)", out.Status, out.Message)
	}

	for _, id := range m.asked {
		switch id {
		case drtID:
			t.Errorf("AFFECTED: derived relation type %s resolved by uuid", id)
		case datID:
			t.Errorf("AFFECTED: derived attribute type %s resolved by uuid", id)
		}
	}

	for _, e := range out.AttributeSchema {
		if e.AttributeTypeID == datID {
			t.Errorf("derived attribute type surfaced as an attribute slot: %+v", e)
		}
	}
	for _, e := range out.RelationTypes {
		if e.RelationTypeID == drtID {
			t.Errorf("derived relation type surfaced as a relation slot: %+v", e)
		}
	}

	if strings.Contains(out.Message, "could not be fully hydrated") {
		t.Errorf("a lookup 404 reached the agent-visible message: %s", out.Message)
	}
}
