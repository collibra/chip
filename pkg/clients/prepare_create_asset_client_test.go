package clients

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/collibra/chip/pkg/tools/testutil"
)

// attrByID finds a resolved attribute slot by its attribute type id.
func attrByID(attrs []PrepareCreateScopedAttribute, id string) (PrepareCreateScopedAttribute, bool) {
	for _, a := range attrs {
		if a.AttributeTypeID == id {
			return a, true
		}
	}
	return PrepareCreateScopedAttribute{}, false
}

func attrRef(id, name string, min int) rawAssignedCharacteristicTypeReference {
	return rawAssignedCharacteristicTypeReference{
		ID: "ref-" + id,
		AssignedResourceReference: rawAssignmentResourceRef{
			ID: id, Name: name,
			ResourceType: "StringAttributeType", ResourceDiscriminator: "StringAttributeType",
		},
		MinimumOccurrences: min,
	}
}

// relRef builds a relation reference for a given relation-type resource id and
// role direction ("TO_TARGET" / "TO_SOURCE"). The direction is what keeps the
// two sides of a bidirectional relation distinct in the dedup key.
func relRef(id, roleDirection string) rawAssignedCharacteristicTypeReference {
	return rawAssignedCharacteristicTypeReference{
		ID: "ref-" + id + "-" + roleDirection,
		AssignedResourceReference: rawAssignmentResourceRef{
			ID: id, ResourceType: "RelationType", ResourceDiscriminator: "RelationType",
		},
		RoleDirection: roleDirection,
	}
}

// relTypeIDs collects the relation-type ids of the resolved relation slots, in
// order, for assertions that care only about which relations survived.
func relTypeIDs(rels []PrepareCreateScopedRelation) []string {
	out := make([]string, len(rels))
	for i, r := range rels {
		out[i] = r.RelationTypeID
	}
	return out
}

// When the asset type has its own assignment set, that single located level is
// used whole; characteristics from ancestor levels must NOT bleed in. This is
// the DEV-204211 regression: Data Product Port's parent-only Descriptive
// Example / Location attributes were leaking into the schema as required.
func TestSelectScopedAssignment_OwnAssignmentWins_DropsAncestorExtras(t *testing.T) {
	const domainType = "dt-contract"
	levels := []assignmentLevel{
		// level 0 — the asset type's own assignment.
		{raws: []rawScopedAssignment{{
			ID:          "asgn-own",
			DomainTypes: []rawAssignmentResourceRef{{ID: domainType, Name: "Data Contract"}},
			AssignedCharacteristicTypeReferences: []rawAssignedCharacteristicTypeReference{
				attrRef("own-1", "Own Attr", 1),
			},
		}}},
		// level 1 — a parent asset type that also lists the domain type and
		// carries extra required attributes. These must be dropped: the own
		// level is located, so level 1 is never consumed.
		{raws: []rawScopedAssignment{{
			ID:          "asgn-parent",
			DomainTypes: []rawAssignmentResourceRef{{ID: domainType, Name: "Data Contract"}},
			AssignedCharacteristicTypeReferences: []rawAssignedCharacteristicTypeReference{
				attrRef("loc", "Location", 1),
				attrRef("desc", "Descriptive Example", 1),
			},
		}}},
	}

	got, err := selectScopedAssignment(levels, domainType, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if a, ok := attrByID(got.Attributes, "own-1"); !ok {
		t.Errorf("own attribute must be present, got %+v", got.Attributes)
	} else if !a.FromOwnAssignment {
		t.Errorf("own-level attribute should have FromOwnAssignment=true")
	}
	if a, ok := attrByID(got.Attributes, "loc"); ok {
		t.Errorf("ancestor-only attribute Location must NOT bleed in, got %+v", a)
	}
	if a, ok := attrByID(got.Attributes, "desc"); ok {
		t.Errorf("ancestor-only attribute Descriptive Example must NOT bleed in, got %+v", a)
	}
	if len(got.Attributes) != 1 {
		t.Errorf("expected exactly the own attribute, got %d slots", len(got.Attributes))
	}
}

// An asset type with no assignment of its own resolves to the first ancestor
// level that carries one, used whole — the Acronym → Business Term walk-up. The
// ancestor level's slots carry FromOwnAssignment=false.
func TestSelectScopedAssignment_NoOwnAssignment_WalksUpToFirstAssignedAncestor(t *testing.T) {
	const domainType = "dt-glossary"
	levels := []assignmentLevel{
		// level 0 — Acronym: no assignment of its own.
		{raws: nil},
		// level 1 — Business Term: the first ancestor with an assignment.
		{raws: []rawScopedAssignment{{
			ID:          "asgn-bt",
			DomainTypes: []rawAssignmentResourceRef{{ID: domainType, Name: "Glossary"}},
			AssignedCharacteristicTypeReferences: []rawAssignedCharacteristicTypeReference{
				attrRef("def", "Definition", 1),
			},
		}}},
	}

	got, err := selectScopedAssignment(levels, domainType, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.AssignmentID != "asgn-bt" {
		t.Errorf("expected the first assigned ancestor to govern, got %q", got.AssignmentID)
	}
	def, ok := attrByID(got.Attributes, "def")
	if !ok {
		t.Fatalf("ancestor Definition must be used, got %+v", got.Attributes)
	}
	if def.FromOwnAssignment {
		t.Errorf("walk-up ancestor slot should have FromOwnAssignment=false")
	}
}

// The walk-up is not capped at one level: an intermediate ancestor that also
// carries no assignment is skipped, and resolution continues to the next
// ancestor that does.
func TestSelectScopedAssignment_MultiLevelWalkUp_SkipsUnassignedIntermediate(t *testing.T) {
	const domainType = "dt-glossary"
	levels := []assignmentLevel{
		{raws: nil}, // level 0 — no assignment
		{raws: nil}, // level 1 — intermediate ancestor, also no assignment
		// level 2 — first ancestor that carries an assignment.
		{raws: []rawScopedAssignment{{
			ID:          "asgn-grandparent",
			DomainTypes: []rawAssignmentResourceRef{{ID: domainType, Name: "Glossary"}},
			AssignedCharacteristicTypeReferences: []rawAssignedCharacteristicTypeReference{
				attrRef("def", "Definition", 1),
			},
		}}},
	}

	got, err := selectScopedAssignment(levels, domainType, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.AssignmentID != "asgn-grandparent" {
		t.Errorf("expected the walk to skip the unassigned intermediate and reach asgn-grandparent, got %q", got.AssignmentID)
	}
	if _, ok := attrByID(got.Attributes, "def"); !ok {
		t.Errorf("grandparent Definition must be used, got %+v", got.Attributes)
	}
}

// The selected assignment does not list the target domain type → not allowed.
// Selection itself never consults the domain type; this is the post-selection
// creatability gate.
func TestSelectScopedAssignment_SelectedAssignmentLacksDomainType_NotAllowed(t *testing.T) {
	levels := []assignmentLevel{
		{raws: []rawScopedAssignment{{
			ID:          "asgn",
			DomainTypes: []rawAssignmentResourceRef{{ID: "dt-other", Name: "Other"}},
		}}},
	}
	if _, err := selectScopedAssignment(levels, "dt-glossary", nil); err == nil {
		t.Fatal("expected 'not allowed' error when the selected assignment does not list the target domain type")
	}
}

// A scoped assignment whose scope does not cover the target domain must be
// invisible: its characteristics never reach the effective assignment. This
// is the 2026-07-20 "Pricebooks" regression — a Business Term assignment
// scoped elsewhere shared the Glossary domain type, and its required
// Pricebook attributes blocked creates in every ordinary glossary domain.
func TestSelectScopedAssignment_ScopedAssignmentOutsideScope_Ignored(t *testing.T) {
	const domainType = "dt-glossary"
	levels := []assignmentLevel{
		{raws: []rawScopedAssignment{
			{
				ID:          "asgn-global",
				DomainTypes: []rawAssignmentResourceRef{{ID: domainType, Name: "Glossary"}},
				AssignedCharacteristicTypeReferences: []rawAssignedCharacteristicTypeReference{
					attrRef("def", "Definition", 1),
				},
			},
			{
				ID:          "asgn-pricebooks",
				DomainTypes: []rawAssignmentResourceRef{{ID: domainType, Name: "Glossary"}},
				Scope: &rawAssignmentScope{
					ID: "scope-pricebooks", Name: "Pricebooks",
					Domains: []rawAssignmentResourceRef{{ID: "dom-pricebook-4"}},
				},
				AssignedCharacteristicTypeReferences: []rawAssignedCharacteristicTypeReference{
					attrRef("pb-premier", "Pricebook 4 - Premier Package", 1),
					attrRef("pb-ultimate", "Pricebook 4 - Ultimate Package", 1),
				},
			},
		}},
	}

	// No scope covers the target domain.
	got, err := selectScopedAssignment(levels, domainType, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := attrByID(got.Attributes, "def"); !ok {
		t.Errorf("global Definition must be present, got %+v", got.Attributes)
	}
	if len(got.Attributes) != 1 {
		t.Errorf("scoped Pricebook attributes must NOT be unioned in, got %+v", got.Attributes)
	}
	if got.AssignmentID != "asgn-global" {
		t.Errorf("expected the global assignment to govern, got %q", got.AssignmentID)
	}
}

// When the target domain IS covered by an assignment's scope, that scoped
// assignment replaces the global one outright — exactly one assignment
// governs a (type, domain) pair; scoped and global are never merged.
func TestSelectScopedAssignment_CoveringScopedAssignmentWinsOverGlobal(t *testing.T) {
	const domainType = "dt-glossary"
	levels := []assignmentLevel{
		{raws: []rawScopedAssignment{
			{
				ID:          "asgn-global",
				DomainTypes: []rawAssignmentResourceRef{{ID: domainType, Name: "Glossary"}},
				AssignedCharacteristicTypeReferences: []rawAssignedCharacteristicTypeReference{
					attrRef("def", "Definition", 1),
				},
			},
			{
				ID:          "asgn-pricebooks",
				DomainTypes: []rawAssignmentResourceRef{{ID: domainType, Name: "Glossary"}},
				Scope: &rawAssignmentScope{
					ID: "scope-pricebooks", Name: "Pricebooks",
					Domains: []rawAssignmentResourceRef{{ID: "dom-pricebook-4"}},
				},
				AssignedCharacteristicTypeReferences: []rawAssignedCharacteristicTypeReference{
					attrRef("pb-premier", "Pricebook 4 - Premier Package", 1),
				},
			},
		}},
	}

	covered := map[string]scopeTier{"scope-pricebooks": scopeTierDomainDirect}
	got, err := selectScopedAssignment(levels, domainType, covered)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := attrByID(got.Attributes, "pb-premier"); !ok {
		t.Errorf("covering scoped assignment's attribute must be present, got %+v", got.Attributes)
	}
	if _, ok := attrByID(got.Attributes, "def"); ok {
		t.Errorf("global Definition must NOT be merged with the covering scoped assignment, got %+v", got.Attributes)
	}
	if got.AssignmentID != "asgn-pricebooks" {
		t.Errorf("expected the scoped assignment to govern, got %q", got.AssignmentID)
	}
}

// Two scopes cover the same target domain — one domain-direct, one community.
// The domain-direct assignment must be selected whole; the community scope's
// and the global's characteristics must NOT be merged in. This is the key
// regression guard against residual over-collection.
func TestSelectScopedAssignment_TwoCoveringScopes_DomainDirectWins(t *testing.T) {
	const domainType = "dt-glossary"
	levels := []assignmentLevel{
		{raws: []rawScopedAssignment{
			{
				ID:          "asgn-global",
				DomainTypes: []rawAssignmentResourceRef{{ID: domainType, Name: "Glossary"}},
				AssignedCharacteristicTypeReferences: []rawAssignedCharacteristicTypeReference{
					attrRef("g", "Global Attr", 1),
				},
			},
			{
				ID:          "asgn-community",
				DomainTypes: []rawAssignmentResourceRef{{ID: domainType, Name: "Glossary"}},
				Scope:       &rawAssignmentScope{ID: "scope-community"},
				AssignedCharacteristicTypeReferences: []rawAssignedCharacteristicTypeReference{
					attrRef("c", "Community Attr", 1),
				},
			},
			{
				ID:          "asgn-direct",
				DomainTypes: []rawAssignmentResourceRef{{ID: domainType, Name: "Glossary"}},
				Scope:       &rawAssignmentScope{ID: "scope-direct"},
				AssignedCharacteristicTypeReferences: []rawAssignedCharacteristicTypeReference{
					attrRef("d", "Direct Attr", 1),
				},
			},
		}},
	}

	covered := map[string]scopeTier{
		"scope-direct":    scopeTierDomainDirect,
		"scope-community": scopeTierCommunity,
	}
	got, err := selectScopedAssignment(levels, domainType, covered)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.AssignmentID != "asgn-direct" {
		t.Errorf("domain-direct scope must win, got %q", got.AssignmentID)
	}
	if _, ok := attrByID(got.Attributes, "d"); !ok {
		t.Errorf("domain-direct attribute must be present, got %+v", got.Attributes)
	}
	if len(got.Attributes) != 1 {
		t.Errorf("only the domain-direct assignment's characteristics must be emitted (no union), got %+v", got.Attributes)
	}
}

// Two scopes cover the target domain in the SAME tier. A single assignment is
// selected first-found — never a union of both.
func TestSelectScopedAssignment_SameTierScopes_SingleFirstFound(t *testing.T) {
	const domainType = "dt-glossary"
	levels := []assignmentLevel{
		{raws: []rawScopedAssignment{
			{
				ID:          "asgn-global",
				DomainTypes: []rawAssignmentResourceRef{{ID: domainType, Name: "Glossary"}},
				AssignedCharacteristicTypeReferences: []rawAssignedCharacteristicTypeReference{
					attrRef("g", "Global Attr", 1),
				},
			},
			{
				ID:          "asgn-a",
				DomainTypes: []rawAssignmentResourceRef{{ID: domainType, Name: "Glossary"}},
				Scope:       &rawAssignmentScope{ID: "scope-a"},
				AssignedCharacteristicTypeReferences: []rawAssignedCharacteristicTypeReference{
					attrRef("a", "A Attr", 1),
				},
			},
			{
				ID:          "asgn-b",
				DomainTypes: []rawAssignmentResourceRef{{ID: domainType, Name: "Glossary"}},
				Scope:       &rawAssignmentScope{ID: "scope-b"},
				AssignedCharacteristicTypeReferences: []rawAssignedCharacteristicTypeReference{
					attrRef("b", "B Attr", 1),
				},
			},
		}},
	}

	covered := map[string]scopeTier{
		"scope-a": scopeTierDomainDirect,
		"scope-b": scopeTierDomainDirect,
	}
	got, err := selectScopedAssignment(levels, domainType, covered)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.AssignmentID != "asgn-a" {
		t.Errorf("same-tier tie must resolve first-found (asgn-a), got %q", got.AssignmentID)
	}
	if len(got.Attributes) != 1 {
		t.Errorf("a same-tier tie must select one assignment, not union both, got %+v", got.Attributes)
	}
}

// Characteristics a Trait applies DIRECTLY to the asset type
// (traitAssignmentInheritances) are merged into the selected assignment
// alongside its own — both attributes and relations.
func TestSelectScopedAssignment_DirectTraitCharacteristicsMerged(t *testing.T) {
	const domainType = "dt-glossary"
	levels := []assignmentLevel{{raws: []rawScopedAssignment{{
		ID:          "asgn",
		DomainTypes: []rawAssignmentResourceRef{{ID: domainType, Name: "Glossary"}},
		AssignedCharacteristicTypeReferences: []rawAssignedCharacteristicTypeReference{
			attrRef("own", "Own Attr", 1),
		},
		TraitAssignmentInheritances: []rawTraitAssignmentInheritance{{
			AssignedCharacteristicTypeReferences: []rawAssignedCharacteristicTypeReference{
				attrRef("trait-attr", "Trait Attr", 1),
				relRef("trait-rel", "TO_TARGET"),
			},
		}},
	}}}}

	got, err := selectScopedAssignment(levels, domainType, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := attrByID(got.Attributes, "own"); !ok {
		t.Errorf("own attribute must be present, got %+v", got.Attributes)
	}
	if _, ok := attrByID(got.Attributes, "trait-attr"); !ok {
		t.Errorf("directly-applied Trait attribute must be merged in, got %+v", got.Attributes)
	}
	if ids := relTypeIDs(got.Relations); len(ids) != 1 || ids[0] != "trait-rel" {
		t.Errorf("directly-applied Trait relation must be merged in, got %+v", ids)
	}
}

// Characteristics a Trait applies to an ANCESTOR asset type
// (assignmentInheritances) sit under the entry's nested
// traitAssignmentInheritances and must be merged in too.
func TestSelectScopedAssignment_AncestorTraitCharacteristicsMerged(t *testing.T) {
	const domainType = "dt-glossary"
	levels := []assignmentLevel{{raws: []rawScopedAssignment{{
		ID:          "asgn",
		DomainTypes: []rawAssignmentResourceRef{{ID: domainType, Name: "Glossary"}},
		AssignedCharacteristicTypeReferences: []rawAssignedCharacteristicTypeReference{
			attrRef("own", "Own Attr", 1),
		},
		AssignmentInheritances: []rawAssignmentInheritance{{
			TraitAssignmentInheritances: []rawTraitAssignmentInheritance{{
				AssignedCharacteristicTypeReferences: []rawAssignedCharacteristicTypeReference{
					attrRef("ancestor-attr", "Ancestor Trait Attr", 1),
					relRef("ancestor-rel", "TO_TARGET"),
				},
			}},
		}},
	}}}}

	got, err := selectScopedAssignment(levels, domainType, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := attrByID(got.Attributes, "ancestor-attr"); !ok {
		t.Errorf("ancestor Trait attribute must be merged in, got %+v", got.Attributes)
	}
	if ids := relTypeIDs(got.Relations); len(ids) != 1 || ids[0] != "ancestor-rel" {
		t.Errorf("ancestor Trait relation must be merged in, got %+v", ids)
	}
}

// Closest wins: a characteristic on the selected assignment's own references
// shadows the same characteristic (same resource id) inherited from a direct
// Trait — one entry, carrying the own assignment's values.
func TestSelectScopedAssignment_OwnShadowsDirectTrait(t *testing.T) {
	const domainType = "dt-glossary"
	levels := []assignmentLevel{{raws: []rawScopedAssignment{{
		ID:          "asgn",
		DomainTypes: []rawAssignmentResourceRef{{ID: domainType, Name: "Glossary"}},
		AssignedCharacteristicTypeReferences: []rawAssignedCharacteristicTypeReference{
			attrRef("shared", "Shared Attr", 2), // own: min 2
		},
		TraitAssignmentInheritances: []rawTraitAssignmentInheritance{{
			AssignedCharacteristicTypeReferences: []rawAssignedCharacteristicTypeReference{
				attrRef("shared", "Shared Attr", 7), // direct trait: min 7 — shadowed
			},
		}},
	}}}}

	got, err := selectScopedAssignment(levels, domainType, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	shared, ok := attrByID(got.Attributes, "shared")
	if !ok {
		t.Fatalf("shared attribute must be present, got %+v", got.Attributes)
	}
	if len(got.Attributes) != 1 {
		t.Errorf("shadowed duplicate must be dropped, got %d slots", len(got.Attributes))
	}
	if shared.Min != 2 {
		t.Errorf("own assignment must win the whole characteristic (Min=2), got Min=%d", shared.Min)
	}
}

// Closest wins: a characteristic on a directly-applied Trait shadows the same
// one inherited from an ancestor Trait.
func TestSelectScopedAssignment_DirectTraitShadowsAncestorTrait(t *testing.T) {
	const domainType = "dt-glossary"
	levels := []assignmentLevel{{raws: []rawScopedAssignment{{
		ID:          "asgn",
		DomainTypes: []rawAssignmentResourceRef{{ID: domainType, Name: "Glossary"}},
		TraitAssignmentInheritances: []rawTraitAssignmentInheritance{{
			AssignedCharacteristicTypeReferences: []rawAssignedCharacteristicTypeReference{
				attrRef("shared", "Shared Attr", 3), // direct trait: min 3 — wins
			},
		}},
		AssignmentInheritances: []rawAssignmentInheritance{{
			TraitAssignmentInheritances: []rawTraitAssignmentInheritance{{
				AssignedCharacteristicTypeReferences: []rawAssignedCharacteristicTypeReference{
					attrRef("shared", "Shared Attr", 9), // ancestor trait: min 9 — shadowed
				},
			}},
		}},
	}}}}

	got, err := selectScopedAssignment(levels, domainType, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	shared, ok := attrByID(got.Attributes, "shared")
	if !ok {
		t.Fatalf("shared attribute must be present, got %+v", got.Attributes)
	}
	if len(got.Attributes) != 1 {
		t.Errorf("shadowed duplicate must be dropped, got %d slots", len(got.Attributes))
	}
	if shared.Min != 3 {
		t.Errorf("direct Trait must win over ancestor Trait (Min=3), got Min=%d", shared.Min)
	}
}

// A relation type assigned in both directions is two distinct characteristics
// (same resource id, different roleDirection) and both must survive — the
// direction-aware dedup key replaces the old resource-id-only dedup that dropped
// the second direction.
func TestSelectScopedAssignment_BidirectionalRelation_BothDirectionsKept(t *testing.T) {
	const domainType = "dt-glossary"
	levels := []assignmentLevel{{raws: []rawScopedAssignment{{
		ID:          "asgn",
		DomainTypes: []rawAssignmentResourceRef{{ID: domainType, Name: "Glossary"}},
		AssignedCharacteristicTypeReferences: []rawAssignedCharacteristicTypeReference{
			relRef("groups", "TO_TARGET"),
			relRef("groups", "TO_SOURCE"),
		},
	}}}}

	got, err := selectScopedAssignment(levels, domainType, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ids := relTypeIDs(got.Relations); len(ids) != 2 || ids[0] != "groups" || ids[1] != "groups" {
		t.Errorf("both directions of the bidirectional relation must be kept, got %+v", ids)
	}
}

// Relation role / co-role / direction / target are joined from characteristicTypes
// on the reference's own top-level LINE id (not the resource id), while the emitted
// RelationTypeID stays the resource id. No prior pure-selection fixture set
// characteristicTypes, so this guards the join-key fix.
func TestSelectScopedAssignment_RelationMetadataJoinedByLineID(t *testing.T) {
	const domainType = "dt-glossary"
	levels := []assignmentLevel{{raws: []rawScopedAssignment{{
		ID:          "asgn",
		DomainTypes: []rawAssignmentResourceRef{{ID: domainType, Name: "Glossary"}},
		AssignedCharacteristicTypeReferences: []rawAssignedCharacteristicTypeReference{{
			ID: "line-1", // the assignment-characteristic LINE id — the join key
			AssignedResourceReference: rawAssignmentResourceRef{
				ID:           "rel-resource-1", // the relation-type RESOURCE id — the emitted RelationTypeID
				ResourceType: "RelationType", ResourceDiscriminator: "RelationType",
			},
			RoleDirection: "TO_TARGET",
		}},
		CharacteristicTypes: []rawAssignmentCharacteristicTypeMetadata{{
			ID:         "line-1", // shares the reference's line id — NOT the resource id
			Role:       "is grouped by",
			CoRole:     "groups",
			Direction:  "SOURCE_TO_TARGET",
			TargetType: &rawAssignmentResourceRef{ID: "at-target", Name: "Business Term"},
		}},
	}}}}

	got, err := selectScopedAssignment(levels, domainType, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got.Relations) != 1 {
		t.Fatalf("expected exactly one relation, got %+v", got.Relations)
	}
	rel := got.Relations[0]
	if rel.RelationTypeID != "rel-resource-1" {
		t.Errorf("emitted RelationTypeID must be the resource id, got %q", rel.RelationTypeID)
	}
	if rel.Role != "is grouped by" || rel.CoRole != "groups" || rel.Direction != "SOURCE_TO_TARGET" {
		t.Errorf("relation metadata must be joined via the line id, got %+v", rel)
	}
	if rel.TargetType == nil || rel.TargetType.ID != "at-target" {
		t.Errorf("relation target must be joined via the line id, got %+v", rel.TargetType)
	}
}

// The platform-bug discriminator "DateTimeAttributeType" is normalized to
// "DateAttributeType" (what the frozen deprecated resourceType enum collapses it
// to) and never surfaced verbatim. This is the only such normalization.
func TestSelectScopedAssignment_DateTimeAttributeType_NormalizedToDate(t *testing.T) {
	const domainType = "dt-glossary"
	levels := []assignmentLevel{{raws: []rawScopedAssignment{{
		ID:          "asgn",
		DomainTypes: []rawAssignmentResourceRef{{ID: domainType, Name: "Glossary"}},
		AssignedCharacteristicTypeReferences: []rawAssignedCharacteristicTypeReference{{
			ID: "ref-dt",
			AssignedResourceReference: rawAssignmentResourceRef{
				ID: "attr-dt", Name: "Effective Date",
				ResourceType: "DateAttributeType", ResourceDiscriminator: "DateTimeAttributeType",
			},
			MinimumOccurrences: 1,
		}},
	}}}}

	got, err := selectScopedAssignment(levels, domainType, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	attr, ok := attrByID(got.Attributes, "attr-dt")
	if !ok {
		t.Fatalf("date attribute must be present, got %+v", got.Attributes)
	}
	if attr.Kind != "DateAttributeType" {
		t.Errorf("DateTimeAttributeType must be normalized to DateAttributeType, got %q", attr.Kind)
	}
}

// Derived relation types are non-creatable and must never be offered. An explicit
// guard skips reference discriminator "DerivedRelationType" while an explicit
// ("RelationType") sibling is kept.
func TestSelectScopedAssignment_DerivedRelationType_Excluded(t *testing.T) {
	const domainType = "dt-glossary"
	levels := []assignmentLevel{{raws: []rawScopedAssignment{{
		ID:          "asgn",
		DomainTypes: []rawAssignmentResourceRef{{ID: domainType, Name: "Glossary"}},
		AssignedCharacteristicTypeReferences: []rawAssignedCharacteristicTypeReference{
			{
				ID: "ref-explicit",
				AssignedResourceReference: rawAssignmentResourceRef{
					ID:           "rel-explicit",
					ResourceType: "RelationType", ResourceDiscriminator: "RelationType",
				},
				RoleDirection: "TO_TARGET",
			},
			{
				ID: "ref-derived",
				AssignedResourceReference: rawAssignmentResourceRef{
					ID:           "rel-derived",
					ResourceType: "RelationType", ResourceDiscriminator: "DerivedRelationType",
				},
				RoleDirection: "TO_TARGET",
			},
			{
				// Empty discriminator: classification falls to the resourceType
				// fallback, which matches "DerivedRelationType" on suffix. The guard
				// must still skip it — otherwise the fallback reintroduces the DRT.
				ID: "ref-derived-nodisc",
				AssignedResourceReference: rawAssignmentResourceRef{
					ID:           "rel-derived-nodisc",
					ResourceType: "DerivedRelationType", ResourceDiscriminator: "",
				},
				RoleDirection: "TO_TARGET",
			},
		},
	}}}}

	got, err := selectScopedAssignment(levels, domainType, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ids := relTypeIDs(got.Relations); len(ids) != 1 || ids[0] != "rel-explicit" {
		t.Errorf("only the explicit relation type must be kept, got %+v", ids)
	}
}

// resolveCoveredScopes must recognise every way a scope can cover the target
// domain and tag each with its tier: the domain listed directly (domain-direct
// tier), and an ancestor community listed (community tier, walking domain →
// community → parent community). A scope covering only other domains stays
// uncovered.
func TestResolveCoveredScopes(t *testing.T) {
	const (
		targetDomain   = "dom-target"
		childCommunity = "c-child"
		rootCommunity  = "c-root"
	)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /rest/2.0/domains/"+targetDomain, func(w http.ResponseWriter, _ *http.Request) {
		writeTestJSON(t, w, map[string]any{
			"id": targetDomain, "community": map[string]string{"id": childCommunity},
		})
	})
	mux.HandleFunc("GET /rest/2.0/communities/"+childCommunity, func(w http.ResponseWriter, _ *http.Request) {
		writeTestJSON(t, w, map[string]any{
			"id": childCommunity, "parent": map[string]string{"id": rootCommunity},
		})
	})
	mux.HandleFunc("GET /rest/2.0/communities/"+rootCommunity, func(w http.ResponseWriter, _ *http.Request) {
		writeTestJSON(t, w, map[string]any{"id": rootCommunity})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	client := testutil.NewClient(srv)

	levels := []assignmentLevel{{raws: []rawScopedAssignment{
		{ID: "a-direct", Scope: &rawAssignmentScope{
			ID: "scope-direct", Domains: []rawAssignmentResourceRef{{ID: targetDomain}},
		}},
		{ID: "a-community", Scope: &rawAssignmentScope{
			ID: "scope-community", Communities: []rawAssignmentResourceRef{{ID: rootCommunity}},
		}},
		{ID: "a-elsewhere", Scope: &rawAssignmentScope{
			ID: "scope-elsewhere", Domains: []rawAssignmentResourceRef{{ID: "dom-other"}},
		}},
		{ID: "a-global"},
	}}}

	covered, err := resolveCoveredScopes(t.Context(), client, levels, targetDomain)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	wantTiers := map[string]scopeTier{
		"scope-direct":    scopeTierDomainDirect,
		"scope-community": scopeTierCommunity,
	}
	for id, want := range wantTiers {
		got, ok := covered[id]
		if !ok {
			t.Errorf("scope %q must cover the target domain, covered=%v", id, covered)
			continue
		}
		if got != want {
			t.Errorf("scope %q covered at tier %v, want %v", id, got, want)
		}
	}
	if _, ok := covered["scope-elsewhere"]; ok {
		t.Errorf("scope covering only other domains must NOT be covered, covered=%v", covered)
	}
}

// ListAllowedDomainTypesForAssetType stops at the first level with ANY
// assignment. When the asset type's own assignment level has assignments but
// every one lists empty domainTypes, the result is empty (creatable nowhere) —
// it must NOT fall through to a parent level that does list domain types. This
// guards the removal of the old levelHasExplicit "inherit-sentinel" walk-through.
func TestListAllowedDomainTypes_StopsAtFirstAssignedLevel_AllEmptyIsNowhere(t *testing.T) {
	const (
		childID    = "at-child"
		parentID   = "at-parent"
		domainType = "dt-glossary"
	)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /rest/2.0/assetTypes/"+childID, func(w http.ResponseWriter, _ *http.Request) {
		writeTestJSON(t, w, map[string]any{
			"id": childID, "name": "Child", "parent": map[string]string{"id": parentID, "name": "Parent"},
		})
	})
	mux.HandleFunc("GET /rest/2.0/assetTypes/"+parentID, func(w http.ResponseWriter, _ *http.Request) {
		writeTestJSON(t, w, map[string]any{"id": parentID, "name": "Parent"})
	})
	mux.HandleFunc("GET /rest/2.0/assignments/assetType/"+childID, func(w http.ResponseWriter, _ *http.Request) {
		// Own level: an assignment with EMPTY domainTypes.
		writeTestJSON(t, w, []map[string]any{{"id": "asgn-child", "domainTypes": []any{}}})
	})
	mux.HandleFunc("GET /rest/2.0/assignments/assetType/"+parentID, func(w http.ResponseWriter, _ *http.Request) {
		// Parent level DOES list a domain type — but the walk must never reach
		// it, because the child's own level already carried an assignment.
		writeTestJSON(t, w, []map[string]any{{
			"id": "asgn-parent", "domainTypes": []map[string]string{{"id": domainType, "name": "Glossary"}},
		}})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	client := testutil.NewClient(srv)

	got, err := ListAllowedDomainTypesForAssetType(t.Context(), client, childID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("all-empty own level must be creatable nowhere (empty result), got %+v", got)
	}
}

// When the located level DOES list domain types, they are returned — the
// non-empty result is the "creatable somewhere" verdict.
func TestListAllowedDomainTypes_LocatedLevelListsTypes(t *testing.T) {
	const (
		assetID    = "at-x"
		domainType = "dt-glossary"
	)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /rest/2.0/assetTypes/"+assetID, func(w http.ResponseWriter, _ *http.Request) {
		writeTestJSON(t, w, map[string]any{"id": assetID, "name": "X"})
	})
	mux.HandleFunc("GET /rest/2.0/assignments/assetType/"+assetID, func(w http.ResponseWriter, _ *http.Request) {
		writeTestJSON(t, w, []map[string]any{{
			"id": "asgn-x", "domainTypes": []map[string]string{{"id": domainType, "name": "Glossary"}},
		}})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	client := testutil.NewClient(srv)

	got, err := ListAllowedDomainTypesForAssetType(t.Context(), client, assetID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 || got[0].ID != domainType {
		t.Errorf("expected the located level's domain type, got %+v", got)
	}
}

func writeTestJSON(t *testing.T, w http.ResponseWriter, v any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		t.Errorf("encoding test response: %v", err)
	}
}
