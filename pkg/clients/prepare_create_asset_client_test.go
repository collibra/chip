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

	got, err := reduceScopedAssignmentChain(chain, domainType, nil)
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

	got, err := reduceScopedAssignmentChain(chain, domainType, nil)
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
	if _, err := reduceScopedAssignmentChain(chain, "dt-glossary", nil); err == nil {
		t.Fatal("expected 'not allowed' error when no level lists the target domain type")
	}
}

// A scoped assignment whose scope does not cover the target domain must be
// invisible: its characteristics never reach the effective assignment. This
// is the 2026-07-20 "Pricebooks" regression — a Business Term assignment
// scoped elsewhere shared the Glossary domain type, and its required
// Pricebook attributes blocked creates in every ordinary glossary domain.
func TestReduceScopedAssignmentChain_ScopedAssignmentOutsideScope_Ignored(t *testing.T) {
	const domainType = "dt-glossary"
	chain := []assignmentChainNode{
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
	got, err := reduceScopedAssignmentChain(chain, domainType, nil)
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
func TestReduceScopedAssignmentChain_CoveringScopedAssignmentWinsOverGlobal(t *testing.T) {
	const domainType = "dt-glossary"
	chain := []assignmentChainNode{
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
	got, err := reduceScopedAssignmentChain(chain, domainType, covered)
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

	chain := []assignmentChainNode{{raws: []rawScopedAssignment{
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

	covered, err := resolveCoveredScopes(t.Context(), client, chain, targetDomain)
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

func writeTestJSON(t *testing.T, w http.ResponseWriter, v any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		t.Errorf("encoding test response: %v", err)
	}
}
