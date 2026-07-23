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

func writeTestJSON(t *testing.T, w http.ResponseWriter, v any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		t.Errorf("encoding test response: %v", err)
	}
}
