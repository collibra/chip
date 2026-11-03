package edit_asset_test

import (
	"strings"
	"testing"

	"github.com/collibra/chip/pkg/clients"
	"github.com/collibra/chip/pkg/tools/edit_asset"
)

const (
	usesAgentRelTypeID = "00000000-0000-0000-0000-00000000a101"
	usesModelRelTypeID = "00000000-0000-0000-0000-00000000a102"
	governsRelTypeID   = "00000000-0000-0000-0000-00000000a103"
	unknownRelTypeID   = "00000000-0000-0000-0000-00000000a1ff"
)

// stubWithSharedRole serves an assignment with two relation types that share
// the forward role "uses" toward different target types.
func stubWithSharedRole() *stub {
	s := newStub()
	s.relationTypes = append(s.relationTypes,
		clients.EditAssetAssignmentRelationType{
			ID:         usesAgentRelTypeID,
			Role:       "uses",
			CoRole:     "is used by",
			SourceType: &clients.EditAssetTypeRef{ID: testAssetTypeID, Name: "Business Term"},
			TargetType: &clients.EditAssetTypeRef{ID: "00000000-0000-0000-0000-00000000b001", Name: "Example Agent"},
		},
		clients.EditAssetAssignmentRelationType{
			ID:         usesModelRelTypeID,
			Role:       "uses",
			CoRole:     "is used by",
			SourceType: &clients.EditAssetTypeRef{ID: testAssetTypeID, Name: "Business Term"},
			TargetType: &clients.EditAssetTypeRef{ID: "00000000-0000-0000-0000-00000000b002", Name: "Example Model"},
		},
	)
	return s
}

func (s *stub) relationWrites() int {
	n := len(s.createdRelations)
	for _, batch := range s.bulkCreatedRelations {
		n += len(batch)
	}
	return n
}

func TestEditAsset_AddRelation_RelationTypeIDPicksTheRightTypeWhenTwoShareARole(t *testing.T) {
	s := stubWithSharedRole()
	out, err := runTool(t, s, edit_asset.Input{
		AssetID: testAssetID,
		Operations: []edit_asset.Operation{{
			Type: edit_asset.OpAddRelation, RelationTypeID: usesModelRelTypeID, TargetAssetID: targetAssetID,
		}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Status != edit_asset.StatusSuccess {
		t.Fatalf("expected success, got %q, results=%+v", out.Status, out.Results)
	}
	if len(s.createdRelations) != 1 {
		t.Fatalf("expected one POST to /rest/2.0/relations, got %+v", s.createdRelations)
	}
	got := s.createdRelations[0]
	if got.TypeID != usesModelRelTypeID || got.SourceID != testAssetID || got.TargetID != targetAssetID {
		t.Fatalf("unexpected relation payload: %+v", got)
	}
	if out.Results[0].RelationTypeID != usesModelRelTypeID {
		t.Errorf("result relationTypeId = %q, want %q", out.Results[0].RelationTypeID, usesModelRelTypeID)
	}
}

func TestEditAsset_AddRelation_SharedRoleNameAloneIsAmbiguous(t *testing.T) {
	s := stubWithSharedRole()
	out, err := runTool(t, s, edit_asset.Input{
		AssetID: testAssetID,
		Operations: []edit_asset.Operation{{
			Type: edit_asset.OpAddRelation, RelationType: "Uses", TargetAssetID: targetAssetID,
		}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Status != edit_asset.StatusError {
		t.Fatalf("expected error for an ambiguous role, got %q", out.Status)
	}
	msg := out.Results[0].Error
	for _, want := range []string{usesAgentRelTypeID, usesModelRelTypeID, "Example Agent", "Example Model", "relationTypeId"} {
		if !strings.Contains(msg, want) {
			t.Errorf("ambiguity error should mention %q: %s", want, msg)
		}
	}
	if s.relationWrites() != 0 {
		t.Fatalf("expected no relation written on an ambiguous role, got %+v", s.createdRelations)
	}
}

func TestEditAsset_AddRelation_RelationTypeIDNotInAssignmentIsRefusedBeforeAnyWrite(t *testing.T) {
	s := stubWithSharedRole()
	out, err := runTool(t, s, edit_asset.Input{
		AssetID: testAssetID,
		Operations: []edit_asset.Operation{{
			Type: edit_asset.OpAddRelation, RelationTypeID: unknownRelTypeID, TargetAssetID: targetAssetID,
		}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Status != edit_asset.StatusError || !strings.Contains(out.Results[0].Error, "not assigned") {
		t.Fatalf("expected a not-assigned error, got %+v", out)
	}
	if !strings.Contains(out.Results[0].Error, usesAgentRelTypeID) {
		t.Errorf("error should list the assigned relation types: %s", out.Results[0].Error)
	}
	if s.relationWrites() != 0 {
		t.Fatalf("expected no relation written, got %+v", s.createdRelations)
	}
}

func TestEditAsset_AddRelation_RelationTypeIDWinsOverRelationType(t *testing.T) {
	s := stubWithSharedRole()
	out, _ := runTool(t, s, edit_asset.Input{
		AssetID: testAssetID,
		Operations: []edit_asset.Operation{{
			Type: edit_asset.OpAddRelation, RelationType: "is synonym of", RelationTypeID: usesAgentRelTypeID, TargetAssetID: targetAssetID,
		}},
	})
	if out.Status != edit_asset.StatusSuccess || len(s.createdRelations) != 1 || s.createdRelations[0].TypeID != usesAgentRelTypeID {
		t.Fatalf("expected the relationTypeId to be used, got %+v / %+v", out, s.createdRelations)
	}
}

func TestEditAsset_AddRelation_RelationTypeIDOnTheTargetSideFlipsDirection(t *testing.T) {
	s := newStub()
	s.relationTypes = append(s.relationTypes, clients.EditAssetAssignmentRelationType{
		ID:         governsRelTypeID,
		Role:       "governs",
		CoRole:     "is governed by",
		SourceType: &clients.EditAssetTypeRef{ID: "00000000-0000-0000-0000-00000000b003", Name: "Example Policy"},
		TargetType: &clients.EditAssetTypeRef{ID: testAssetTypeID, Name: "Business Term"},
		Reversed:   true,
	})
	out, _ := runTool(t, s, edit_asset.Input{
		AssetID: testAssetID,
		Operations: []edit_asset.Operation{{
			Type: edit_asset.OpAddRelation, RelationTypeID: governsRelTypeID, TargetAssetID: targetAssetID,
		}},
	})
	if out.Status != edit_asset.StatusSuccess || len(s.createdRelations) != 1 {
		t.Fatalf("expected one relation, got %+v / %+v", out, s.createdRelations)
	}
	got := s.createdRelations[0]
	if got.SourceID != targetAssetID || got.TargetID != testAssetID || got.TypeID != governsRelTypeID {
		t.Fatalf("expected the edited asset as target, got %+v", got)
	}
}

func TestEditAsset_AddRelation_BulkKeepsTheInverseDirection(t *testing.T) {
	s := newStub()
	s.relationTypes = append(s.relationTypes, clients.EditAssetAssignmentRelationType{
		ID:         governsRelTypeID,
		Role:       "governs",
		CoRole:     "is governed by",
		SourceType: &clients.EditAssetTypeRef{ID: "00000000-0000-0000-0000-00000000b003", Name: "Example Policy"},
		TargetType: &clients.EditAssetTypeRef{ID: testAssetTypeID, Name: "Business Term"},
		Reversed:   true,
	})
	otherTarget := "018d3602-aaaa-0000-0000-000000000002"
	out, _ := runTool(t, s, edit_asset.Input{
		AssetID: testAssetID,
		Operations: []edit_asset.Operation{
			{Type: edit_asset.OpAddRelation, RelationTypeID: governsRelTypeID, TargetAssetID: targetAssetID},
			{Type: edit_asset.OpAddRelation, RelationType: "is synonym of", TargetAssetID: otherTarget},
		},
	})
	if out.Status != edit_asset.StatusSuccess || len(s.bulkCreatedRelations) != 1 || len(s.bulkCreatedRelations[0]) != 2 {
		t.Fatalf("expected one bulk POST with 2 items, got %+v / %+v", out, s.bulkCreatedRelations)
	}
	inverse, forward := s.bulkCreatedRelations[0][0], s.bulkCreatedRelations[0][1]
	if inverse.SourceID != targetAssetID || inverse.TargetID != testAssetID {
		t.Errorf("inverse relation not flipped in bulk: %+v", inverse)
	}
	if forward.SourceID != testAssetID || forward.TargetID != otherTarget {
		t.Errorf("forward relation changed in bulk: %+v", forward)
	}
}

func TestEditAsset_AddRelation_InvalidRelationTypeID(t *testing.T) {
	s := newStub()
	out, _ := runTool(t, s, edit_asset.Input{
		AssetID: testAssetID,
		Operations: []edit_asset.Operation{{
			Type: edit_asset.OpAddRelation, RelationTypeID: "not-a-uuid", TargetAssetID: targetAssetID,
		}},
	})
	if out.Status != edit_asset.StatusError || !strings.Contains(out.Results[0].Error, "relationTypeId") || s.relationWrites() != 0 {
		t.Fatalf("expected a relationTypeId validation error and no write, got %+v", out)
	}
}

func TestEditAsset_AddRelation_NeedsRelationTypeOrID(t *testing.T) {
	s := newStub()
	out, _ := runTool(t, s, edit_asset.Input{
		AssetID:    testAssetID,
		Operations: []edit_asset.Operation{{Type: edit_asset.OpAddRelation, TargetAssetID: targetAssetID}},
	})
	if out.Status != edit_asset.StatusError || !strings.Contains(out.Results[0].Error, "relationTypeId or relationType is required") {
		t.Fatalf("expected a required-field error, got %+v", out)
	}
}
