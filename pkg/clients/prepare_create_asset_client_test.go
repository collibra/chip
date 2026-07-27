package clients

import "testing"

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
			ResourceDiscriminator: "StringAttributeType",
		},
		MinimumOccurrences: min,
	}
}

// When the asset type has its own authoritative assignment (a level whose
// domainTypes explicitly lists the target domain type), characteristics from
// deeper ancestor levels must NOT be unioned in. This is the DEV-204211
// regression: Data Product Port's parent-only Descriptive Example / Location
// attributes were leaking into the schema as required.
func TestReduceScopedAssignmentChain_OwnAssignmentWins_DropsParentExtras(t *testing.T) {
	const domainType = "dt-contract"
	chain := []assignmentChainNode{
		// level 0 — the asset type's own assignment, explicitly scoped.
		{raws: []rawScopedAssignment{{
			ID:          "asgn-own",
			DomainTypes: []rawAssignmentResourceRef{{ID: domainType, Name: "Data Contract"}},
			AssignedCharacteristicTypeReferences: []rawAssignedCharacteristicTypeReference{
				attrRef("own-1", "Own Attr", 1),
			},
		}}},
		// level 1 — a parent asset type that also lists the domain type and
		// carries extra required attributes. These must be dropped.
		{raws: []rawScopedAssignment{{
			ID:          "asgn-parent",
			DomainTypes: []rawAssignmentResourceRef{{ID: domainType, Name: "Data Contract"}},
			AssignedCharacteristicTypeReferences: []rawAssignedCharacteristicTypeReference{
				attrRef("loc", "Location", 1),
				attrRef("desc", "Descriptive Example", 1),
			},
		}}},
	}

	got, err := reduceScopedAssignmentChain(chain, domainType)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := attrByID(got.Attributes, "own-1"); !ok {
		t.Errorf("own attribute must be present, got %+v", got.Attributes)
	}
	if a, ok := attrByID(got.Attributes, "loc"); ok {
		t.Errorf("parent-only attribute Location must NOT be unioned in, got %+v", a)
	}
	if a, ok := attrByID(got.Attributes, "desc"); ok {
		t.Errorf("parent-only attribute Descriptive Example must NOT be unioned in, got %+v", a)
	}
	if len(got.Attributes) != 1 {
		t.Errorf("expected exactly the own attribute, got %d slots", len(got.Attributes))
	}
}

// A sentinel subtype (its own assignment has empty domainTypes) genuinely
// inherits its parent's assignment. The parent's characteristics must still be
// unioned in, but be marked FromOwnAssignment=false so create_asset does not
// block on the parent's required attributes (DEV-202031). The subtype's own
// characteristics stay FromOwnAssignment=true.
func TestReduceScopedAssignmentChain_SentinelInheritsParent(t *testing.T) {
	const domainType = "dt-glossary"
	chain := []assignmentChainNode{
		// level 0 — sentinel: empty domainTypes, one own attribute.
		{raws: []rawScopedAssignment{{
			ID:          "asgn-acronym",
			DomainTypes: []rawAssignmentResourceRef{},
			AssignedCharacteristicTypeReferences: []rawAssignedCharacteristicTypeReference{
				attrRef("acr-own", "Acronym Own", 0),
			},
		}}},
		// level 1 — parent explicitly lists the domain type + a required attr.
		{raws: []rawScopedAssignment{{
			ID:          "asgn-bt",
			DomainTypes: []rawAssignmentResourceRef{{ID: domainType, Name: "Glossary"}},
			AssignedCharacteristicTypeReferences: []rawAssignedCharacteristicTypeReference{
				attrRef("def", "Definition", 1),
			},
		}}},
	}

	got, err := reduceScopedAssignmentChain(chain, domainType)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	own, ok := attrByID(got.Attributes, "acr-own")
	if !ok {
		t.Fatalf("subtype's own attribute must be present, got %+v", got.Attributes)
	}
	if !own.FromOwnAssignment {
		t.Errorf("own attribute should have FromOwnAssignment=true")
	}
	def, ok := attrByID(got.Attributes, "def")
	if !ok {
		t.Fatalf("inherited Definition must be unioned in for sentinel subtype, got %+v", got.Attributes)
	}
	if def.FromOwnAssignment {
		t.Errorf("inherited Definition should have FromOwnAssignment=false so create does not block on it")
	}
	if !def.Required {
		t.Errorf("inherited Definition should still carry Required=true")
	}
}

// No chain level explicitly lists the target domain type → not allowed.
func TestReduceScopedAssignmentChain_NoExplicitDomainType_NotAllowed(t *testing.T) {
	chain := []assignmentChainNode{
		{raws: []rawScopedAssignment{{
			ID:          "asgn",
			DomainTypes: []rawAssignmentResourceRef{{ID: "dt-other", Name: "Other"}},
		}}},
	}
	if _, err := reduceScopedAssignmentChain(chain, "dt-glossary"); err == nil {
		t.Fatal("expected 'not allowed' error when no level lists the target domain type")
	}
}
