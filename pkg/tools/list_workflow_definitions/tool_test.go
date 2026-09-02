package list_workflow_definitions_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/collibra/chip/pkg/chip"
	"github.com/collibra/chip/pkg/tools/list_workflow_definitions"
	"github.com/collibra/chip/pkg/tools/testutil"
)

const assetID = "11111111-1111-1111-1111-111111111111"

// server boots an httptest server that responds to GET /rest/2.0/workflowDefinitions, capturing
// the request's raw query so tests can assert on the exact params sent — this is what catches a
// singular/plural query-param regression (assetId vs assetIds), which a decoded-struct assertion
// would miss since both would decode to the same Go value.
func server(t *testing.T, capturedQuery *url.Values, page workflowDefinitionsPage) *http.Client {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /rest/2.0/workflowDefinitions", func(w http.ResponseWriter, r *http.Request) {
		*capturedQuery = r.URL.Query()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(page)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return testutil.NewClient(srv)
}

// workflowDefinitionsPage mirrors clients.WorkflowDefinitionsPage for test fixtures without
// importing the clients package's unexported wire details.
type workflowDefinitionsPage struct {
	Total   int                  `json:"total"`
	Offset  int                  `json:"offset"`
	Limit   int                  `json:"limit"`
	Results []workflowDefinition `json:"results"`
}

type workflowDefinition struct {
	ID                       string `json:"id"`
	Name                     string `json:"name"`
	StartLabel               string `json:"startLabel,omitempty"`
	Description              string `json:"description,omitempty"`
	Enabled                  bool   `json:"enabled"`
	FormRequired             bool   `json:"formRequired"`
	BusinessItemResourceType string `json:"businessItemResourceType,omitempty"`
}

// graphqlServer mocks POST /graphql for the global=true path, which routes through
// workflowDefinitionsGlobal instead of the REST endpoint — see workflow_client.go.
func graphqlServer(t *testing.T, workflows []globalWorkflow) *http.Client {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /graphql", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{
				"api": map[string]any{
					"workflowDefinitionsGlobal": workflows,
				},
			},
		})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return testutil.NewClient(srv)
}

// globalWorkflow mirrors the GraphQL Workflow type's actual fields — no "enabled" (confirmed
// live: the schema does not expose one; every result is enabled by construction).
type globalWorkflow struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	StartLabel   string `json:"startLabel,omitempty"`
	Description  string `json:"description,omitempty"`
	FormRequired bool   `json:"formRequired"`
}

func TestListWorkflowDefinitions_HappyPath_MapsResults(t *testing.T) {
	var q url.Values
	c := server(t, &q, workflowDefinitionsPage{
		Total: 2,
		Results: []workflowDefinition{
			{ID: "wf-1", Name: "Request Access", Description: "Requests access to a dataset", Enabled: true, FormRequired: true, BusinessItemResourceType: "ASSET"},
			{ID: "wf-2", Name: "Propose Term", Enabled: true, BusinessItemResourceType: "GLOBAL"},
		},
	})

	out, err := list_workflow_definitions.NewTool(c).Handler(t.Context(), list_workflow_definitions.Input{AssetID: assetID})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Total != 2 || len(out.Results) != 2 {
		t.Fatalf("unexpected output: %+v", out)
	}
	if out.Results[0].WorkflowDefinitionID != "wf-1" || out.Results[0].Scope != "ASSET" || !out.Results[0].FormRequired {
		t.Fatalf("unexpected first result: %+v", out.Results[0])
	}
	if out.Results[1].WorkflowDefinitionID != "wf-2" || out.Results[1].Scope != "GLOBAL" || out.Results[1].FormRequired {
		t.Fatalf("unexpected second result: %+v", out.Results[1])
	}
	if out.HasMore {
		t.Fatalf("hasMore = true, want false (all results returned)")
	}
}

// TestListWorkflowDefinitions_GlobalPath_UsesGraphQLAndMapsResults is the regression test for
// routing global=true through workflowDefinitionsGlobal (GraphQL) instead of the unfiltered
// REST endpoint. Scope on every mapped result must read GLOBAL even though the GraphQL shape
// carries no scope field of its own — it's set by construction (see ListGlobalWorkflowDefinitions).
//
// The fixture's id carries the real "Workflow:" GraphQL global-id prefix (confirmed live against
// a live instance) rather than a plain UUID — without stripGraphQLGlobalIDPrefix, this test would
// pass while the real workflowDefinitionId silently broke start_workflow.
func TestListWorkflowDefinitions_GlobalPath_UsesGraphQLAndMapsResults(t *testing.T) {
	c := graphqlServer(t, []globalWorkflow{
		{ID: "Workflow:697ee7bd-12ff-4cb2-bc57-03257ab2f64e", Name: "Propose New Business Term", Description: "Propose a term", FormRequired: true},
	})

	out, err := list_workflow_definitions.NewTool(c).Handler(t.Context(), list_workflow_definitions.Input{Global: chip.Ptr(true)})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Total != 1 || len(out.Results) != 1 {
		t.Fatalf("unexpected output: %+v", out)
	}
	if out.Results[0].WorkflowDefinitionID != "697ee7bd-12ff-4cb2-bc57-03257ab2f64e" {
		t.Fatalf("workflowDefinitionId = %q, want the plain UUID with the \"Workflow:\" prefix stripped", out.Results[0].WorkflowDefinitionID)
	}
	if out.Results[0].Scope != "GLOBAL" || !out.Results[0].FormRequired {
		t.Fatalf("unexpected result: %+v", out.Results[0])
	}
}

// TestListWorkflowDefinitions_NegativeOffsetIsRejectedBeforeAnyCall is the regression test for a
// crash, not a mere validation nicety: on the global lane offset reaches paginate's slice
// expression, and `defs[-1:]` panics. Nothing in chip or the MCP SDK recovers from a panic in a
// tool handler, so a single call with offset=-1 would kill the whole server process and take every
// other tool down with it for the rest of the session.
func TestListWorkflowDefinitions_NegativeOffsetIsRejectedBeforeAnyCall(t *testing.T) {
	for _, tc := range []struct {
		name  string
		input list_workflow_definitions.Input
	}{
		{"global lane", list_workflow_definitions.Input{Global: chip.Ptr(true), Offset: -1}},
		{"scoped lane", list_workflow_definitions.Input{AssetID: assetID, Offset: -1}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var q url.Values
			c := server(t, &q, workflowDefinitionsPage{})

			out, err := list_workflow_definitions.NewTool(c).Handler(t.Context(), tc.input)
			if err != nil {
				t.Fatalf("unexpected transport error: %v", err)
			}
			if out.Status != list_workflow_definitions.StatusValidationError {
				t.Fatalf("status = %q, want %q (for a negative offset); message: %s", out.Status, list_workflow_definitions.StatusValidationError, out.Message)
			}
			if out.Message == "" {
				t.Errorf("a validation_error must explain itself — message was empty")
			}
			if q != nil {
				t.Fatalf("expected rejection before any network call, but the server was hit: %v", q)
			}
		})
	}
}

// TestListWorkflowDefinitions_MultipleScopesAreRejected enforces the "exactly one" rule the tool
// states in three places (its Description, the Input doc comment, and every scope field's tag).
// Before this, a plain OR meant all three could be supplied at once and every id was forwarded —
// documentation and behaviour disagreeing, which the contribution standards call a defect in
// itself (§6.2).
func TestListWorkflowDefinitions_MultipleScopesAreRejected(t *testing.T) {
	for _, tc := range []struct {
		name  string
		input list_workflow_definitions.Input
	}{
		{"asset+domain", list_workflow_definitions.Input{AssetID: assetID, DomainID: "22222222-2222-2222-2222-222222222222"}},
		{"asset+community", list_workflow_definitions.Input{AssetID: assetID, CommunityID: "33333333-3333-3333-3333-333333333333"}},
		{"all three", list_workflow_definitions.Input{AssetID: assetID, DomainID: "22222222-2222-2222-2222-222222222222", CommunityID: "33333333-3333-3333-3333-333333333333"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var q url.Values
			c := server(t, &q, workflowDefinitionsPage{})

			out, err := list_workflow_definitions.NewTool(c).Handler(t.Context(), tc.input)
			if err != nil {
				t.Fatalf("unexpected transport error: %v", err)
			}
			if out.Status != list_workflow_definitions.StatusValidationError {
				t.Fatalf("status = %q, want %q (when more than one scope is supplied); message: %s", out.Status, list_workflow_definitions.StatusValidationError, out.Message)
			}
			if out.Message == "" {
				t.Errorf("a validation_error must explain itself — message was empty")
			}
			if q != nil {
				t.Fatalf("expected rejection before any network call, but the server was hit: %v", q)
			}
		})
	}
}

// TestListWorkflowDefinitions_NegativeLimitIsRejected: limit<=0 is otherwise silently reinterpreted
// as the default page size, so -5 would quietly return 50 results rather than flagging the mistake.
func TestListWorkflowDefinitions_NegativeLimitIsRejected(t *testing.T) {
	var q url.Values
	c := server(t, &q, workflowDefinitionsPage{})

	out, err := list_workflow_definitions.NewTool(c).Handler(t.Context(), list_workflow_definitions.Input{AssetID: assetID, Limit: -5})
	if err != nil {
		t.Fatalf("unexpected transport error: %v", err)
	}
	if out.Status != list_workflow_definitions.StatusValidationError {
		t.Fatalf("status = %q, want %q (for a negative limit); message: %s", out.Status, list_workflow_definitions.StatusValidationError, out.Message)
	}
	if out.Message == "" {
		t.Errorf("a validation_error must explain itself — message was empty")
	}
	if q != nil {
		t.Fatalf("expected rejection before any network call, but the server was hit: %v", q)
	}
}

// TestListWorkflowDefinitions_GlobalPath_OffsetPastEndIsEmptyNotAPanic covers the other slice
// boundary: an offset at or beyond the result count must yield an empty page, not a panic.
func TestListWorkflowDefinitions_GlobalPath_OffsetPastEndIsEmptyNotAPanic(t *testing.T) {
	c := graphqlServer(t, []globalWorkflow{{ID: "wf-1", Name: "A"}, {ID: "wf-2", Name: "B"}})

	out, err := list_workflow_definitions.NewTool(c).Handler(t.Context(), list_workflow_definitions.Input{
		Global: chip.Ptr(true),
		Offset: 99,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out.Results) != 0 {
		t.Fatalf("results = %+v, want empty for an offset past the end", out.Results)
	}
	if out.Total != 2 {
		t.Fatalf("total = %d, want 2 (the true match count, independent of paging)", out.Total)
	}
	if out.HasMore {
		t.Fatalf("hasMore = true, want false — there is nothing beyond an offset already past the end")
	}
}

// TestListWorkflowDefinitions_HasMoreAccountsForOffset pins hasMore on a LATER page. Every other
// paging test uses offset=0, where `offset+len(results) < total` and a naive `len(results) < total`
// agree — so without this case, dropping the offset term survives, and the last page keeps
// reporting hasMore=true, sending a caller into an endless paging loop.
func TestListWorkflowDefinitions_HasMoreAccountsForOffset(t *testing.T) {
	var q url.Values
	c := server(t, &q, workflowDefinitionsPage{
		Total:   3,
		Results: []workflowDefinition{{ID: "wf-3", Name: "C", BusinessItemResourceType: "ASSET"}},
	})

	out, err := list_workflow_definitions.NewTool(c).Handler(t.Context(), list_workflow_definitions.Input{
		AssetID: assetID,
		Offset:  2,
		Limit:   1,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.HasMore {
		t.Fatalf("hasMore = true, want false: offset 2 + 1 result == total 3, so this is the last page")
	}
}

// TestListWorkflowDefinitions_GlobalPath_ClientSideFilterAndPagination covers
// nameContains/offset/limit and hasMore, all computed client-side since
// workflowDefinitionsGlobal takes no name/pagination arguments of its own.
func TestListWorkflowDefinitions_GlobalPath_ClientSideFilterAndPagination(t *testing.T) {
	c := graphqlServer(t, []globalWorkflow{
		{ID: "wf-1", Name: "Propose New Business Term"},
		{ID: "wf-2", Name: "Log Security Issue"},
		{ID: "wf-3", Name: "Propose New Code Value"},
	})

	out, err := list_workflow_definitions.NewTool(c).Handler(t.Context(), list_workflow_definitions.Input{
		Global:       chip.Ptr(true),
		NameContains: "Propose",
		Limit:        1,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Total != 2 {
		t.Fatalf("total = %d, want 2 (only the two matching nameContains)", out.Total)
	}
	if len(out.Results) != 1 || out.Results[0].WorkflowDefinitionID != "wf-1" {
		t.Fatalf("unexpected page: %+v", out.Results)
	}
	if !out.HasMore {
		t.Fatalf("hasMore = false, want true (2 matches, limit 1)")
	}
}

// TestListWorkflowDefinitions_AssetScopeUsesSingularQueryParam is the regression test for the
// exact bug that made asset-scoped discovery a silent no-op on an earlier attempt at this tool:
// the server binds the query param as "assetId" — singular — despite it being a list
// server-side. Sending "assetIds" (plural) is ignored by the server and
// returns the full unfiltered list instead of erroring, so only a query-string-level assertion
// (not a decoded-struct one) catches it.
func TestListWorkflowDefinitions_AssetScopeUsesSingularQueryParam(t *testing.T) {
	var q url.Values
	c := server(t, &q, workflowDefinitionsPage{})

	out, err := list_workflow_definitions.NewTool(c).Handler(t.Context(), list_workflow_definitions.Input{AssetID: assetID})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Status != list_workflow_definitions.StatusSuccess {
		t.Fatalf("status = %q, want %q: %s", out.Status, list_workflow_definitions.StatusSuccess, out.Message)
	}
	if got := q["assetId"]; len(got) != 1 || got[0] != assetID {
		t.Fatalf("query param %q = %v, want [%q]", "assetId", got, assetID)
	}
	if _, present := q["assetIds"]; present {
		t.Fatalf("query unexpectedly sent the plural %q param: %v", "assetIds", q)
	}
}

func TestListWorkflowDefinitions_DefaultLimitIs50(t *testing.T) {
	var q url.Values
	c := server(t, &q, workflowDefinitionsPage{})

	if _, err := list_workflow_definitions.NewTool(c).Handler(t.Context(), list_workflow_definitions.Input{AssetID: assetID}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := q.Get("limit"); got != "50" {
		t.Fatalf("limit param = %q, want %q", got, "50")
	}
}

func TestListWorkflowDefinitions_ExplicitLimitOverridesDefault(t *testing.T) {
	var q url.Values
	c := server(t, &q, workflowDefinitionsPage{})

	if _, err := list_workflow_definitions.NewTool(c).Handler(t.Context(), list_workflow_definitions.Input{AssetID: assetID, Limit: 5}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := q.Get("limit"); got != "5" {
		t.Fatalf("limit param = %q, want %q", got, "5")
	}
}

func TestListWorkflowDefinitions_HasMoreWhenTruncated(t *testing.T) {
	var q url.Values
	c := server(t, &q, workflowDefinitionsPage{
		Total:   3,
		Results: []workflowDefinition{{ID: "wf-1", Name: "A", BusinessItemResourceType: "GLOBAL"}},
	})

	out, err := list_workflow_definitions.NewTool(c).Handler(t.Context(), list_workflow_definitions.Input{AssetID: assetID, Limit: 1})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !out.HasMore {
		t.Fatalf("hasMore = false, want true (total=3, returned=1)")
	}
}

// TestListWorkflowDefinitions_NoLaneIsRejected is the regression test for the design decision
// behind requiring a lane at all: without it, this call would silently return every workflow
// definition unfiltered by the current user's authorization — exactly the guarantee the tool
// exists to provide.
func TestListWorkflowDefinitions_NoLaneIsRejected(t *testing.T) {
	var q url.Values
	c := server(t, &q, workflowDefinitionsPage{})

	out, err := list_workflow_definitions.NewTool(c).Handler(t.Context(), list_workflow_definitions.Input{})
	if err != nil {
		t.Fatalf("unexpected transport error: %v", err)
	}
	if out.Status != list_workflow_definitions.StatusValidationError {
		t.Fatalf("status = %q, want %q (when no assetId/domainId/communityId/global=true is supplied); message: %s", out.Status, list_workflow_definitions.StatusValidationError, out.Message)
	}
	if out.Message == "" {
		t.Errorf("a validation_error must explain itself — message was empty")
	}
	if q != nil {
		t.Fatalf("expected the request to be rejected before any network call, but the server was hit: %v", q)
	}
}

// TestListWorkflowDefinitions_GlobalFalseAloneIsRejected: global=false without a resource asks
// for "resource-scoped workflows, unspecified which resource" — as ill-defined as no lane at all.
func TestListWorkflowDefinitions_GlobalFalseAloneIsRejected(t *testing.T) {
	var q url.Values
	c := server(t, &q, workflowDefinitionsPage{})

	out, err := list_workflow_definitions.NewTool(c).Handler(t.Context(), list_workflow_definitions.Input{Global: chip.Ptr(false)})
	if err != nil {
		t.Fatalf("unexpected transport error: %v", err)
	}
	if out.Status != list_workflow_definitions.StatusValidationError {
		t.Fatalf("status = %q, want %q (for global=false with no resource); message: %s", out.Status, list_workflow_definitions.StatusValidationError, out.Message)
	}
	if out.Message == "" {
		t.Errorf("a validation_error must explain itself — message was empty")
	}
}

// TestListWorkflowDefinitions_AlwaysRequestsEnabledTrue is the regression test for the tool-level
// policy decision (candidate, not settled — see the package comment) that this tool never exposes
// disabled workflows in any scope, matching the ticket's "returns startable workflow definitions"
// — there is deliberately no Input field that could ask for enabled=false at all.
func TestListWorkflowDefinitions_AlwaysRequestsEnabledTrue(t *testing.T) {
	var q url.Values
	c := server(t, &q, workflowDefinitionsPage{})

	if _, err := list_workflow_definitions.NewTool(c).Handler(t.Context(), list_workflow_definitions.Input{AssetID: assetID}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := q.Get("enabled"); got != "true" {
		t.Fatalf("enabled param = %q, want %q", got, "true")
	}
}

func TestListWorkflowDefinitions_InvalidAssetIDIsError(t *testing.T) {
	var q url.Values
	c := server(t, &q, workflowDefinitionsPage{})

	out, err := list_workflow_definitions.NewTool(c).Handler(t.Context(), list_workflow_definitions.Input{AssetID: "not-a-uuid"})
	if err != nil {
		t.Fatalf("unexpected transport error: %v", err)
	}
	if out.Status != list_workflow_definitions.StatusValidationError {
		t.Fatalf("status = %q, want %q (for a malformed assetId); message: %s", out.Status, list_workflow_definitions.StatusValidationError, out.Message)
	}
	if out.Message == "" {
		t.Errorf("a validation_error must explain itself — message was empty")
	}
}

func TestListWorkflowDefinitions_AssetIDWithGlobalTrueIsRejected(t *testing.T) {
	var q url.Values
	c := server(t, &q, workflowDefinitionsPage{})

	global := true
	out, err := list_workflow_definitions.NewTool(c).Handler(t.Context(), list_workflow_definitions.Input{
		AssetID: "11111111-1111-1111-1111-111111111111",
		Global:  &global,
	})
	if err != nil {
		t.Fatalf("unexpected transport error: %v", err)
	}
	if out.Status != list_workflow_definitions.StatusValidationError {
		t.Fatalf("status = %q, want %q (for assetId combined with global=true); message: %s", out.Status, list_workflow_definitions.StatusValidationError, out.Message)
	}
	if out.Message == "" {
		t.Errorf("a validation_error must explain itself — message was empty")
	}
	if q != nil {
		t.Fatalf("expected the request to be rejected before any network call, but the server was hit: %v", q)
	}
}

func TestListWorkflowDefinitions_ReadOnlyAnnotations(t *testing.T) {
	tool := list_workflow_definitions.NewTool(&http.Client{})
	if tool.Annotations == nil || !tool.Annotations.ReadOnlyHint {
		t.Fatalf("expected ReadOnlyHint=true")
	}
	if tool.Annotations.DestructiveHint == nil || *tool.Annotations.DestructiveHint {
		t.Fatalf("expected DestructiveHint=false")
	}
	if !tool.Annotations.IdempotentHint {
		t.Fatalf("expected IdempotentHint=true")
	}
}

// TestListWorkflowDefinitions_NameMatchIsCaseInsensitiveAndCoversStartLabel pins the search
// contract on BOTH lanes at once, because they reach it by different routes: the global lane
// filters in Go, while the scoped lane deliberately stops forwarding `name` to the server (whose
// own filter is a case-SENSITIVE match on the name only) and filters here instead.
//
// Both halves matter in practice. Live, 8 of 26 global definitions have a start label that differs
// from the name — "Propose New Business Term" is labelled "Propose Business Term" — so a user who
// quotes what the product showed them would otherwise get nothing, and lowercase input would miss
// everything regardless.
func TestListWorkflowDefinitions_NameMatchIsCaseInsensitiveAndCoversStartLabel(t *testing.T) {
	const name, label = "Propose New Business Term", "Propose Business Term"

	for _, needle := range []string{
		"Propose New Business Term", // the name, exactly
		"propose new business term", // the name, wrong case
		"Propose Business Term",     // the START LABEL — absent from the name
		"business term",             // a fragment, lowercase
	} {
		t.Run(needle, func(t *testing.T) {
			t.Run("global lane", func(t *testing.T) {
				c := graphqlServer(t, []globalWorkflow{{ID: "wf-1", Name: name, StartLabel: label}})
				out, err := list_workflow_definitions.NewTool(c).Handler(t.Context(), list_workflow_definitions.Input{
					Global: chip.Ptr(true), NameContains: needle,
				})
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if len(out.Results) != 1 {
					t.Fatalf("%q matched %d results, want 1", needle, len(out.Results))
				}
			})
			t.Run("scoped lane", func(t *testing.T) {
				var q url.Values
				c := server(t, &q, workflowDefinitionsPage{
					Total:   1,
					Results: []workflowDefinition{{ID: "wf-1", Name: name, StartLabel: label, BusinessItemResourceType: "ASSET"}},
				})
				out, err := list_workflow_definitions.NewTool(c).Handler(t.Context(), list_workflow_definitions.Input{
					AssetID: assetID, NameContains: needle,
				})
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if len(out.Results) != 1 {
					t.Fatalf("%q matched %d results, want 1", needle, len(out.Results))
				}
				if q.Get("name") != "" {
					t.Errorf("name=%q was forwarded to the server, whose match is case-sensitive and name-only — that would re-narrow the set before this filter ever sees it", q.Get("name"))
				}
			})
		})
	}
}

// TestListWorkflowDefinitions_NonMatchingNameStillExcluded guards the obvious regression in the
// other direction: a case-insensitive two-field match must not turn into "matches everything".
func TestListWorkflowDefinitions_NonMatchingNameStillExcluded(t *testing.T) {
	c := graphqlServer(t, []globalWorkflow{
		{ID: "wf-1", Name: "Propose New Business Term", StartLabel: "Propose Business Term"},
		{ID: "wf-2", Name: "Escalation Process", StartLabel: "Escalate"},
	})
	out, err := list_workflow_definitions.NewTool(c).Handler(t.Context(), list_workflow_definitions.Input{
		Global: chip.Ptr(true), NameContains: "escalate",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out.Results) != 1 || out.Results[0].WorkflowDefinitionID != "wf-2" {
		t.Fatalf("expected only the Escalate workflow, got %+v", out.Results)
	}
}

// TestListWorkflowDefinitions_StartLabelOmittedWhenSameAsName: most definitions set the label to
// the name, and repeating it on every row is noise the model has to read past.
func TestListWorkflowDefinitions_StartLabelOmittedWhenSameAsName(t *testing.T) {
	c := graphqlServer(t, []globalWorkflow{
		{ID: "wf-1", Name: "Issue Creation", StartLabel: "Log Issue"},
		{ID: "wf-2", Name: "AAAtest", StartLabel: "AAAtest"},
	})
	out, err := list_workflow_definitions.NewTool(c).Handler(t.Context(), list_workflow_definitions.Input{Global: chip.Ptr(true)})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Results[0].StartLabel != "Log Issue" {
		t.Errorf("startLabel = %q, want %q — it differs from the name, so the user needs it", out.Results[0].StartLabel, "Log Issue")
	}
	if out.Results[1].StartLabel != "" {
		t.Errorf("startLabel = %q, want empty — it is identical to the name", out.Results[1].StartLabel)
	}
}

// TestListWorkflowDefinitions_ScopedNameSearchFetchesEverything: once the match moves client-side,
// the server call must not be pre-truncated by the caller's own limit, or the very row being
// searched for could be paged out before the filter runs.
func TestListWorkflowDefinitions_ScopedNameSearchFetchesEverything(t *testing.T) {
	var q url.Values
	c := server(t, &q, workflowDefinitionsPage{})

	out, err := list_workflow_definitions.NewTool(c).Handler(t.Context(), list_workflow_definitions.Input{
		AssetID: assetID, NameContains: "anything", Limit: 1, Offset: 5,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Status != list_workflow_definitions.StatusSuccess {
		t.Fatalf("status = %q, want %q: %s", out.Status, list_workflow_definitions.StatusSuccess, out.Message)
	}
	if got := q.Get("limit"); got != "1000" {
		t.Errorf("limit = %q, want %q (fetch the whole authorized set, not the caller's page)", got, "1000")
	}
	if got := q.Get("offset"); got == "5" {
		t.Errorf("offset = %q, want the fetch-everything default, not the caller's own offset", got)
	}
}

// TestListWorkflowDefinitions_DownstreamErrorsMapPerStatusCode covers the typed mapping of a
// failed resource-scoped lookup. Before typed statuses these all collapsed into one opaque Go
// error, so a caller could not tell "that id does not exist" (fix the id) from "you may not read
// it" (do not retry) from "the network blipped" (retry) — §6.6.
func TestListWorkflowDefinitions_DownstreamErrorsMapPerStatusCode(t *testing.T) {
	for _, tc := range []struct {
		code       int
		wantStatus list_workflow_definitions.OutputStatus
		wantInMsg  string
	}{
		{http.StatusNotFound, list_workflow_definitions.StatusValidationError, "No asset found"},
		{http.StatusForbidden, list_workflow_definitions.StatusError, "Do not retry"},
		{http.StatusInternalServerError, list_workflow_definitions.StatusError, "500"},
	} {
		t.Run(http.StatusText(tc.code), func(t *testing.T) {
			mux := http.NewServeMux()
			mux.HandleFunc("GET /rest/2.0/workflowDefinitions", func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.code)
				_, _ = w.Write([]byte(`{"errorCode":"SOMETHING","userMessage":"nope"}`))
			})
			srv := httptest.NewServer(mux)
			defer srv.Close()

			out, err := list_workflow_definitions.NewTool(testutil.NewClient(srv)).Handler(
				t.Context(), list_workflow_definitions.Input{AssetID: assetID})
			if err != nil {
				t.Fatalf("a downstream failure must come back as a typed status, not a Go error: %v", err)
			}
			if out.Status != tc.wantStatus {
				t.Errorf("status = %q, want %q: %s", out.Status, tc.wantStatus, out.Message)
			}
			if !strings.Contains(out.Message, tc.wantInMsg) {
				t.Errorf("message %q does not mention %q — the caller needs to know which remedy applies", out.Message, tc.wantInMsg)
			}
		})
	}
}

// TestListWorkflowDefinitions_EmptyResultIsSuccessNotError: "there are none here" and "the lookup
// failed" must not look alike — an empty list is a real answer.
func TestListWorkflowDefinitions_EmptyResultIsSuccessNotError(t *testing.T) {
	var q url.Values
	c := server(t, &q, workflowDefinitionsPage{})

	out, err := list_workflow_definitions.NewTool(c).Handler(t.Context(), list_workflow_definitions.Input{AssetID: assetID})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Status != list_workflow_definitions.StatusSuccess {
		t.Fatalf("status = %q, want %q for an empty but successful lookup", out.Status, list_workflow_definitions.StatusSuccess)
	}
	if out.Message == "" || !strings.Contains(out.Message, "No startable workflow") {
		t.Errorf("message = %q, want it to say plainly that nothing was found", out.Message)
	}
}

// TestListWorkflowDefinitions_TruncatedScanIsAdmitted: with a name filter the scoped lane matches
// client-side over whatever the server returned, capped at maxServerLimit. If the server held more
// rows than that, the match ran over a prefix — reporting "Found N" would be a confident answer
// derived from a partial scan. The caller must be told.
func TestListWorkflowDefinitions_TruncatedScanIsAdmitted(t *testing.T) {
	var q url.Values
	c := server(t, &q, workflowDefinitionsPage{
		Total: 5000, // the server has far more than it handed us
		Results: []workflowDefinition{
			{ID: "wf-1", Name: "Approval A", BusinessItemResourceType: "ASSET"},
		},
	})

	out, err := list_workflow_definitions.NewTool(c).Handler(t.Context(), list_workflow_definitions.Input{
		AssetID: assetID, NameContains: "approval",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out.Message, "only the first") {
		t.Errorf("a partial scan must be admitted in the message, got %q", out.Message)
	}
}

// TestListWorkflowDefinitions_MessageAndHasMoreAgree: the prose and the flag are derived from one
// value. Previously they came from different expressions, so the last page said "page on with
// offset" while hasMore was already false — an invitation to loop forever.
func TestListWorkflowDefinitions_MessageAndHasMoreAgree(t *testing.T) {
	for _, tc := range []struct {
		name          string
		offset, limit int
	}{
		{"first page", 0, 1}, {"middle page", 1, 1}, {"last page", 2, 1}, {"past the end", 3, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := graphqlServer(t, []globalWorkflow{{ID: "a", Name: "A"}, {ID: "b", Name: "B"}, {ID: "c", Name: "C"}})
			out, err := list_workflow_definitions.NewTool(c).Handler(t.Context(), list_workflow_definitions.Input{
				Global: chip.Ptr(true), Offset: tc.offset, Limit: tc.limit,
			})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			says := strings.Contains(out.Message, "page on with offset")
			if says != out.HasMore {
				t.Errorf("hasMore=%v but message %q — prose and flag must not disagree", out.HasMore, out.Message)
			}
		})
	}
}

// TestListWorkflowDefinitions_EmptyFilterResultBlamesTheFilterNotTheResource: "no workflows for
// asset X" is a claim about the resource. When a name filter is what excluded everything, saying
// that sends the caller away believing the resource has nothing.
func TestListWorkflowDefinitions_EmptyFilterResultBlamesTheFilterNotTheResource(t *testing.T) {
	c := graphqlServer(t, []globalWorkflow{{ID: "a", Name: "Propose Term"}})

	out, err := list_workflow_definitions.NewTool(c).Handler(t.Context(), list_workflow_definitions.Input{
		Global: chip.Ptr(true), NameContains: "nothing matches this",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out.Message, "nothing matches this") || !strings.Contains(out.Message, "without nameContains") {
		t.Errorf("message must name the filter and offer the way out, got %q", out.Message)
	}
}

// TestListWorkflowDefinitions_NameNeedleIsTrimmed: an LLM copying a quoted label routinely brings
// surrounding whitespace with it. Every other id input is trimmed; this one was not.
func TestListWorkflowDefinitions_NameNeedleIsTrimmed(t *testing.T) {
	c := graphqlServer(t, []globalWorkflow{{ID: "a", Name: "Approval Process"}})

	out, err := list_workflow_definitions.NewTool(c).Handler(t.Context(), list_workflow_definitions.Input{
		Global: chip.Ptr(true), NameContains: "  approval  ",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out.Results) != 1 {
		t.Errorf("a padded needle must still match, got %d results (message: %s)", len(out.Results), out.Message)
	}
}

// TestListWorkflowDefinitions_GlobalLaneErrorsAreTypedLikeTheScopedLane: both lanes of one tool
// must give the same class of answer for the same class of failure (§6.5/§6.6). The global lane
// used to collapse 403, 404 and a dead socket into one untyped string.
func TestListWorkflowDefinitions_GlobalLaneErrorsAreTypedLikeTheScopedLane(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /graphql", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"errorCode":"NO_PERMISSION"}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	out, err := list_workflow_definitions.NewTool(testutil.NewClient(srv)).Handler(
		t.Context(), list_workflow_definitions.Input{Global: chip.Ptr(true)})
	if err != nil {
		t.Fatalf("a downstream failure must be a typed status, not a Go error: %v", err)
	}
	if out.Status != list_workflow_definitions.StatusError || !strings.Contains(out.Message, "Do not retry") {
		t.Errorf("a 403 on the global lane must carry the same do-not-retry guidance as the scoped lane; got %s / %q", out.Status, out.Message)
	}
}

// TestListWorkflowDefinitions_GraphQLWithoutDataIsAnErrorNotAnEmptyList: a 200 carrying no data
// means something answered that was not this field — a gateway, or a renamed schema. Rendering it
// as "you have no startable workflows" is confidently wrong.
func TestListWorkflowDefinitions_GraphQLWithoutDataIsAnErrorNotAnEmptyList(t *testing.T) {
	for _, body := range []string{`{}`, `{"data":null}`, `{"data":{"api":null}}`, `{"message":"Unauthorized"}`} {
		t.Run(body, func(t *testing.T) {
			mux := http.NewServeMux()
			mux.HandleFunc("POST /graphql", func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(body))
			})
			srv := httptest.NewServer(mux)
			defer srv.Close()

			out, err := list_workflow_definitions.NewTool(testutil.NewClient(srv)).Handler(
				t.Context(), list_workflow_definitions.Input{Global: chip.Ptr(true)})
			if err != nil {
				t.Fatalf("unexpected transport error: %v", err)
			}
			if out.Status == list_workflow_definitions.StatusSuccess {
				t.Errorf("body %s was reported as success with %d results — a data-less 200 is a failure, not an empty catalogue", body, out.Total)
			}
		})
	}
}

// TestListWorkflowDefinitions_LimitAboveServerCapIsRejected: the tool knows the endpoint's ceiling,
// so an over-cap limit should be a structured validation_error rather than an opaque downstream 400.
func TestListWorkflowDefinitions_LimitAboveServerCapIsRejected(t *testing.T) {
	var q url.Values
	c := server(t, &q, workflowDefinitionsPage{})

	out, err := list_workflow_definitions.NewTool(c).Handler(t.Context(), list_workflow_definitions.Input{
		AssetID: assetID, Limit: 100000,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Status != list_workflow_definitions.StatusValidationError {
		t.Fatalf("status = %q, want validation_error", out.Status)
	}
	if q != nil {
		t.Errorf("rejected input must not reach the server, but it did: %v", q)
	}
}
