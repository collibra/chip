package clients

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/collibra/chip/pkg/tools/testutil"
)

// attrByName finds a resolved attribute slot by its attribute type name.
func attrByName(attrs []PrepareCreateScopedAttribute, name string) (PrepareCreateScopedAttribute, bool) {
	for _, a := range attrs {
		if a.AttributeTypeName == name {
			return a, true
		}
	}
	return PrepareCreateScopedAttribute{}, false
}

func attrCount(attrs []PrepareCreateScopedAttribute, name string) int {
	n := 0
	for _, a := range attrs {
		if a.AttributeTypeName == name {
			n++
		}
	}
	return n
}

func relsByID(rels []PrepareCreateScopedRelation, id string) []PrepareCreateScopedRelation {
	var out []PrepareCreateScopedRelation
	for _, r := range rels {
		if r.RelationTypeID == id {
			out = append(out, r)
		}
	}
	return out
}

func TestGetScopedAssignment_OwnAssignmentWins_DropsAncestorExtras(t *testing.T) {
	h := newScenarioHarness(t, "hierarchy_walk")

	got, err := h.resolveScopedAssignment("Data Product Port", "Contracts")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.AssignmentID != "06000000-0000-0000-0000-000000000001" {
		t.Errorf("expected the own assignment to govern, got %q", got.AssignmentID)
	}
	if _, ok := attrByName(got.Attributes, "Own Attr"); !ok {
		t.Errorf("own attribute must be present, got %+v", got.Attributes)
	}
	if a, ok := attrByName(got.Attributes, "Location"); ok {
		t.Errorf("ancestor-only attribute Location must NOT bleed in, got %+v", a)
	}
	if a, ok := attrByName(got.Attributes, "Descriptive Example"); ok {
		t.Errorf("ancestor-only attribute Descriptive Example must NOT bleed in, got %+v", a)
	}
	if len(got.Attributes) != 1 {
		t.Errorf("expected exactly the own attribute, got %d slots", len(got.Attributes))
	}
}

func TestGetScopedAssignment_NoOwnAssignment_WalksUpToFirstAssignedAncestor(t *testing.T) {
	h := newScenarioHarness(t, "hierarchy_walk")

	got, err := h.resolveScopedAssignment("Abbreviation", "Business Glossary")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.AssignmentID != "06000000-0000-0000-0000-000000000003" {
		t.Errorf("expected the first assigned ancestor to govern, got %q", got.AssignmentID)
	}
	if _, ok := attrByName(got.Attributes, "Definition"); !ok {
		t.Fatalf("ancestor Definition must be used, got %+v", got.Attributes)
	}
}

func TestGetScopedAssignment_MultiLevelWalkUp_SkipsUnassignedIntermediate(t *testing.T) {
	h := newScenarioHarness(t, "hierarchy_walk")

	got, err := h.resolveScopedAssignment("Acronym", "Business Glossary")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.AssignmentID != "06000000-0000-0000-0000-000000000003" {
		t.Errorf("expected the walk to skip the unassigned Abbreviation and reach Business Term's assignment, got %q", got.AssignmentID)
	}
	if _, ok := attrByName(got.Attributes, "Definition"); !ok {
		t.Errorf("grandparent Definition must be used, got %+v", got.Attributes)
	}
}

func TestGetScopedAssignment_SelectedAssignmentLacksDomainType_NotAllowed(t *testing.T) {
	h := newScenarioHarness(t, "hierarchy_walk")

	if _, err := h.resolveScopedAssignment("Policy", "Business Glossary"); err == nil {
		t.Fatal("expected 'not allowed' error when the selected assignment does not list the target domain type")
	}
}

func TestGetScopedAssignment_ScopedAssignmentOutsideScope_Ignored(t *testing.T) {
	h := newScenarioHarness(t, "scope_tiers")

	got, err := h.resolveScopedAssignment("Business Term", "Business Glossary")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := attrByName(got.Attributes, "Definition"); !ok {
		t.Errorf("global Definition must be present, got %+v", got.Attributes)
	}
	if len(got.Attributes) != 1 {
		t.Errorf("scoped attributes must NOT be unioned in, got %+v", got.Attributes)
	}
	if got.AssignmentID != "06000000-0000-0000-0000-000000000001" {
		t.Errorf("expected the global assignment to govern, got %q", got.AssignmentID)
	}
}

func TestGetScopedAssignment_CoveringScopedAssignmentWinsOverGlobal(t *testing.T) {
	h := newScenarioHarness(t, "scope_tiers")

	got, err := h.resolveScopedAssignment("Business Term", "Regional Glossary")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := attrByName(got.Attributes, "Regional Attr 1"); !ok {
		t.Errorf("covering scoped assignment's attribute must be present, got %+v", got.Attributes)
	}
	if _, ok := attrByName(got.Attributes, "Definition"); ok {
		t.Errorf("global Definition must NOT be merged with the covering scoped assignment, got %+v", got.Attributes)
	}
	if got.AssignmentID != "06000000-0000-0000-0000-000000000002" {
		t.Errorf("expected the scoped assignment to govern, got %q", got.AssignmentID)
	}
}

func TestGetScopedAssignment_TwoCoveringScopes_DomainDirectWins(t *testing.T) {
	h := newScenarioHarness(t, "scope_tiers")

	got, err := h.resolveScopedAssignment("Business Term", "Glossary A")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.AssignmentID != "06000000-0000-0000-0000-000000000003" {
		t.Errorf("domain-direct scope must win over the community scope, got %q", got.AssignmentID)
	}
	if _, ok := attrByName(got.Attributes, "Direct Attr"); !ok {
		t.Errorf("domain-direct attribute must be present, got %+v", got.Attributes)
	}
	if len(got.Attributes) != 1 {
		t.Errorf("only the domain-direct assignment's characteristics must be emitted (no union), got %+v", got.Attributes)
	}
}

func TestGetScopedAssignment_SameTierScopes_SingleFirstFound(t *testing.T) {
	h := newScenarioHarness(t, "scope_tiers")

	got, err := h.resolveScopedAssignment("Business Term", "Glossary B")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.AssignmentID != "06000000-0000-0000-0000-000000000005" {
		t.Errorf("same-tier tie must resolve first-found (Direct B1's assignment), got %q", got.AssignmentID)
	}
	if len(got.Attributes) != 1 {
		t.Errorf("a same-tier tie must select one assignment, not union both, got %+v", got.Attributes)
	}
}

func TestGetScopedAssignment_ScopeCoversViaAncestorCommunity(t *testing.T) {
	h := newScenarioHarness(t, "scope_tiers")

	got, err := h.resolveScopedAssignment("Business Term", "Target Glossary")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.AssignmentID != "06000000-0000-0000-0000-000000000007" {
		t.Errorf("scope covering via the ancestor community must win over global, got %q", got.AssignmentID)
	}
	if _, ok := attrByName(got.Attributes, "Deep Attr"); !ok {
		t.Errorf("scoped assignment's attribute must be present, got %+v", got.Attributes)
	}
	if _, ok := attrByName(got.Attributes, "Definition"); ok {
		t.Errorf("global Definition must NOT be merged in, got %+v", got.Attributes)
	}
}

func TestGetScopedAssignment_DirectTraitCharacteristicsMerged(t *testing.T) {
	h := newScenarioHarness(t, "trait_inheritance")

	got, err := h.resolveScopedAssignment("Business Term", "Business Glossary")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := attrByName(got.Attributes, "Own Attr"); !ok {
		t.Errorf("own attribute must be present, got %+v", got.Attributes)
	}
	if _, ok := attrByName(got.Attributes, "Trait Attr"); !ok {
		t.Errorf("directly-applied Trait attribute must be merged in, got %+v", got.Attributes)
	}
	if rels := relsByID(got.Relations, "08000000-0000-0000-0000-000000000001"); len(rels) != 1 {
		t.Errorf("directly-applied Trait relation must be merged in exactly once, got %+v", got.Relations)
	}
}

func TestGetScopedAssignment_AncestorTraitCharacteristicsMerged(t *testing.T) {
	h := newScenarioHarness(t, "trait_inheritance")

	got, err := h.resolveScopedAssignment("Business Term", "Business Glossary")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := attrByName(got.Attributes, "Ancestor Attr"); !ok {
		t.Errorf("ancestor Trait attribute must be merged in, got %+v", got.Attributes)
	}
	if rels := relsByID(got.Relations, "08000000-0000-0000-0000-000000000002"); len(rels) != 1 {
		t.Errorf("ancestor Trait relation must be merged in exactly once, got %+v", got.Relations)
	}
}

func TestGetScopedAssignment_OwnShadowsDirectTrait(t *testing.T) {
	h := newScenarioHarness(t, "trait_inheritance")

	got, err := h.resolveScopedAssignment("Business Term", "Business Glossary")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n := attrCount(got.Attributes, "Shared"); n != 1 {
		t.Fatalf("shadowed duplicate must be dropped, got %d 'Shared' slots", n)
	}
	shared, _ := attrByName(got.Attributes, "Shared")
	if shared.Min != 2 {
		t.Errorf("own assignment must win the whole characteristic (Min=2), got Min=%d", shared.Min)
	}
}

func TestGetScopedAssignment_DirectTraitShadowsAncestorTrait(t *testing.T) {
	h := newScenarioHarness(t, "trait_inheritance")

	got, err := h.resolveScopedAssignment("Business Term", "Business Glossary")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n := attrCount(got.Attributes, "Shared 2"); n != 1 {
		t.Fatalf("shadowed duplicate must be dropped, got %d 'Shared 2' slots", n)
	}
	shared2, _ := attrByName(got.Attributes, "Shared 2")
	if shared2.Min != 3 {
		t.Errorf("direct Trait must win over ancestor Trait (Min=3), got Min=%d", shared2.Min)
	}
}

func TestGetScopedAssignment_BidirectionalRelation_BothDirectionsKept(t *testing.T) {
	h := newScenarioHarness(t, "characteristic_emit")

	got, err := h.resolveScopedAssignment("Business Term", "Business Glossary")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	rels := relsByID(got.Relations, "08000000-0000-0000-0000-000000000002")
	if len(rels) != 2 {
		t.Fatalf("both directions of the bidirectional relation must be kept, got %+v", rels)
	}
	dirs := map[string]bool{rels[0].Direction: true, rels[1].Direction: true}
	if !dirs["TO_TARGET"] || !dirs["TO_SOURCE"] {
		t.Errorf("expected one entry per direction, got %+v", rels)
	}
}

func TestGetScopedAssignment_RelationReferenceFieldsEmitted(t *testing.T) {
	h := newScenarioHarness(t, "characteristic_emit")

	got, err := h.resolveScopedAssignment("Business Term", "Business Glossary")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	rels := relsByID(got.Relations, "08000000-0000-0000-0000-000000000001")
	if len(rels) != 1 {
		t.Fatalf("expected exactly one slot emitted under the resource id (not the line id), got %+v", got.Relations)
	}
	rel := rels[0]
	if rel.RelationTypePublicID != "TermGroupsTerm" {
		t.Errorf("relation publicId must be surfaced, got %q", rel.RelationTypePublicID)
	}
	if rel.Kind != "RelationType" {
		t.Errorf("relation kind must be the discriminator, got %q", rel.Kind)
	}
	if rel.Direction != "TO_TARGET" {
		t.Errorf("relation direction must come from the reference, got %q", rel.Direction)
	}
	if rel.Role != "" || rel.CoRole != "" || rel.TargetType != nil {
		t.Errorf("role/coRole/target live on the relation type resource and must stay empty here, got %+v", rel)
	}
}

func TestGetScopedAssignment_RelationTargetFromRestrictionOnly(t *testing.T) {
	h := newScenarioHarness(t, "characteristic_emit")

	got, err := h.resolveScopedAssignment("Business Term", "Business Glossary")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rels := relsByID(got.Relations, "08000000-0000-0000-0000-000000000003"); len(rels) != 1 || rels[0].TargetType != nil {
		t.Errorf("without a restriction the target must stay empty, got %+v", rels)
	}
	rels := relsByID(got.Relations, "08000000-0000-0000-0000-000000000004")
	if len(rels) != 1 || rels[0].TargetType == nil || rels[0].TargetType.ID != "03000000-0000-0000-0000-000000000002" {
		t.Errorf("assignment restriction must set the target type, got %+v", rels)
	}
}

func TestGetScopedAssignment_DateTimeAttributeType_NormalizedToDate(t *testing.T) {
	h := newScenarioHarness(t, "characteristic_emit")

	got, err := h.resolveScopedAssignment("Business Term", "Business Glossary")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	attr, ok := attrByName(got.Attributes, "Effective Date")
	if !ok {
		t.Fatalf("date attribute must be present, got %+v", got.Attributes)
	}
	if attr.Kind != "DateAttributeType" {
		t.Errorf("DateTimeAttributeType must be normalized to DateAttributeType, got %q", attr.Kind)
	}
}

func TestGetScopedAssignment_DerivedRelationType_Excluded(t *testing.T) {
	h := newScenarioHarness(t, "characteristic_emit")

	got, err := h.resolveScopedAssignment("Business Term", "Business Glossary")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rels := relsByID(got.Relations, "08000000-0000-0000-0000-000000000005"); len(rels) != 0 {
		t.Errorf("derived relation type must be excluded, got %+v", rels)
	}
	if len(got.Relations) != 5 {
		t.Errorf("expected the 5 explicit relation slots only, got %+v", got.Relations)
	}
}

func TestListAllowedDomainTypes_StopsAtFirstAssignedLevel_AllEmptyIsNowhere(t *testing.T) {
	h := newScenarioHarness(t, "allowed_domain_types")

	got, err := h.listAllowedDomainTypes("Child")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("all-empty own level must be creatable nowhere (empty result), got %+v", got)
	}
}

func TestListAllowedDomainTypes_LocatedLevelListsTypes(t *testing.T) {
	h := newScenarioHarness(t, "allowed_domain_types")

	got, err := h.listAllowedDomainTypes("X")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 || got[0].ID != "04000000-0000-0000-0000-000000000001" {
		t.Errorf("expected the located level's domain type, got %+v", got)
	}
}

func TestNotAllowedMessage_CreatableNowhere(t *testing.T) {
	h := newScenarioHarness(t, "allowed_domain_types")

	got := NotAllowedMessage(t.Context(), h.client, h.assetType("Child").ID, "Child", "Business Glossary", "Glossary")
	want := `Asset type "Child" can't be created in any domain on this instance.`
	if got != want {
		t.Errorf("want %q, got %q", want, got)
	}
}

func TestNotAllowedMessage_NotHere_DoesNotLeakAllowedTypes(t *testing.T) {
	h := newScenarioHarness(t, "allowed_domain_types")

	got := NotAllowedMessage(t.Context(), h.client, h.assetType("X").ID, "X", "My Domain", "Other Domain Type")
	want := `Asset type "X" isn't allowed in domain "My Domain" (domain type "Other Domain Type"). Pick a different asset type, or a different domain.`
	if got != want {
		t.Errorf("want %q, got %q", want, got)
	}
	if strings.Contains(got, "Glossary") || strings.Contains(got, "04000000-0000-0000-0000-000000000001") {
		t.Errorf("allowed domain types must not leak into the message, got %q", got)
	}
}

func TestNotAllowedMessage_LookupErrorFallsBackToNotHere(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)
	client := testutil.NewClient(srv)

	got := NotAllowedMessage(t.Context(), client, "03000000-0000-0000-0000-000000000001", "X", "My Domain", "Other Domain Type")
	if !strings.Contains(got, "isn't allowed in domain") {
		t.Errorf("lookup failure must fall back to the not-here message, got %q", got)
	}
	if strings.Contains(got, "can't be created in any domain") {
		t.Errorf("lookup failure must not claim creatable-nowhere, got %q", got)
	}
}
