package start_workflow_test

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"

	"github.com/collibra/chip/pkg/chip"
	"github.com/collibra/chip/pkg/clients"
	"github.com/collibra/chip/pkg/tools/start_workflow"
	"github.com/collibra/chip/pkg/tools/testutil"
)

const (
	workflowID = "22222222-2222-2222-2222-222222222222"
	assetID    = "33333333-3333-3333-3333-333333333333"
	unknownID  = "44444444-4444-4444-4444-444444444444"
	instanceID = "55555555-5555-5555-5555-555555555555"
)

// wireDefinition mirrors clients.WorkflowDefinition's wire shape for test fixtures.
type wireDefinition struct {
	ID                          string `json:"id"`
	Name                        string `json:"name"`
	Description                 string `json:"description,omitempty"`
	Enabled                     bool   `json:"enabled"`
	FormRequired                bool   `json:"formRequired"`
	StartFormJSONModelAvailable bool   `json:"startFormJsonModelAvailable"`
	BusinessItemResourceType    string `json:"businessItemResourceType,omitempty"`
}

func newServer(t *testing.T) (*http.ServeMux, *http.Client) {
	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return mux, testutil.NewClient(srv)
}

func handleDefinition(mux *http.ServeMux, def wireDefinition) {
	mux.HandleFunc("GET /rest/2.0/workflowDefinitions/"+def.ID, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(def)
	})
}

func handleDefinitionNotFound(mux *http.ServeMux, id string) {
	mux.HandleFunc("GET /rest/2.0/workflowDefinitions/"+id, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"errorCode":"WORKFLOW_DEFINITION_NOT_FOUND","userMessage":"not found"}`))
	})
}

// handleLegacyForm serves the legacy (BPMN <formProperty>) start-form endpoint with a raw JSON
// body, so tests can construct exactly the wire shapes documented in
// pkg/clients/workflow_client.go (idAsString for enumValues, value for checkButtons/radioButtons).
func handleLegacyForm(mux *http.ServeMux, id, rawJSON string) {
	mux.HandleFunc("GET /rest/2.0/workflowDefinitions/workflowDefinition/"+id+"/startFormData", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(rawJSON))
	})
}

// handleJSONModelForm serves the GraphQL endpoint's workflowStartFormJsonModel field — itself a
// JSON-encoded STRING (a Flowable SimpleFormModel), matching the real API's double-encoding.
func handleJSONModelForm(mux *http.ServeMux, formModelJSON string) {
	mux.HandleFunc("POST /graphql", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{
				"api": map[string]any{
					"workflowStartFormJsonModel": formModelJSON,
				},
			},
		})
	})
}

func handleStart(mux *http.ServeMux, captured *clients.StartWorkflowInstanceRequest, called *bool, code int) {
	mux.HandleFunc("POST /rest/2.0/workflowInstances", func(w http.ResponseWriter, r *http.Request) {
		if called != nil {
			*called = true
		}
		if captured != nil {
			_ = json.NewDecoder(r.Body).Decode(captured)
		}
		if code != 0 && code != http.StatusCreated {
			w.WriteHeader(code)
			_, _ = w.Write([]byte(`{"errorCode":"START_WORKFLOW_NO_PERMISSION","userMessage":"not allowed"}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode([]map[string]string{{"id": instanceID}})
	})
}

func handleStartWithForm(mux *http.ServeMux, captured *clients.StartWorkflowInstanceWithFormRequest, called *bool) {
	mux.HandleFunc("POST /rest/2.0/internal/workflow/startWithForm", func(w http.ResponseWriter, r *http.Request) {
		if called != nil {
			*called = true
		}
		if captured != nil {
			_ = json.NewDecoder(r.Body).Decode(captured)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode([]map[string]string{{"id": instanceID}})
	})
}

func TestStartWorkflow_InvalidWorkflowDefinitionIDIsValidationError(t *testing.T) {
	mux, c := newServer(t)
	// Without this the test proves nothing: an empty mux answers 404, and a 404 maps to the very
	// same validation_error the assertion below looks for. The point is that nothing is CALLED.
	reached := false
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) { reached = true })
	defer func() {
		if reached {
			t.Errorf("a malformed id must be refused before any network call, but the server was reached")
		}
	}()

	out, err := start_workflow.NewTool(c).Handler(t.Context(), start_workflow.Input{WorkflowDefinitionID: "not-a-uuid"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Status != start_workflow.StatusValidationError {
		t.Fatalf("status = %q, want validation_error (%s)", out.Status, out.Message)
	}
}

func TestStartWorkflow_InvalidBusinessItemIDIsValidationError(t *testing.T) {
	mux, c := newServer(t)
	// See the sibling test: an empty mux 404s into the same status, so assert nothing is called.
	reached := false
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) { reached = true })
	defer func() {
		if reached {
			t.Errorf("a malformed businessItemId must be refused before any network call")
		}
	}()

	out, err := start_workflow.NewTool(c).Handler(t.Context(), start_workflow.Input{
		WorkflowDefinitionID: workflowID,
		BusinessItemID:       "not-a-uuid",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Status != start_workflow.StatusValidationError {
		t.Fatalf("status = %q, want validation_error (%s)", out.Status, out.Message)
	}
}

func TestStartWorkflow_UnknownWorkflowIsValidationError(t *testing.T) {
	mux, c := newServer(t)
	handleDefinitionNotFound(mux, unknownID)

	out, err := start_workflow.NewTool(c).Handler(t.Context(), start_workflow.Input{WorkflowDefinitionID: unknownID})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Status != start_workflow.StatusValidationError {
		t.Fatalf("status = %q, want validation_error (%s)", out.Status, out.Message)
	}
}

func TestStartWorkflow_DisabledWorkflowIsValidationError(t *testing.T) {
	mux, c := newServer(t)
	handleDefinition(mux, wireDefinition{ID: workflowID, Name: "Disabled Flow", Enabled: false, BusinessItemResourceType: "GLOBAL"})

	out, err := start_workflow.NewTool(c).Handler(t.Context(), start_workflow.Input{WorkflowDefinitionID: workflowID})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Status != start_workflow.StatusValidationError {
		t.Fatalf("status = %q, want validation_error (%s)", out.Status, out.Message)
	}
}

func TestStartWorkflow_ScopedWorkflowWithoutBusinessItemIDNeedsInput(t *testing.T) {
	mux, c := newServer(t)
	handleDefinition(mux, wireDefinition{ID: workflowID, Name: "Request Access", Enabled: true, BusinessItemResourceType: "ASSET"})

	out, err := start_workflow.NewTool(c).Handler(t.Context(), start_workflow.Input{WorkflowDefinitionID: workflowID})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Status != start_workflow.StatusNeedsInput {
		t.Fatalf("status = %q, want needs_input (%s)", out.Status, out.Message)
	}
	if out.Scope != "ASSET" {
		t.Fatalf("scope = %q, want ASSET", out.Scope)
	}
}

func TestStartWorkflow_GlobalNoFormDefaultsToPreviewAndNeverStarts(t *testing.T) {
	mux, c := newServer(t)
	handleDefinition(mux, wireDefinition{ID: workflowID, Name: "Propose Term", Enabled: true, BusinessItemResourceType: "GLOBAL"})
	var started bool
	handleStart(mux, nil, &started, http.StatusCreated)

	out, err := start_workflow.NewTool(c).Handler(t.Context(), start_workflow.Input{WorkflowDefinitionID: workflowID})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Status != start_workflow.StatusPreview {
		t.Fatalf("status = %q, want preview (%s)", out.Status, out.Message)
	}
	if started {
		t.Fatalf("start endpoint was called despite confirm=false")
	}
}

func TestStartWorkflow_LegacyFormRequired_MissingFieldReturnsNeedsInput(t *testing.T) {
	mux, c := newServer(t)
	handleDefinition(mux, wireDefinition{ID: workflowID, Name: "Log Issue", Enabled: true, FormRequired: true, BusinessItemResourceType: "GLOBAL"})
	handleLegacyForm(mux, workflowID, `{"processId":"p1","formProperties":[{"id":"reason","name":"Reason","type":"string","required":true}]}`)

	out, err := start_workflow.NewTool(c).Handler(t.Context(), start_workflow.Input{WorkflowDefinitionID: workflowID})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Status != start_workflow.StatusNeedsInput {
		t.Fatalf("status = %q, want needs_input (%s)", out.Status, out.Message)
	}
	if len(out.MissingFields) != 1 || out.MissingFields[0] != "reason" {
		t.Fatalf("missingFields = %v, want [reason]", out.MissingFields)
	}
	if len(out.FormFields) != 1 || out.FormFields[0].ID != "reason" {
		t.Fatalf("formFields = %+v", out.FormFields)
	}
}

// TestStartWorkflow_LegacyEnumField_SubmitsKeyNotLabel is the crux regression test: the legacy
// enum wire shape carries idAsString (the value Collibra actually validates on submit) alongside
// text (display only) — this asserts the value that reaches POST /workflowInstances is the key,
// not the label, end to end through the tool.
func TestStartWorkflow_LegacyEnumField_SubmitsKeyNotLabel(t *testing.T) {
	mux, c := newServer(t)
	handleDefinition(mux, wireDefinition{ID: workflowID, Name: "Log Issue", Enabled: true, FormRequired: true, BusinessItemResourceType: "GLOBAL"})
	handleLegacyForm(mux, workflowID, `{"processId":"p1","formProperties":[{"id":"choice","name":"Choice","type":"enum","required":true,"enumValues":[{"idAsString":"opt1","text":"Option One"},{"idAsString":"opt2","text":"Option Two"}]}]}`)
	var captured clients.StartWorkflowInstanceRequest
	handleStart(mux, &captured, nil, http.StatusCreated)

	out, err := start_workflow.NewTool(c).Handler(t.Context(), start_workflow.Input{
		WorkflowDefinitionID: workflowID,
		FormProperties:       map[string]string{"choice": "opt1"},
		Confirm:              true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Status != start_workflow.StatusSuccess {
		t.Fatalf("status = %q, want success (%s)", out.Status, out.Message)
	}
	if captured.FormProperties["choice"] != "opt1" {
		t.Fatalf("submitted choice = %q, want %q (the key, not the label)", captured.FormProperties["choice"], "opt1")
	}
}

func TestStartWorkflow_LegacyEnumField_InvalidValueReturnsNeedsInput(t *testing.T) {
	mux, c := newServer(t)
	handleDefinition(mux, wireDefinition{ID: workflowID, Name: "Log Issue", Enabled: true, FormRequired: true, BusinessItemResourceType: "GLOBAL"})
	handleLegacyForm(mux, workflowID, `{"processId":"p1","formProperties":[{"id":"choice","name":"Choice","type":"enum","required":true,"enumValues":[{"idAsString":"opt1","text":"Option One"}]}]}`)
	var started bool
	handleStart(mux, nil, &started, http.StatusCreated)

	out, err := start_workflow.NewTool(c).Handler(t.Context(), start_workflow.Input{
		WorkflowDefinitionID: workflowID,
		FormProperties:       map[string]string{"choice": "Option One"}, // the LABEL, not the key
		Confirm:              true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Status != start_workflow.StatusNeedsInput {
		t.Fatalf("status = %q, want needs_input (%s)", out.Status, out.Message)
	}
	if started {
		t.Fatalf("start endpoint was called despite an invalid field value")
	}
	// An invalid-but-supplied value is not "missing" — it's called out in the message instead.
	if len(out.MissingFields) != 0 {
		t.Fatalf("missingFields = %v, want none (the field was supplied, just invalid)", out.MissingFields)
	}
}

func TestStartWorkflow_JSONModelForm_UsesGraphQLAndInternalStartEndpoint(t *testing.T) {
	mux, c := newServer(t)
	handleDefinition(mux, wireDefinition{ID: workflowID, Name: "Designer Flow", Enabled: true, FormRequired: true, StartFormJSONModelAvailable: true, BusinessItemResourceType: "GLOBAL"})
	// Real FlowableFormModel shape (rows[].cols[], label/isRequired), matching
	// workflow-designer's own .form fixtures — not org.flowable's flat SimpleFormModel.
	handleJSONModelForm(mux, `{"rows":[{"cols":[{"designInfo":{"stencilId":"cloud-text"},"id":"justification","type":"text","label":"Justification","isRequired":true,"value":"{{justification}}"}]}],"metadata":{"key":"f1","modelType":"form"}}`)
	var capturedLegacy clients.StartWorkflowInstanceRequest
	var legacyCalled bool
	handleStart(mux, &capturedLegacy, &legacyCalled, http.StatusCreated)
	var captured clients.StartWorkflowInstanceWithFormRequest
	var jsonCalled bool
	handleStartWithForm(mux, &captured, &jsonCalled)

	out, err := start_workflow.NewTool(c).Handler(t.Context(), start_workflow.Input{
		WorkflowDefinitionID: workflowID,
		FormProperties:       map[string]string{"justification": "Q3 audit"},
		Confirm:              true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Status != start_workflow.StatusSuccess {
		t.Fatalf("status = %q, want success (%s)", out.Status, out.Message)
	}
	if legacyCalled {
		t.Fatalf("the legacy /workflowInstances endpoint was called for a JSON-model workflow")
	}
	if !jsonCalled {
		t.Fatalf("the internal /startWithForm endpoint was never called")
	}
	if captured.FormProperties["justification"] != "Q3 audit" {
		t.Fatalf("submitted formProperties = %v", captured.FormProperties)
	}
}

// TestStartWorkflow_JSONModelForm_RequiredFieldIsActuallyParsed is the test that fails if the
// rows[].cols[] parser is wrong. Supplying nothing must produce needs_input naming the field —
// a parser that finds zero fields would instead sail through to preview/success, which is how a
// wrong form-model shape hides itself.
func TestStartWorkflow_JSONModelForm_RequiredFieldIsActuallyParsed(t *testing.T) {
	mux, c := newServer(t)
	handleDefinition(mux, wireDefinition{ID: workflowID, Name: "Designer Flow", Enabled: true, FormRequired: true, StartFormJSONModelAvailable: true, BusinessItemResourceType: "GLOBAL"})
	handleJSONModelForm(mux, `{"rows":[{"cols":[{"designInfo":{"stencilId":"cloud-text"},"id":"justification","type":"text","label":"Justification","isRequired":true,"value":"{{justification}}"}]}]}`)
	var started bool
	handleStartWithForm(mux, nil, &started)

	out, err := start_workflow.NewTool(c).Handler(t.Context(), start_workflow.Input{
		WorkflowDefinitionID: workflowID,
		Confirm:              true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Status != start_workflow.StatusNeedsInput {
		t.Fatalf("status = %q, want needs_input — the required JSON-model field was not parsed (%s)", out.Status, out.Message)
	}
	if len(out.MissingFields) != 1 || out.MissingFields[0] != "justification" {
		t.Fatalf("missingFields = %v, want [justification]", out.MissingFields)
	}
	if len(out.FormFields) != 1 || out.FormFields[0].Name != "Justification" {
		t.Fatalf("formFields = %+v, want the label read from \"label\" (not \"name\")", out.FormFields)
	}
	if started {
		t.Fatalf("start endpoint was called despite a missing required field")
	}
}

// TestStartWorkflow_JSONModelForm_CollibraStencilIsResourcePicker covers the JSON model's own
// resource pickers — identified by the collibra- designInfo.stencilId prefix, since their
// serialized `type` collapses to a generic value.
func TestStartWorkflow_JSONModelForm_CollibraStencilIsResourcePicker(t *testing.T) {
	mux, c := newServer(t)
	handleDefinition(mux, wireDefinition{ID: workflowID, Name: "Designer Flow", Enabled: true, FormRequired: true, StartFormJSONModelAvailable: true, BusinessItemResourceType: "GLOBAL"})
	handleJSONModelForm(mux, `{"rows":[{"cols":[{"designInfo":{"stencilId":"collibra-user"},"id":"approver","type":"text","label":"Approver","isRequired":true,"value":"{{approver}}"}]}]}`)

	out, err := start_workflow.NewTool(c).Handler(t.Context(), start_workflow.Input{WorkflowDefinitionID: workflowID})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out.FormFields) != 1 || !out.FormFields[0].ResourcePicker {
		t.Fatalf("expected the collibra-user field to be flagged resourcePicker: %+v", out.FormFields)
	}
}

// TestStartWorkflow_JSONModelForm_NestedContainerFieldsAreFound covers layout containers, whose
// children would otherwise be invisible — a form built with a Panel or Subform wrapper is the
// common case in Workflow Designer, not an edge case.
//
// The fixtures use the shapes the platform actually serializes. An earlier version of this test
// hand-wrote `{"type":"panel","rows":[…]}` — a shape nothing ever emits — so it passed against a
// parser that found no container field at all. Both real shapes are covered: panel/subform nest
// under extraSettings.layoutDefinition.rows, while tabs and friends nest under
// extraSettings.sections[], whose entries are themselves COL-shaped rather than rows.
func TestStartWorkflow_JSONModelForm_NestedContainerFieldsAreFound(t *testing.T) {
	nestedField := `{"designInfo":{"stencilId":"cloud-text"},"id":"nested","type":"text","label":"Nested Field","isRequired":true,"value":"{{nested}}"}`
	for _, tc := range []struct {
		name  string
		model string
	}{
		{
			"panel via extraSettings.layoutDefinition",
			`{"rows":[{"cols":[{"id":"panel1","type":"panel","extraSettings":{"layoutDefinition":{"rows":[{"cols":[` + nestedField + `]}]}}}]}]}`,
		},
		{
			"tabs via extraSettings.sections (col-shaped entries)",
			`{"rows":[{"cols":[{"id":"tabs1","type":"tabs","extraSettings":{"sections":[{"id":"tab1","type":"panel","extraSettings":{"layoutDefinition":{"rows":[{"cols":[` + nestedField + `]}]}}}]}}]}]}`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mux, c := newServer(t)
			handleDefinition(mux, wireDefinition{ID: workflowID, Name: "Designer Flow", Enabled: true, FormRequired: true, StartFormJSONModelAvailable: true, BusinessItemResourceType: "GLOBAL"})
			handleJSONModelForm(mux, tc.model)

			out, err := start_workflow.NewTool(c).Handler(t.Context(), start_workflow.Input{WorkflowDefinitionID: workflowID})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(out.FormFields) != 1 || out.FormFields[0].ID != "nested" {
				t.Fatalf("expected the field nested inside the container to be found, got %+v", out.FormFields)
			}
			if len(out.MissingFields) != 1 || out.MissingFields[0] != "nested" {
				t.Fatalf("missingFields = %v, want [nested]", out.MissingFields)
			}
		})
	}
}

// TestStartWorkflow_JSONModelForm_StaticSelectOptionsAreOffered covers extraSettings.items, the
// statically-configured choice list on a select/radio.
func TestStartWorkflow_JSONModelForm_StaticSelectOptionsAreOffered(t *testing.T) {
	mux, c := newServer(t)
	handleDefinition(mux, wireDefinition{ID: workflowID, Name: "Designer Flow", Enabled: true, FormRequired: true, StartFormJSONModelAvailable: true, BusinessItemResourceType: "GLOBAL"})
	handleJSONModelForm(mux, `{"rows":[{"cols":[{"designInfo":{"stencilId":"cloud-single-select"},"id":"severity","type":"select","label":"Severity","isRequired":true,"value":"{{severity}}","extraSettings":{"items":[{"value":"high","label":"High"},{"value":"low","label":"Low"}]}}]}]}`)
	var started bool
	handleStartWithForm(mux, nil, &started)

	out, err := start_workflow.NewTool(c).Handler(t.Context(), start_workflow.Input{
		WorkflowDefinitionID: workflowID,
		FormProperties:       map[string]string{"severity": "High"}, // the LABEL, not the key
		Confirm:              true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Status != start_workflow.StatusNeedsInput {
		t.Fatalf("status = %q, want needs_input for a label-instead-of-key value (%s)", out.Status, out.Message)
	}
	if started {
		t.Fatalf("start endpoint was called despite an invalid option value")
	}
	if len(out.FormFields) != 1 || len(out.FormFields[0].Options) != 2 || out.FormFields[0].Options[0].Key != "high" {
		t.Fatalf("expected two options with keys high/low, got %+v", out.FormFields)
	}
}

func TestStartWorkflow_PermissionDeniedMapsToClearError(t *testing.T) {
	mux, c := newServer(t)
	handleDefinition(mux, wireDefinition{ID: workflowID, Name: "Admin Only Flow", Enabled: true, BusinessItemResourceType: "GLOBAL"})
	handleStart(mux, nil, nil, http.StatusForbidden)

	out, err := start_workflow.NewTool(c).Handler(t.Context(), start_workflow.Input{WorkflowDefinitionID: workflowID, Confirm: true})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Status != start_workflow.StatusError {
		t.Fatalf("status = %q, want error (%s)", out.Status, out.Message)
	}
	if out.WorkflowInstanceID != "" {
		t.Fatalf("workflowInstanceId set on a failed start: %q", out.WorkflowInstanceID)
	}
}

func TestStartWorkflow_ResourcePickerFieldMissing_FlagsInsteadOfGuessing(t *testing.T) {
	mux, c := newServer(t)
	handleDefinition(mux, wireDefinition{ID: workflowID, Name: "Assign Owner Flow", Enabled: true, FormRequired: true, BusinessItemResourceType: "GLOBAL"})
	handleLegacyForm(mux, workflowID, `{"processId":"p1","formProperties":[{"id":"ownerId","name":"Owner","type":"user","required":true}]}`)

	out, err := start_workflow.NewTool(c).Handler(t.Context(), start_workflow.Input{WorkflowDefinitionID: workflowID})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Status != start_workflow.StatusNeedsInput {
		t.Fatalf("status = %q, want needs_input (%s)", out.Status, out.Message)
	}
	if len(out.FormFields) != 1 || !out.FormFields[0].ResourcePicker {
		t.Fatalf("expected the 'ownerId' field to be flagged resourcePicker: %+v", out.FormFields)
	}
}

func TestStartWorkflow_WriteAnnotations(t *testing.T) {
	tool := start_workflow.NewTool(&http.Client{})
	if tool.Annotations == nil || tool.Annotations.ReadOnlyHint {
		t.Fatalf("expected ReadOnlyHint=false")
	}
	if tool.Annotations.DestructiveHint == nil || *tool.Annotations.DestructiveHint {
		t.Fatalf("expected DestructiveHint=false")
	}
}

// businessItemScoped uses the asset UUID constant so it's exercised at least once — asserts the
// business item is actually threaded through to the start request.
func TestStartWorkflow_AssetScoped_ThreadsBusinessItemIDIntoStartRequest(t *testing.T) {
	mux, c := newServer(t)
	handleDefinition(mux, wireDefinition{ID: workflowID, Name: "Request Access", Enabled: true, BusinessItemResourceType: "ASSET"})
	var captured clients.StartWorkflowInstanceRequest
	handleStart(mux, &captured, nil, http.StatusCreated)

	out, err := start_workflow.NewTool(c).Handler(t.Context(), start_workflow.Input{
		WorkflowDefinitionID: workflowID,
		BusinessItemID:       assetID,
		Confirm:              true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Status != start_workflow.StatusSuccess {
		t.Fatalf("status = %q, want success (%s)", out.Status, out.Message)
	}
	if len(captured.BusinessItemIDs) != 1 || captured.BusinessItemIDs[0] != assetID {
		t.Fatalf("businessItemIds = %v, want [%s]", captured.BusinessItemIDs, assetID)
	}
	if captured.BusinessItemType != "ASSET" {
		t.Fatalf("businessItemType = %q, want ASSET", captured.BusinessItemType)
	}
}

// --- Regression tests added after review ---

// handleStartRaw captures the start request as RAW bytes rather than decoding into the production
// request struct. Decoding into the same type the client marshals from round-trips any json-tag
// mistake perfectly, so a renamed wire key (the singular/plural class of bug this team already hit
// on the query-param side) would be invisible to an assertion made on the decoded value.
func handleStartRaw(mux *http.ServeMux, path string, body *string, called *bool, code int) {
	mux.HandleFunc("POST "+path, func(w http.ResponseWriter, r *http.Request) {
		if called != nil {
			*called = true
		}
		b, _ := io.ReadAll(r.Body)
		if body != nil {
			*body = string(b)
		}
		if code != 0 && code != http.StatusCreated {
			w.WriteHeader(code)
			_, _ = w.Write([]byte(`{"errorCode":"SOME_ERROR","userMessage":"rejected"}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode([]map[string]string{{"id": instanceID}})
	})
}

const enumFormJSON = `{"formProperties":[{"id":"priority","name":"Priority","type":"enum","required":true,
  "enumValues":[{"idAsString":"high","text":"High"},{"idAsString":"low","text":"Low"}]}]}`

// TestStartWorkflow_PaddedValueIsTrimmedForBothValidationAndWrite is the regression test for a
// confirmed defect: validation trimmed the value while the preview and the start request carried
// the raw one. "high " therefore passed the allowed-options check against "high" and was then sent
// verbatim, so Collibra rejected it as an unknown enum key — AFTER the user had approved the
// preview, defeating the confirm checkpoint. The write body is asserted raw, not via the decoded
// request struct.
func TestStartWorkflow_PaddedValueIsTrimmedForBothValidationAndWrite(t *testing.T) {
	mux, c := newServer(t)
	handleDefinition(mux, wireDefinition{ID: workflowID, Name: "Padded", Enabled: true, FormRequired: true, BusinessItemResourceType: "GLOBAL"})
	handleLegacyForm(mux, workflowID, enumFormJSON)
	var body string
	handleStartRaw(mux, "/rest/2.0/workflowInstances", &body, nil, http.StatusCreated)

	out, err := start_workflow.NewTool(c).Handler(t.Context(), start_workflow.Input{
		WorkflowDefinitionID: workflowID,
		FormProperties:       map[string]string{"priority": "  high  "},
		Confirm:              true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Status != start_workflow.StatusSuccess {
		t.Fatalf("status = %q, want %q (a paddable value must not be rejected): %s", out.Status, start_workflow.StatusSuccess, out.Message)
	}
	var sent map[string]any
	if uerr := json.Unmarshal([]byte(body), &sent); uerr != nil {
		t.Fatalf("could not parse the request body %q: %v", body, uerr)
	}
	props, _ := sent["formProperties"].(map[string]any)
	if got := props["priority"]; got != "high" {
		t.Errorf("sent priority = %q, want %q — validation and the write must agree on the exact value", got, "high")
	}
	if got := out.FormProperties["priority"]; got != "high" {
		t.Errorf("preview/success echoed priority = %q, want %q — the echo must be what was actually sent", got, "high")
	}
}

// TestStartWorkflow_WhitespaceOnlyRequiredValueIsStillMissing: trimming must not let "   " satisfy
// a required field.
func TestStartWorkflow_WhitespaceOnlyRequiredValueIsStillMissing(t *testing.T) {
	mux, c := newServer(t)
	handleDefinition(mux, wireDefinition{ID: workflowID, Name: "Padded", Enabled: true, FormRequired: true, BusinessItemResourceType: "GLOBAL"})
	handleLegacyForm(mux, workflowID, enumFormJSON)
	started := false
	handleStartRaw(mux, "/rest/2.0/workflowInstances", nil, &started, http.StatusCreated)

	out, err := start_workflow.NewTool(c).Handler(t.Context(), start_workflow.Input{
		WorkflowDefinitionID: workflowID,
		FormProperties:       map[string]string{"priority": "   "},
		Confirm:              true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Status != start_workflow.StatusNeedsInput {
		t.Fatalf("status = %q, want %q for a whitespace-only required value", out.Status, start_workflow.StatusNeedsInput)
	}
	if started {
		t.Errorf("the workflow was started despite a required field being effectively blank")
	}
}

// TestStartWorkflow_PreviewEchoesEveryFieldThatWillBeWritten enforces §5.2 structurally: every
// field the start request carries must appear in the preview as a field, not merely woven into the
// prose message. Previously workflowDefinitionId was absent entirely and businessItemId existed
// only inside Message, so a user could approve a preview without seeing the target resource as
// data. It also asserts nothing was written.
func TestStartWorkflow_PreviewEchoesEveryFieldThatWillBeWritten(t *testing.T) {
	mux, c := newServer(t)
	handleDefinition(mux, wireDefinition{ID: workflowID, Name: "Scoped", Description: "does a thing", Enabled: true, FormRequired: true, BusinessItemResourceType: "ASSET"})
	handleLegacyForm(mux, workflowID, enumFormJSON)
	started := false
	handleStartRaw(mux, "/rest/2.0/workflowInstances", nil, &started, http.StatusCreated)

	out, err := start_workflow.NewTool(c).Handler(t.Context(), start_workflow.Input{
		WorkflowDefinitionID: workflowID,
		BusinessItemID:       assetID,
		FormProperties:       map[string]string{"priority": "high"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Status != start_workflow.StatusPreview {
		t.Fatalf("status = %q, want %q: %s", out.Status, start_workflow.StatusPreview, out.Message)
	}
	if started {
		t.Fatalf("confirm=false started the workflow anyway")
	}
	if out.WorkflowDefinitionID != workflowID {
		t.Errorf("preview workflowDefinitionId = %q, want %q", out.WorkflowDefinitionID, workflowID)
	}
	if out.BusinessItemID != assetID {
		t.Errorf("preview businessItemId = %q, want %q", out.BusinessItemID, assetID)
	}
	if out.FormProperties["priority"] != "high" {
		t.Errorf("preview formProperties[priority] = %q, want %q", out.FormProperties["priority"], "high")
	}
	if out.Name == "" || out.Description == "" || out.Scope != "ASSET" {
		t.Errorf("preview is missing resolved workflow context: name=%q description=%q scope=%q", out.Name, out.Description, out.Scope)
	}
}

// TestStartWorkflow_PreviewOmitsBusinessItemForGlobalWorkflow: a GLOBAL workflow is started against
// no resource, so a businessItemId the caller supplied is NOT sent. The preview must therefore not
// claim it either — otherwise the user approves a scoping that will not happen.
func TestStartWorkflow_PreviewOmitsBusinessItemForGlobalWorkflow(t *testing.T) {
	mux, c := newServer(t)
	handleDefinition(mux, wireDefinition{ID: workflowID, Name: "Global", Enabled: true, BusinessItemResourceType: "GLOBAL"})

	out, err := start_workflow.NewTool(c).Handler(t.Context(), start_workflow.Input{
		WorkflowDefinitionID: workflowID,
		BusinessItemID:       assetID,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Status != start_workflow.StatusPreview {
		t.Fatalf("status = %q, want %q", out.Status, start_workflow.StatusPreview)
	}
	if out.BusinessItemID != "" {
		t.Errorf("preview businessItemId = %q, want empty — a GLOBAL workflow is not started against a resource, so the supplied id is unused", out.BusinessItemID)
	}
}

// TestStartWorkflow_StartErrorsMapPerStatusCode covers every branch of startError. Previously only
// 403 was exercised, and only for `status == error` — which every branch returns — so collapsing
// the whole switch into its default survived. Guidance is asserted because that is what the model
// acts on: telling it to retry a 403 or a 422 sends it into a loop it cannot win.
func TestStartWorkflow_StartErrorsMapPerStatusCode(t *testing.T) {
	for _, tc := range []struct {
		code            int
		wantInMessage   string
		wantInGuidance  string
		wantNotGuidance string
	}{
		{http.StatusForbidden, "403", "administrator", ""},
		{http.StatusNotFound, "404", "list_workflow_definitions", ""},
		{http.StatusBadRequest, "400", "formFields", ""},
		{http.StatusUnprocessableEntity, "422", "Do not retry unchanged", ""},
		{http.StatusInternalServerError, "500", "NO instance was created", ""},
	} {
		t.Run(http.StatusText(tc.code), func(t *testing.T) {
			mux, c := newServer(t)
			handleDefinition(mux, wireDefinition{ID: workflowID, Name: "Err", Enabled: true, BusinessItemResourceType: "GLOBAL"})
			handleStartRaw(mux, "/rest/2.0/workflowInstances", nil, nil, tc.code)

			out, err := start_workflow.NewTool(c).Handler(t.Context(), start_workflow.Input{
				WorkflowDefinitionID: workflowID,
				Confirm:              true,
			})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if out.Status != start_workflow.StatusError {
				t.Fatalf("status = %q, want %q", out.Status, start_workflow.StatusError)
			}
			if !strings.Contains(out.Message, tc.wantInMessage) {
				t.Errorf("message %q does not mention %q", out.Message, tc.wantInMessage)
			}
			if !strings.Contains(out.Guidance, tc.wantInGuidance) {
				t.Errorf("guidance %q does not contain %q — each status needs its own actionable advice", out.Guidance, tc.wantInGuidance)
			}
			if out.WorkflowInstanceID != "" {
				t.Errorf("workflowInstanceId = %q, want empty on failure", out.WorkflowInstanceID)
			}
		})
	}
}

// TestStartWorkflow_ForbiddenLookupDoesNotAdviseRetry: a 403 on resolving the definition used to
// fall through to the generic "Retry; if it persists, contact your administrator", which is advice
// the caller can never act on successfully (§6.3).
func TestStartWorkflow_ForbiddenLookupDoesNotAdviseRetry(t *testing.T) {
	mux, c := newServer(t)
	mux.HandleFunc("GET /rest/2.0/workflowDefinitions/"+workflowID, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"errorCode":"ACCESS_DENIED","userMessage":"nope"}`))
	})

	out, err := start_workflow.NewTool(c).Handler(t.Context(), start_workflow.Input{WorkflowDefinitionID: workflowID})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Status != start_workflow.StatusError {
		t.Fatalf("status = %q, want %q", out.Status, start_workflow.StatusError)
	}
	if !strings.Contains(out.Message, "403") || !strings.Contains(out.Guidance, "Do not retry") {
		t.Errorf("a forbidden lookup must say so and must not advise a retry; got message=%q guidance=%q", out.Message, out.Guidance)
	}
}

// TestStartWorkflow_SuccessReturnsInstanceID: the instance id is the only actionable output of the
// write, and blanking it previously survived the whole suite.
func TestStartWorkflow_SuccessReturnsInstanceID(t *testing.T) {
	mux, c := newServer(t)
	handleDefinition(mux, wireDefinition{ID: workflowID, Name: "Ok", Enabled: true, BusinessItemResourceType: "GLOBAL"})
	handleStartRaw(mux, "/rest/2.0/workflowInstances", nil, nil, http.StatusCreated)

	out, err := start_workflow.NewTool(c).Handler(t.Context(), start_workflow.Input{
		WorkflowDefinitionID: workflowID,
		Confirm:              true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Status != start_workflow.StatusSuccess {
		t.Fatalf("status = %q, want %q: %s", out.Status, start_workflow.StatusSuccess, out.Message)
	}
	if out.WorkflowInstanceID != instanceID {
		t.Errorf("workflowInstanceId = %q, want %q", out.WorkflowInstanceID, instanceID)
	}
}

// TestStartWorkflow_JSONModelForm_SubmissionKeyIsTheBoundVariable is the regression test for a
// confirmed silent-corruption defect, and its fixture is lifted verbatim from a real shipped form
// (workflows-ootb ProposeNewAssetApp/form-proposeNewAssetForm.form) rather than hand-written.
//
// A col's `id` is the designer's element id; the process variable it writes to is the {{...}} in
// its `value`, and the two routinely differ — 20 of the 35 fields in the OOTB corpus diverge.
// Submitting under the id leaves the real variables unset, and because these start events carry
// flowable:formFieldValidation="false" the start SUCCEEDS: the tool reported success while the
// proposed asset had no name and no type.
//
// The same fixture pins two more behaviours from that form. "collibra-assetType6" is only
// conditionally visible ("visible" is an expression, not true) yet marked isRequired — reporting
// it as required would deadlock the caller in needs_input demanding a value for a field the real
// form hides, so it must NOT be required here. And its label must survive as authored.
func TestStartWorkflow_JSONModelForm_SubmissionKeyIsTheBoundVariable(t *testing.T) {
	mux, c := newServer(t)
	handleDefinition(mux, wireDefinition{ID: workflowID, Name: "Propose New Asset", Enabled: true, FormRequired: true, StartFormJSONModelAvailable: true, BusinessItemResourceType: "GLOBAL"})
	handleJSONModelForm(mux, `{"rows": [{"cols": [{"designInfo": {"stencilId": "cloud-text"}, "value": "{{signifier}}", "ignore": false, "visible": true, "isRequired": true, "label": "Name", "id": "text1", "type": "text"}]}, {"cols": [{"designInfo": {"stencilId": "collibra-assetType"}, "label": "Asset Type", "value": "{{conceptType}}", "ignore": false, "visible": "{{flw.exists(intakeVocabulary)}}", "isRequired": true, "id": "collibra-assetType6", "type": "assetType"}]}]}`)
	var body string
	handleStartRaw(mux, "/rest/2.0/internal/workflow/startWithForm", &body, nil, http.StatusCreated)

	tool := start_workflow.NewTool(c)

	out, err := tool.Handler(t.Context(), start_workflow.Input{WorkflowDefinitionID: workflowID})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	ids := map[string]start_workflow.FormField{}
	for _, f := range out.FormFields {
		ids[f.ID] = f
	}
	if _, ok := ids["signifier"]; !ok {
		t.Fatalf("form fields keyed by %v, want the bound variable \"signifier\" (not the element id \"text1\")", keysOf(ids))
	}
	if _, ok := ids["conceptType"]; !ok {
		t.Fatalf("form fields keyed by %v, want the bound variable \"conceptType\" (not \"collibra-assetType6\")", keysOf(ids))
	}
	if !ids["signifier"].Required {
		t.Errorf("\"signifier\" is visible:true + isRequired:true, so it must be reported required")
	}
	// conceptType is isRequired:true with visible "{{flw.exists(intakeVocabulary)}}". It must stay
	// REQUIRED: the server does not enforce requiredness for a hidden field (its validator ignores
	// visibility, and this start event disables field validation), so reporting it optional means
	// the asset is created with no type and nothing anywhere says so. The condition is surfaced
	// instead, so the caller can judge.
	if !ids["conceptType"].Required {
		t.Errorf("\"conceptType\" is isRequired:true and must be reported required; the server will not catch it if we drop it")
	}
	if ids["conceptType"].VisibleWhen == "" {
		t.Errorf("\"conceptType\" is only conditionally visible — the condition must be surfaced so the caller can decide, got empty")
	}
	if ids["signifier"].Name != "Name" {
		t.Errorf("label = %q, want %q", ids["signifier"].Name, "Name")
	}

	// Now actually start it and assert the WIRE body, not a struct decoded with the same type the
	// client marshals from.
	out, err = tool.Handler(t.Context(), start_workflow.Input{
		WorkflowDefinitionID: workflowID,
		FormProperties:       map[string]string{"signifier": "Customer Revenue", "conceptType": "some-type-uuid"},
		Confirm:              true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Status != start_workflow.StatusSuccess {
		t.Fatalf("status = %q, want success: %s", out.Status, out.Message)
	}
	var sent map[string]any
	if uerr := json.Unmarshal([]byte(body), &sent); uerr != nil {
		t.Fatalf("bad request body %q: %v", body, uerr)
	}
	props, _ := sent["formProperties"].(map[string]any)
	if props["signifier"] != "Customer Revenue" {
		t.Errorf("sent formProperties = %v, want the value under \"signifier\" — under any other key the workflow starts with the asset name unset, and still reports success", props)
	}
}

func keysOf(m map[string]start_workflow.FormField) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// --- Legacy start-form regressions (customers still run these) ---

// TestStartWorkflow_LegacyMultiValueAcceptsCommaSeparatedKeys: a multiValue field takes several
// option keys in ONE comma-separated string. Comparing the whole string against the option list
// made "a,b" permanently invalid, with nothing in the response hinting a list was even legal — the
// caller could only loop.
func TestStartWorkflow_LegacyMultiValueAcceptsCommaSeparatedKeys(t *testing.T) {
	mux, c := newServer(t)
	handleDefinition(mux, wireDefinition{ID: workflowID, Name: "Multi", Enabled: true, FormRequired: true, BusinessItemResourceType: "GLOBAL"})
	handleLegacyForm(mux, workflowID, `{"formProperties":[{"id":"tags","name":"Tags","type":"enum","required":true,"multiValue":true,
	  "enumValues":[{"idAsString":"a","text":"Alpha"},{"idAsString":"b","text":"Beta"}]}]}`)
	var body string
	handleStartRaw(mux, "/rest/2.0/workflowInstances", &body, nil, http.StatusCreated)

	tool := start_workflow.NewTool(c)

	out, err := tool.Handler(t.Context(), start_workflow.Input{WorkflowDefinitionID: workflowID})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out.FormFields) != 1 || !out.FormFields[0].MultiValue {
		t.Fatalf("the field must be reported as multiValue so the caller knows a list is allowed: %+v", out.FormFields)
	}

	out, err = tool.Handler(t.Context(), start_workflow.Input{
		WorkflowDefinitionID: workflowID,
		FormProperties:       map[string]string{"tags": "a,b"},
		Confirm:              true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Status != start_workflow.StatusSuccess {
		t.Fatalf("a comma-separated list of valid keys was rejected: %s — %s", out.Status, out.Message)
	}

	out, _ = tool.Handler(t.Context(), start_workflow.Input{
		WorkflowDefinitionID: workflowID,
		FormProperties:       map[string]string{"tags": "a,nope"},
		Confirm:              true,
	})
	if out.Status != start_workflow.StatusNeedsInput {
		t.Errorf("an invalid entry inside the list must still be caught, got %s", out.Status)
	}
}

// TestStartWorkflow_LegacyPickerShortlistIsOfferedNotEnforced: for a resource picker the server
// sends the allowed ids in proposedDropdownValues. Dropping them stranded the caller — the field
// said "resolve a real resource" and this tool cannot resolve a role. They are a SHORTLIST though,
// not a closed set, unless proposedFixed says otherwise, so an id outside it must still pass.
func TestStartWorkflow_LegacyPickerShortlistIsOfferedNotEnforced(t *testing.T) {
	mux, c := newServer(t)
	handleDefinition(mux, wireDefinition{ID: workflowID, Name: "Picker", Enabled: true, FormRequired: true, BusinessItemResourceType: "GLOBAL"})
	handleLegacyForm(mux, workflowID, `{"formProperties":[{"id":"approverRole","name":"Approver Role","type":"role","required":true,
	  "proposedDropdownValues":[{"idAsString":"role-1","text":"Steward"},{"idAsString":"role-2","text":"Owner"}],
	  "proposedFixed":false}]}`)
	handleStartRaw(mux, "/rest/2.0/workflowInstances", nil, nil, http.StatusCreated)

	tool := start_workflow.NewTool(c)

	out, err := tool.Handler(t.Context(), start_workflow.Input{WorkflowDefinitionID: workflowID})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	f := out.FormFields[0]
	if len(f.Options) != 2 || f.Options[0].Key != "role-1" {
		t.Fatalf("the server-supplied shortlist must be offered to the caller, got %+v", f.Options)
	}
	if f.OptionsExhaustive {
		t.Errorf("proposedFixed=false means the list is a shortlist, not a closed set")
	}

	out, _ = tool.Handler(t.Context(), start_workflow.Input{
		WorkflowDefinitionID: workflowID,
		FormProperties:       map[string]string{"approverRole": "role-99"},
		Confirm:              true,
	})
	if out.Status != start_workflow.StatusSuccess {
		t.Errorf("an id outside a non-fixed shortlist is still valid and must not be refused here: %s — %s", out.Status, out.Message)
	}
}

// TestStartWorkflow_LegacyProposedFixedIsEnforced is the other half: when the server says the
// proposed values ARE the only permitted ones, a value outside them must be caught before the write.
func TestStartWorkflow_LegacyProposedFixedIsEnforced(t *testing.T) {
	mux, c := newServer(t)
	handleDefinition(mux, wireDefinition{ID: workflowID, Name: "Fixed", Enabled: true, FormRequired: true, BusinessItemResourceType: "GLOBAL"})
	handleLegacyForm(mux, workflowID, `{"formProperties":[{"id":"approverRole","name":"Approver Role","type":"role","required":true,
	  "proposedDropdownValues":[{"idAsString":"role-1","text":"Steward"}],"proposedFixed":true}]}`)
	started := false
	handleStartRaw(mux, "/rest/2.0/workflowInstances", nil, &started, http.StatusCreated)

	out, err := start_workflow.NewTool(c).Handler(t.Context(), start_workflow.Input{
		WorkflowDefinitionID: workflowID,
		FormProperties:       map[string]string{"approverRole": "role-99"},
		Confirm:              true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Status != start_workflow.StatusNeedsInput {
		t.Fatalf("status = %q, want %q for a value outside a fixed list", out.Status, start_workflow.StatusNeedsInput)
	}
	if started {
		t.Errorf("the workflow was started with a value the server would reject")
	}
}

// TestStartWorkflow_LegacyUnsupportedFieldsExplainThemselves covers the two legacy types this tool
// genuinely cannot answer. Both used to be reported as ordinary resource pickers, sending the
// caller to search_asset_keyword — which cannot upload a file and cannot resolve a role id, so it
// would search, fail, and retry indefinitely.
func TestStartWorkflow_LegacyUnsupportedFieldsExplainThemselves(t *testing.T) {
	for _, tc := range []struct{ name, fieldType, wantInMessage string }{
		{"fileUpload", "fileUpload", "uploaded file"},
		{"roleInCommunity", "roleInCommunity", "[roleId, communityId]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mux, c := newServer(t)
			handleDefinition(mux, wireDefinition{ID: workflowID, Name: "Odd", Enabled: true, FormRequired: true, BusinessItemResourceType: "GLOBAL"})
			handleLegacyForm(mux, workflowID, `{"formProperties":[{"id":"odd","name":"Odd","type":"`+tc.fieldType+`","required":true}]}`)

			out, err := start_workflow.NewTool(c).Handler(t.Context(), start_workflow.Input{WorkflowDefinitionID: workflowID})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if out.Status != start_workflow.StatusNeedsInput {
				t.Fatalf("status = %q, want %q", out.Status, start_workflow.StatusNeedsInput)
			}
			if out.FormFields[0].Unsupported == "" {
				t.Fatalf("%s must carry a reason it cannot be answered", tc.fieldType)
			}
			if !strings.Contains(out.Message, tc.wantInMessage) {
				t.Errorf("message %q does not explain the real constraint (%q)", out.Message, tc.wantInMessage)
			}
			if strings.Contains(out.Message, "search_asset_keyword") {
				t.Errorf("message sends the caller to a lookup that cannot succeed: %q", out.Message)
			}
		})
	}
}

// TestStartWorkflow_PreviewShowsOptionalFieldsNobodyAskedFor: when every field is optional the
// call goes straight to preview with nothing missing. Without the form in that response the caller
// cannot know the fields exist, confirms, and starts the workflow with everything unset — found
// live against a workflow whose four date fields are all optional.
func TestStartWorkflow_PreviewShowsOptionalFieldsNobodyAskedFor(t *testing.T) {
	mux, c := newServer(t)
	handleDefinition(mux, wireDefinition{ID: workflowID, Name: "AllOptional", Enabled: true, FormRequired: true, StartFormJSONModelAvailable: true, BusinessItemResourceType: "GLOBAL"})
	handleJSONModelForm(mux, `{"rows":[{"cols":[{"id":"date1","type":"date","label":"When","isRequired":false,"value":"{{whenever}}"}]}]}`)

	out, err := start_workflow.NewTool(c).Handler(t.Context(), start_workflow.Input{WorkflowDefinitionID: workflowID})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Status != start_workflow.StatusPreview {
		t.Fatalf("status = %q, want %q (nothing is missing, so it should preview)", out.Status, start_workflow.StatusPreview)
	}
	if len(out.FormFields) != 1 || out.FormFields[0].ID != "whenever" {
		t.Fatalf("preview must still show the optional fields that could be filled, got %+v", out.FormFields)
	}
}

// TestStartWorkflow_JSONStartFillsDeclaredDefaults is the regression test for a defect found only
// by starting a real OOTB workflow. A start form's script task reads its inputs as bare Groovy
// identifiers, so an unbound one raises MissingPropertyException and — the script being
// synchronous — kills the start with an opaque HTTP 500 naming no field. OOTB "Issue Creation"
// marks `priority` and `responsibleCommunity` optional while dereferencing both; sending the
// defaults the form declares is what makes it start, and is also what the product itself submits.
//
// The test equally pins what must NOT happen: a declared field with no default is left out rather
// than nulled. Nulling everything would also fix the start, but the same helper reused for
// completing a user task would overwrite already-set process variables with null.
func TestStartWorkflow_JSONStartFillsDeclaredDefaults(t *testing.T) {
	mux, c := newServer(t)
	handleDefinition(mux, wireDefinition{ID: workflowID, Name: "Issueish", Enabled: true, FormRequired: true, StartFormJSONModelAvailable: true, BusinessItemResourceType: "GLOBAL"})
	handleJSONModelForm(mux, `{"rows":[{"cols":[
	  {"id":"f1","type":"text","label":"Subject","isRequired":true,"value":"{{subject}}"},
	  {"id":"f2","type":"select","label":"Priority","isRequired":false,"value":"{{priority}}","defaultValue":"Normal"},
	  {"id":"f3","type":"community","label":"Community","isRequired":false,"value":"{{responsibleCommunity}}","defaultValue":"comm-1"},
	  {"id":"f4","type":"asset","label":"Related","isRequired":false,"value":"{{relatedAssets}}"}]}]}`)
	var body string
	handleStartRaw(mux, "/rest/2.0/internal/workflow/startWithForm", &body, nil, http.StatusCreated)

	out, err := start_workflow.NewTool(c).Handler(t.Context(), start_workflow.Input{
		WorkflowDefinitionID: workflowID,
		FormProperties:       map[string]string{"subject": "only this one"},
		Confirm:              true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Status != start_workflow.StatusSuccess {
		t.Fatalf("status = %q: %s", out.Status, out.Message)
	}

	var sent map[string]any
	if uerr := json.Unmarshal([]byte(body), &sent); uerr != nil {
		t.Fatalf("bad body %q: %v", body, uerr)
	}
	props, _ := sent["formProperties"].(map[string]any)
	if props["subject"] != "only this one" {
		t.Errorf("supplied value lost: %v", props["subject"])
	}
	if props["priority"] != "Normal" {
		t.Errorf("priority = %v, want the form's declared default — without it the start script sees an unbound variable", props["priority"])
	}
	if props["responsibleCommunity"] != "comm-1" {
		t.Errorf("responsibleCommunity = %v, want the form's declared default", props["responsibleCommunity"])
	}
	if _, present := props["relatedAssets"]; present {
		t.Errorf("relatedAssets has no default and was not supplied, so it must be OMITTED, not nulled — nulling is what would corrupt an already-set variable if this helper is ever reused for task completion; got %v", props["relatedAssets"])
	}
}

// TestStartWorkflow_JSONStartKeepsKeysNotOnTheForm: a caller may know a process variable the form
// does not declare. Padding the declared set must not become a filter that silently drops it.
func TestStartWorkflow_JSONStartKeepsKeysNotOnTheForm(t *testing.T) {
	mux, c := newServer(t)
	handleDefinition(mux, wireDefinition{ID: workflowID, Name: "Extra", Enabled: true, FormRequired: true, StartFormJSONModelAvailable: true, BusinessItemResourceType: "GLOBAL"})
	handleJSONModelForm(mux, `{"rows":[{"cols":[{"id":"f1","type":"text","label":"S","isRequired":false,"value":"{{subject}}"}]}]}`)
	var body string
	handleStartRaw(mux, "/rest/2.0/internal/workflow/startWithForm", &body, nil, http.StatusCreated)

	_, err := start_workflow.NewTool(c).Handler(t.Context(), start_workflow.Input{
		WorkflowDefinitionID: workflowID,
		FormProperties:       map[string]string{"notOnTheForm": "keep me"},
		Confirm:              true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var sent map[string]any
	if uerr := json.Unmarshal([]byte(body), &sent); uerr != nil {
		t.Fatalf("unparseable request body %q: %v", body, uerr)
	}
	props, _ := sent["formProperties"].(map[string]any)
	if props["notOnTheForm"] != "keep me" {
		t.Errorf("a caller-supplied key absent from the form was dropped: %v", props)
	}
}

// TestStartWorkflow_UnsuppliedFieldSubmitsTheFormsDefaultNotNull: a form field can declare a
// defaultValue, which the product pre-fills. A caller who does not mention that field means
// "leave it as it comes", so the default must be submitted — nulling it produces a different
// outcome than the same action in the UI.
//
// Real case: OOTB "Issue Creation" defaults priority to "Normal". Nulling it created issues with
// no priority at all, while the product creates them as Normal. An explicit null is still
// available to a caller who genuinely wants the field cleared — that is what an empty string does.
func TestStartWorkflow_UnsuppliedFieldSubmitsTheFormsDefaultNotNull(t *testing.T) {
	mux, c := newServer(t)
	handleDefinition(mux, wireDefinition{ID: workflowID, Name: "Defaults", Enabled: true, FormRequired: true, StartFormJSONModelAvailable: true, BusinessItemResourceType: "GLOBAL"})
	handleJSONModelForm(mux, `{"rows":[{"cols":[
	  {"id":"f1","type":"text","label":"Subject","isRequired":true,"value":"{{subject}}"},
	  {"id":"f2","type":"select","label":"Priority","isRequired":false,"value":"{{priority}}","defaultValue":"Normal"},
	  {"id":"f3","type":"text","label":"Note","isRequired":false,"value":"{{note}}"}]}]}`)
	var body string
	handleStartRaw(mux, "/rest/2.0/internal/workflow/startWithForm", &body, nil, http.StatusCreated)

	tool := start_workflow.NewTool(c)

	shown, err := tool.Handler(t.Context(), start_workflow.Input{WorkflowDefinitionID: workflowID})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var priority start_workflow.FormField
	for _, f := range shown.FormFields {
		if f.ID == "priority" {
			priority = f
		}
	}
	if priority.DefaultValue != "Normal" {
		t.Errorf("the caller must be able to see the pre-filled value, got %q", priority.DefaultValue)
	}

	if _, err := tool.Handler(t.Context(), start_workflow.Input{
		WorkflowDefinitionID: workflowID,
		FormProperties:       map[string]string{"subject": "x"},
		Confirm:              true,
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var sent map[string]any
	if uerr := json.Unmarshal([]byte(body), &sent); uerr != nil {
		t.Fatalf("unparseable request body %q: %v", body, uerr)
	}
	props, _ := sent["formProperties"].(map[string]any)
	if props["priority"] != "Normal" {
		t.Errorf("priority = %v, want the form's default %q — dropping it silently diverges from the product", props["priority"], "Normal")
	}
	if props["note"] != nil {
		t.Errorf("note = %v, want null: it has no default, and the variable must still be bound", props["note"])
	}
}

// TestStartWorkflow_ExplicitEmptyStringOverridesTheDefault: clearing a pre-filled field must
// remain possible — that is what the UI does when a user deletes the pre-filled value.
func TestStartWorkflow_ExplicitEmptyStringOverridesTheDefault(t *testing.T) {
	mux, c := newServer(t)
	handleDefinition(mux, wireDefinition{ID: workflowID, Name: "Defaults", Enabled: true, FormRequired: true, StartFormJSONModelAvailable: true, BusinessItemResourceType: "GLOBAL"})
	handleJSONModelForm(mux, `{"rows":[{"cols":[{"id":"f2","type":"select","label":"Priority","isRequired":false,"value":"{{priority}}","defaultValue":"Normal"}]}]}`)
	var body string
	handleStartRaw(mux, "/rest/2.0/internal/workflow/startWithForm", &body, nil, http.StatusCreated)

	if _, err := tool2(c).Handler(t.Context(), start_workflow.Input{
		WorkflowDefinitionID: workflowID,
		FormProperties:       map[string]string{"priority": ""},
		Confirm:              true,
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var sent map[string]any
	if uerr := json.Unmarshal([]byte(body), &sent); uerr != nil {
		t.Fatalf("unparseable request body %q: %v", body, uerr)
	}
	props, _ := sent["formProperties"].(map[string]any)
	if props["priority"] != "" {
		t.Errorf("priority = %v, want the caller's explicit empty value to win over the default", props["priority"])
	}
}

func tool2(c *http.Client) *chip.Tool[start_workflow.Input, start_workflow.Output] {
	return start_workflow.NewTool(c)
}

// TestStartWorkflow_PreviewShowsTheDefaultsItWillActuallySend closes a §5.2 gap introduced when
// form defaults started being submitted: the preview echoed only the caller's own values while the
// write additionally carried the form's defaults, so a user could approve a payload of two fields
// and have four written. Here the caller supplies one field and the form defaults two more; the
// preview must show all three, and the subsequent write must send exactly what was previewed.
func TestStartWorkflow_PreviewShowsTheDefaultsItWillActuallySend(t *testing.T) {
	mux, c := newServer(t)
	handleDefinition(mux, wireDefinition{ID: workflowID, Name: "Defaulted", Enabled: true, FormRequired: true, StartFormJSONModelAvailable: true, BusinessItemResourceType: "GLOBAL"})
	handleJSONModelForm(mux, `{"rows":[{"cols":[
	  {"id":"f1","type":"text","label":"Subject","isRequired":true,"value":"{{subject}}"},
	  {"id":"f2","type":"select","label":"Priority","isRequired":false,"value":"{{priority}}","defaultValue":"Normal"},
	  {"id":"f3","type":"community","label":"Community","isRequired":false,"value":"{{responsibleCommunity}}","defaultValue":"comm-1"}]}]}`)
	var body string
	handleStartRaw(mux, "/rest/2.0/internal/workflow/startWithForm", &body, nil, http.StatusCreated)

	tool := start_workflow.NewTool(c)
	in := start_workflow.Input{WorkflowDefinitionID: workflowID, FormProperties: map[string]string{"subject": "x"}}

	prev, err := tool.Handler(t.Context(), in)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if prev.Status != start_workflow.StatusPreview {
		t.Fatalf("status = %q: %s", prev.Status, prev.Message)
	}
	for key, want := range map[string]string{"subject": "x", "priority": "Normal", "responsibleCommunity": "comm-1"} {
		if got := prev.FormProperties[key]; got != want {
			t.Errorf("preview formProperties[%q] = %q, want %q — a value that will be written must be visible before it is approved", key, got, want)
		}
	}

	in.Confirm = true
	if _, err := tool.Handler(t.Context(), in); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var sent map[string]any
	if uerr := json.Unmarshal([]byte(body), &sent); uerr != nil {
		t.Fatalf("bad body: %v", uerr)
	}
	props, _ := sent["formProperties"].(map[string]any)
	if len(props) != len(prev.FormProperties) {
		t.Errorf("wrote %d fields but previewed %d — preview and request must not diverge: sent=%v previewed=%v", len(props), len(prev.FormProperties), props, prev.FormProperties)
	}
	for k, v := range prev.FormProperties {
		if props[k] != v {
			t.Errorf("field %q: previewed %q but sent %v", k, v, props[k])
		}
	}
}

// --- Findings from the JSON-form-model review round ---

// TestStartWorkflow_ConditionallyVisibleRequiredFieldStaysRequired: an earlier version reported a
// field with a `visible` expression as optional, reasoning the server would reject the start if it
// turned out to be needed. It does not: the engine's required-field validator ignores visibility,
// and these start events commonly set formFieldValidation="false" so no validator runs at all.
// The real OOTB "Propose New Asset" form has exactly this shape, and the effect was an asset
// created with no type and no error anywhere. Report it required; surface the condition instead.
func TestStartWorkflow_ConditionallyVisibleRequiredFieldStaysRequired(t *testing.T) {
	mux, c := newServer(t)
	handleDefinition(mux, wireDefinition{ID: workflowID, Name: "Conditional", Enabled: true, FormRequired: true, StartFormJSONModelAvailable: true, BusinessItemResourceType: "GLOBAL"})
	handleJSONModelForm(mux, `{"rows":[{"cols":[{"id":"c1","type":"assetType","label":"Asset Type","isRequired":true,"visible":"{{flw.exists(other)}}","value":"{{conceptType}}"}]}]}`)

	out, err := start_workflow.NewTool(c).Handler(t.Context(), start_workflow.Input{WorkflowDefinitionID: workflowID})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Status != start_workflow.StatusNeedsInput {
		t.Fatalf("status = %q, want needs_input — a required field must not be waved through because it is conditional", out.Status)
	}
	f := out.FormFields[0]
	if !f.Required {
		t.Errorf("field must stay required; the server will not enforce it")
	}
	if f.VisibleWhen != "{{flw.exists(other)}}" {
		t.Errorf("visibleWhen = %q, want the condition surfaced verbatim so the caller can judge", f.VisibleWhen)
	}
	if !strings.Contains(out.Message, "only shows it when") {
		t.Errorf("the needs_input message must explain the conditionality, got %q", out.Message)
	}
}

// TestStartWorkflow_DisplayOnlyComponentsAreNotOfferedAsFields: a horizontal rule, a link and a
// read-only output all carry an element id but bind no variable. Emitting them invited the caller
// to overwrite an existing value or to invent junk process variables.
func TestStartWorkflow_DisplayOnlyComponentsAreNotOfferedAsFields(t *testing.T) {
	mux, c := newServer(t)
	handleDefinition(mux, wireDefinition{ID: workflowID, Name: "Decorated", Enabled: true, FormRequired: true, StartFormJSONModelAvailable: true, BusinessItemResourceType: "GLOBAL"})
	handleJSONModelForm(mux, `{"rows":[{"cols":[
	  {"id":"hline1","type":"horizontal-line"},
	  {"id":"link1","type":"link","label":"Docs"},
	  {"id":"subject","type":"text","label":"Subject","isRequired":true,"value":"{{subject}}"}]}]}`)

	out, err := start_workflow.NewTool(c).Handler(t.Context(), start_workflow.Input{WorkflowDefinitionID: workflowID})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out.FormFields) != 1 || out.FormFields[0].ID != "subject" {
		t.Fatalf("only the bound field is fillable, got %+v", out.FormFields)
	}
}

// TestStartWorkflow_JSONMultiSelectAcceptsSeveralValues: the multi flag lives in extraSettings, not
// on the col. Without reading it a multi-select rejected "a,b" whole against its option list, with
// no value the caller could ever supply and an empty missingFields to re-target — a permanent loop.
func TestStartWorkflow_JSONMultiSelectAcceptsSeveralValues(t *testing.T) {
	mux, c := newServer(t)
	handleDefinition(mux, wireDefinition{ID: workflowID, Name: "Multi", Enabled: true, FormRequired: true, StartFormJSONModelAvailable: true, BusinessItemResourceType: "GLOBAL"})
	handleJSONModelForm(mux, `{"rows":[{"cols":[{"id":"tags","type":"select","label":"Tags","isRequired":true,"value":"{{tags}}","extraSettings":{"multi":true,"items":[{"value":"a","label":"A"},{"value":"b","label":"B"}]}}]}]}`)
	handleStartRaw(mux, "/rest/2.0/internal/workflow/startWithForm", nil, nil, http.StatusCreated)

	out, err := start_workflow.NewTool(c).Handler(t.Context(), start_workflow.Input{
		WorkflowDefinitionID: workflowID,
		FormProperties:       map[string]string{"tags": "a,b"},
		Confirm:              true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Status != start_workflow.StatusSuccess {
		t.Fatalf("a comma-separated list of valid keys was rejected on a multi-select: %s — %s", out.Status, out.Message)
	}
}

// TestStartWorkflow_NonStringDefaultIsNotDropped: the palette declares a checkbox default as a real
// boolean and a picker's as an array. Reading defaultValue with a string-only accessor dropped
// those, which reintroduced the unbound-variable failure the defaults exist to prevent.
func TestStartWorkflow_NonStringDefaultIsNotDropped(t *testing.T) {
	mux, c := newServer(t)
	handleDefinition(mux, wireDefinition{ID: workflowID, Name: "Bool", Enabled: true, FormRequired: true, StartFormJSONModelAvailable: true, BusinessItemResourceType: "GLOBAL"})
	handleJSONModelForm(mux, `{"rows":[{"cols":[{"id":"c1","type":"boolean","label":"Urgent","isRequired":false,"value":"{{urgent}}","defaultValue":true}]}]}`)
	var body string
	handleStartRaw(mux, "/rest/2.0/internal/workflow/startWithForm", &body, nil, http.StatusCreated)

	if _, err := start_workflow.NewTool(c).Handler(t.Context(), start_workflow.Input{
		WorkflowDefinitionID: workflowID, Confirm: true,
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var sent map[string]any
	if uerr := json.Unmarshal([]byte(body), &sent); uerr != nil {
		t.Fatalf("unparseable request body %q: %v", body, uerr)
	}
	props, _ := sent["formProperties"].(map[string]any)
	if props["urgent"] != true {
		t.Errorf("urgent = %#v, want the boolean true — a non-string default must survive, and reach the engine typed", props["urgent"])
	}
}

// TestStartWorkflow_BooleanValueIsSentTyped: the reason this path uses the form-engine endpoint at
// all is that it carries typed values. Sending the string "false" defeats that — a non-empty
// string is truthy in Groovy, so a start script's `if (urgent)` takes the branch the user did not
// choose.
func TestStartWorkflow_BooleanValueIsSentTyped(t *testing.T) {
	mux, c := newServer(t)
	handleDefinition(mux, wireDefinition{ID: workflowID, Name: "Bool", Enabled: true, FormRequired: true, StartFormJSONModelAvailable: true, BusinessItemResourceType: "GLOBAL"})
	handleJSONModelForm(mux, `{"rows":[{"cols":[{"id":"c1","type":"boolean","label":"Urgent","isRequired":false,"value":"{{urgent}}"}]}]}`)
	var body string
	handleStartRaw(mux, "/rest/2.0/internal/workflow/startWithForm", &body, nil, http.StatusCreated)

	if _, err := start_workflow.NewTool(c).Handler(t.Context(), start_workflow.Input{
		WorkflowDefinitionID: workflowID,
		FormProperties:       map[string]string{"urgent": "false"},
		Confirm:              true,
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var sent map[string]any
	if uerr := json.Unmarshal([]byte(body), &sent); uerr != nil {
		t.Fatalf("unparseable request body %q: %v", body, uerr)
	}
	props, _ := sent["formProperties"].(map[string]any)
	if props["urgent"] != false {
		t.Errorf("urgent = %#v (%T), want the boolean false — the string \"false\" is truthy in Groovy", props["urgent"], props["urgent"])
	}
}

// TestStartWorkflow_JSONUnsupportedStencilsExplainThemselves mirrors the legacy path: a file upload
// cannot be produced through this API at all, and role-in-community needs a paired structure.
// Without this the JSON model sent the caller to search_asset_keyword, which can do neither.
func TestStartWorkflow_JSONUnsupportedStencilsExplainThemselves(t *testing.T) {
	for _, tc := range []struct{ name, stencil, wantIn string }{
		{"fileUpload", "collibra-fileUpload", "uploaded file"},
		{"roleInCommunity", "collibra-roleInCommunity", "[roleId, communityId]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mux, c := newServer(t)
			handleDefinition(mux, wireDefinition{ID: workflowID, Name: "Odd", Enabled: true, FormRequired: true, StartFormJSONModelAvailable: true, BusinessItemResourceType: "GLOBAL"})
			handleJSONModelForm(mux, `{"rows":[{"cols":[{"designInfo":{"stencilId":"`+tc.stencil+`"},"id":"odd","type":"text","label":"Odd","isRequired":true,"value":"{{odd}}"}]}]}`)

			out, err := start_workflow.NewTool(c).Handler(t.Context(), start_workflow.Input{WorkflowDefinitionID: workflowID})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if out.FormFields[0].Unsupported == "" {
				t.Fatalf("%s must carry a reason it cannot be answered here", tc.stencil)
			}
			if !strings.Contains(out.Message, tc.wantIn) {
				t.Errorf("message %q does not explain the real constraint", out.Message)
			}
			if strings.Contains(out.Message, "search_asset_keyword") {
				t.Errorf("message still sends the caller to a lookup that cannot succeed: %q", out.Message)
			}
		})
	}
}

// TestStartWorkflow_FormFetchFailureIsAnErrorNotAPreview: every way of failing to read a start
// form must stop the flow. Reporting a preview instead means the caller sees "nothing missing",
// confirms, and starts a real workflow with an entirely empty form.
func TestStartWorkflow_FormFetchFailureIsAnErrorNotAPreview(t *testing.T) {
	for _, tc := range []struct {
		name string
		wire func(*http.ServeMux)
		json bool
	}{
		{"legacy endpoint 500", func(mux *http.ServeMux) {
			mux.HandleFunc("GET /rest/2.0/workflowDefinitions/workflowDefinition/"+workflowID+"/startFormData",
				func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusInternalServerError) })
		}, false},
		{"graphql errors", func(mux *http.ServeMux) {
			mux.HandleFunc("POST /graphql", func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write([]byte(`{"errors":[{"message":"boom"}]}`))
			})
		}, true},
		{"graphql null model", func(mux *http.ServeMux) {
			mux.HandleFunc("POST /graphql", func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write([]byte(`{"data":{"api":{"workflowStartFormJsonModel":null}}}`))
			})
		}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mux, c := newServer(t)
			handleDefinition(mux, wireDefinition{ID: workflowID, Name: "F", Enabled: true, FormRequired: true, StartFormJSONModelAvailable: tc.json, BusinessItemResourceType: "GLOBAL"})
			tc.wire(mux)
			started := false
			handleStartRaw(mux, "/rest/2.0/workflowInstances", nil, &started, http.StatusCreated)
			handleStartRaw(mux, "/rest/2.0/internal/workflow/startWithForm", nil, &started, http.StatusCreated)

			out, err := start_workflow.NewTool(c).Handler(t.Context(), start_workflow.Input{
				WorkflowDefinitionID: workflowID, Confirm: true,
			})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if out.Status != start_workflow.StatusError {
				t.Errorf("status = %q, want error — a form that could not be read must not become a clean preview or a start", out.Status)
			}
			if started {
				t.Errorf("the workflow was started even though its form could not be read")
			}
		})
	}
}

// TestStartWorkflow_GlobalStartSendsNoBusinessItem asserts the raw body of a GLOBAL start. The
// output schema promises "if you passed one and it is missing here, it was not used"; nothing
// checked that the wire actually honours it.
func TestStartWorkflow_GlobalStartSendsNoBusinessItem(t *testing.T) {
	mux, c := newServer(t)
	handleDefinition(mux, wireDefinition{ID: workflowID, Name: "G", Enabled: true, BusinessItemResourceType: "GLOBAL"})
	var body string
	handleStartRaw(mux, "/rest/2.0/workflowInstances", &body, nil, http.StatusCreated)

	out, err := start_workflow.NewTool(c).Handler(t.Context(), start_workflow.Input{
		WorkflowDefinitionID: workflowID,
		BusinessItemID:       assetID, // supplied but irrelevant for a GLOBAL workflow
		Confirm:              true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Status != start_workflow.StatusSuccess {
		t.Fatalf("status = %q: %s", out.Status, out.Message)
	}
	var sent map[string]any
	if uerr := json.Unmarshal([]byte(body), &sent); uerr != nil {
		t.Fatalf("bad body %q: %v", body, uerr)
	}
	if v, present := sent["businessItemIds"]; present {
		t.Errorf("a GLOBAL workflow must be started against no resource, but the body carried businessItemIds=%v", v)
	}
	if out.BusinessItemID != "" {
		t.Errorf("output businessItemId = %q, want empty — it was not used", out.BusinessItemID)
	}
}

// TestStartWorkflow_ForbiddenStartDoesNotAdviseRetry: the 403 branch of startError was satisfied by
// the generic default branch, whose guidance says "Retry shortly" — the never-winnable loop the
// lookup-side test already forbids.
func TestStartWorkflow_ForbiddenStartDoesNotAdviseRetry(t *testing.T) {
	mux, c := newServer(t)
	handleDefinition(mux, wireDefinition{ID: workflowID, Name: "P", Enabled: true, BusinessItemResourceType: "GLOBAL"})
	handleStartRaw(mux, "/rest/2.0/workflowInstances", nil, nil, http.StatusForbidden)

	out, err := start_workflow.NewTool(c).Handler(t.Context(), start_workflow.Input{
		WorkflowDefinitionID: workflowID, Confirm: true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(out.Guidance, "Retry") && !strings.Contains(out.Guidance, "Do not retry") {
		t.Errorf("a 403 must not advise retrying a call that can never succeed, got %q", out.Guidance)
	}
	if !strings.Contains(out.Guidance, "administrator") {
		t.Errorf("a 403 should point at the remedy that exists, got %q", out.Guidance)
	}
}

// TestStartWorkflow_OptionsExhaustiveReachesTheCaller: checkValue's use of the flag is pinned, but
// nothing checked it is actually projected into the response. Without it the model reads every
// server shortlist as a closed set and refuses ids that are perfectly valid.
func TestStartWorkflow_OptionsExhaustiveReachesTheCaller(t *testing.T) {
	mux, c := newServer(t)
	handleDefinition(mux, wireDefinition{ID: workflowID, Name: "S", Enabled: true, FormRequired: true, BusinessItemResourceType: "GLOBAL"})
	handleLegacyForm(mux, workflowID, `{"formProperties":[
	  {"id":"closed","name":"Closed","type":"enum","required":true,"enumValues":[{"idAsString":"a","text":"A"}]},
	  {"id":"open","name":"Open","type":"role","required":true,"proposedDropdownValues":[{"idAsString":"r1","text":"R1"}],"proposedFixed":false}]}`)

	out, err := start_workflow.NewTool(c).Handler(t.Context(), start_workflow.Input{WorkflowDefinitionID: workflowID})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	byID := map[string]start_workflow.FormField{}
	for _, f := range out.FormFields {
		byID[f.ID] = f
	}
	if !byID["closed"].OptionsExhaustive {
		t.Errorf("an enum is a closed set and must be reported exhaustive")
	}
	if byID["open"].OptionsExhaustive {
		t.Errorf("a non-fixed shortlist must be reported open, or the caller will refuse valid ids")
	}
}

// --- Invariants ---
//
// These tests do not check one branch each. They assert a property across the whole space, because
// the defects they guard kept coming back through NEW branches: three separate review rounds found
// the same shape of bug in a different place. A per-case test only pins the case it names.

// TestInvariant_NoStartFailureEverInvitesABlindRetryOfASucceededWrite sweeps every status code the
// start call can plausibly return and enforces two rules that must hold for a non-idempotent write:
//
//   - a 2xx must never claim nothing was created, and must never advise a plain retry — Collibra
//     accepted the request, so a retry starts the workflow a second time;
//   - a lost connection (code 0) must not advise a plain retry either, because a request that was
//     accepted and then lost is indistinguishable here from one that never arrived.
//
// Codes that genuinely mean "nothing happened" (4xx/5xx) may advise a retry.
func TestInvariant_NoStartFailureEverInvitesABlindRetryOfASucceededWrite(t *testing.T) {
	const wf = "22222222-2222-2222-2222-222222222222"
	codes := []int{0, 200, 201, 202, 204, 400, 401, 403, 404, 409, 422, 429, 500, 502, 503}

	for _, code := range codes {
		t.Run(http.StatusText(code)+"/"+itoa(code), func(t *testing.T) {
			mux := http.NewServeMux()
			mux.HandleFunc("GET /rest/2.0/workflowDefinitions/"+wf, func(w http.ResponseWriter, r *http.Request) {
				_ = json.NewEncoder(w).Encode(wireDefinition{ID: wf, Name: "W", Enabled: true, BusinessItemResourceType: "GLOBAL"})
			})
			srv := httptest.NewServer(mux)
			defer srv.Close()
			c := testutil.NewClient(srv)

			if code == 0 {
				// A real transport failure, not a status: the start endpoint is simply not
				// reachable. Writing a 200 here (as an earlier version of this test did) exercised
				// the success path instead and left the whole code-0 rule unverified.
				mux.HandleFunc("POST /rest/2.0/workflowInstances", func(w http.ResponseWriter, r *http.Request) {
					hj, ok := w.(http.Hijacker)
					if !ok {
						t.Skip("cannot simulate a dropped connection")
					}
					conn, _, _ := hj.Hijack()
					_ = conn.Close() // accepted, then the connection dies before the response
				})
			} else {
				mux.HandleFunc("POST /rest/2.0/workflowInstances", func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(code)
					// Deliberately unreadable: the shape that makes a 2xx reach the error path.
					_, _ = w.Write([]byte(`[]`))
				})
			}

			out, err := start_workflow.NewTool(c).Handler(t.Context(), start_workflow.Input{
				WorkflowDefinitionID: wf, Confirm: true,
			})
			if err != nil {
				t.Fatalf("unexpected transport error: %v", err)
			}
			if out.Status == start_workflow.StatusSuccess {
				return // nothing to guard: it worked
			}

			guidance := strings.ToLower(out.Guidance)
			advisesRetry := strings.Contains(guidance, "retry") &&
				!strings.Contains(guidance, "do not retry") && !strings.Contains(guidance, "not retry")

			mayHaveStarted := code == 0 || (code >= 200 && code < 300)
			if mayHaveStarted {
				if advisesRetry {
					t.Errorf("HTTP %d may have started the workflow, but the guidance invites a retry: %q", code, out.Guidance)
				}
				if strings.Contains(out.Message, "NO instance was created") ||
					strings.Contains(guidance, "no instance was created") {
					t.Errorf("HTTP %d cannot claim nothing was created: %q / %q", code, out.Message, out.Guidance)
				}
			}
			if out.WorkflowInstanceID != "" {
				t.Errorf("HTTP %d produced no readable instance, so no id may be reported, got %q", code, out.WorkflowInstanceID)
			}
		})
	}
}

func itoa(i int) string { return fmt.Sprintf("%d", i) }

// TestInvariant_ValidatedEqualsPreviewedEqualsSent is the structural guard for the defect that
// recurred in three separate review rounds: the value that gets validated, the value shown in the
// preview and the value put on the wire were each derived separately and drifted apart — trimmed
// here but not there, defaults applied after validation rather than before, a list normalised for
// checking but submitted raw.
//
// Rather than pin each instance, this sweeps inputs that have historically broken and asserts the
// three are identical. Any future change that reintroduces a second derivation fails here.
func TestInvariant_ValidatedEqualsPreviewedEqualsSent(t *testing.T) {
	const wf = "22222222-2222-2222-2222-222222222222"
	model := `{"rows":[{"cols":[
	  {"id":"a","type":"text","label":"Plain","isRequired":false,"value":"{{plain}}"},
	  {"id":"b","type":"select","label":"Defaulted","isRequired":true,"value":"{{defaulted}}","defaultValue":"Normal"},
	  {"id":"c","type":"select","label":"Tags","isRequired":false,"value":"{{tags}}","extraSettings":{"multi":true,"items":[{"value":"x","label":"X"},{"value":"y","label":"Y"}]}}]}]}`

	for _, tc := range []struct {
		name     string
		supplied map[string]string
	}{
		{"nothing supplied — the required field's own default must satisfy it", nil},
		{"padded scalar", map[string]string{"plain": "  hello  "}},
		{"padded multi-value list", map[string]string{"tags": "x, y"}},
		{"default overridden", map[string]string{"defaulted": "Urgent"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mux, c := newServer(t)
			handleDefinition(mux, wireDefinition{ID: wf, Name: "W", Enabled: true, FormRequired: true, StartFormJSONModelAvailable: true, BusinessItemResourceType: "GLOBAL"})
			handleJSONModelForm(mux, model)
			var body string
			handleStartRaw(mux, "/rest/2.0/internal/workflow/startWithForm", &body, nil, http.StatusCreated)

			tool := start_workflow.NewTool(c)
			in := start_workflow.Input{WorkflowDefinitionID: wf, FormProperties: tc.supplied}

			prev, err := tool.Handler(t.Context(), in)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			// Validation must not object to a payload it is itself going to send.
			if prev.Status != start_workflow.StatusPreview {
				t.Fatalf("status = %q, want preview — validation rejected values the tool would have submitted: %s", prev.Status, prev.Message)
			}

			in.Confirm = true
			if _, err := tool.Handler(t.Context(), in); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			var sent map[string]any
			if uerr := json.Unmarshal([]byte(body), &sent); uerr != nil {
				t.Fatalf("unparseable request body %q: %v", body, uerr)
			}
			props, ok := sent["formProperties"].(map[string]any)
			if !ok {
				t.Fatalf("request carried no formProperties object: %s", body)
			}

			// Internal consistency is not enough on its own: if BOTH the preview and the request
			// carried the raw " y", they would agree with each other and still be wrong on the
			// wire, because the server splits on commas too and would reject the padded entry.
			for k, v := range prev.FormProperties {
				if strings.Contains(v, ", ") {
					t.Errorf("field %q is submitted as %q — a multi-value list must be canonicalised, or the server rejects the padded entry after the user approved it", k, v)
				}
			}

			if len(props) != len(prev.FormProperties) {
				t.Fatalf("previewed %d fields but sent %d: previewed=%v sent=%v", len(prev.FormProperties), len(props), prev.FormProperties, props)
			}
			for k, previewed := range prev.FormProperties {
				got, present := props[k]
				if !present {
					t.Errorf("field %q was previewed as %q but never sent", k, previewed)
					continue
				}
				if s, isString := got.(string); isString && s != previewed {
					t.Errorf("field %q: previewed %q, sent %q — preview and request must not disagree", k, previewed, s)
				}
			}
		})
	}
}

// TestExplicitlyClearingARequiredFieldIsStillMissing is the other side of the default rule, and
// the invariant sweep above surfaced it: leaving a defaulted field out means "use the default",
// but explicitly setting it to empty means "I want it blank" — and for a REQUIRED field that has
// to be refused, or the workflow starts with a mandatory value unset.
func TestExplicitlyClearingARequiredFieldIsStillMissing(t *testing.T) {
	const wf = "22222222-2222-2222-2222-222222222222"
	mux, c := newServer(t)
	handleDefinition(mux, wireDefinition{ID: wf, Name: "W", Enabled: true, FormRequired: true, StartFormJSONModelAvailable: true, BusinessItemResourceType: "GLOBAL"})
	handleJSONModelForm(mux, `{"rows":[{"cols":[{"id":"p","type":"select","label":"Priority","isRequired":true,"value":"{{priority}}","defaultValue":"Normal"}]}]}`)
	started := false
	handleStartRaw(mux, "/rest/2.0/internal/workflow/startWithForm", nil, &started, http.StatusCreated)

	out, err := start_workflow.NewTool(c).Handler(t.Context(), start_workflow.Input{
		WorkflowDefinitionID: wf,
		FormProperties:       map[string]string{"priority": ""},
		Confirm:              true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Status != start_workflow.StatusNeedsInput {
		t.Fatalf("status = %q, want needs_input — clearing a required field is not the same as leaving it to its default", out.Status)
	}
	if started {
		t.Errorf("the workflow was started with a required field explicitly blanked")
	}
}

// TestRequiredFieldWithADefaultIsNotReportedMissing: the form pre-fills it, so the caller omitting
// it is not an omission — the schema for defaultValue explicitly invites leaving it out. Reporting
// it missing produced a needs_input the caller could satisfy only by repeating the default back.
func TestRequiredFieldWithADefaultIsNotReportedMissing(t *testing.T) {
	const wf = "22222222-2222-2222-2222-222222222222"
	mux, c := newServer(t)
	handleDefinition(mux, wireDefinition{ID: wf, Name: "W", Enabled: true, FormRequired: true, StartFormJSONModelAvailable: true, BusinessItemResourceType: "GLOBAL"})
	handleJSONModelForm(mux, `{"rows":[{"cols":[{"id":"p","type":"select","label":"Priority","isRequired":true,"value":"{{priority}}","defaultValue":"Normal"}]}]}`)

	out, err := start_workflow.NewTool(c).Handler(t.Context(), start_workflow.Input{WorkflowDefinitionID: wf})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Status != start_workflow.StatusPreview {
		t.Fatalf("status = %q, want preview: a required field the form itself fills is not missing (missing=%v)", out.Status, out.MissingFields)
	}
	if out.FormProperties["priority"] != "Normal" {
		t.Errorf("the default must be what gets submitted, got %q", out.FormProperties["priority"])
	}
}

// TestStartWorkflow_NoDefaultIsInjectedForAFieldTheFormWillNotAccept. Defaults are injected so the
// process sees what the product's own UI would have sent. That reasoning stops at fields the form
// will not accept a value for: a disabled one (the server rejects a submitted value outright), an
// unsupported one (the caller was just told it cannot be produced from here), and one the form
// hides unconditionally. Sending their defaults anyway turns a helpful pre-fill into a 400 — or
// worse, writes a value for a field the user was never shown.
func TestStartWorkflow_NoDefaultIsInjectedForAFieldTheFormWillNotAccept(t *testing.T) {
	mux, c := newServer(t)
	handleDefinition(mux, wireDefinition{ID: workflowID, Name: "Guarded", Enabled: true, FormRequired: true, StartFormJSONModelAvailable: true, BusinessItemResourceType: "GLOBAL"})
	handleJSONModelForm(mux, `{"rows":[{"cols":[
	  {"id":"f1","type":"text","label":"Subject","isRequired":true,"value":"{{subject}}"},
	  {"id":"f2","type":"text","label":"Disabled","value":"{{locked}}","defaultValue":"lockedDefault","enabled":false},
	  {"id":"f3","type":"collibra-fileUpload","label":"Attachment","value":"{{upload}}","defaultValue":"uploadDefault","designInfo":{"stencilId":"collibra-fileUpload"}},
	  {"id":"f4","type":"text","label":"Hidden","value":"{{hidden}}","defaultValue":"hiddenDefault","visible":false},
	  {"id":"f5","type":"text","label":"Ordinary","value":"{{ordinary}}","defaultValue":"ordinaryDefault"}]}]}`)
	var body string
	handleStartRaw(mux, "/rest/2.0/internal/workflow/startWithForm", &body, nil, http.StatusCreated)

	tool := start_workflow.NewTool(c)
	if _, err := tool.Handler(t.Context(), start_workflow.Input{
		WorkflowDefinitionID: workflowID,
		FormProperties:       map[string]string{"subject": "x"},
		Confirm:              true,
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var sent map[string]any
	if uerr := json.Unmarshal([]byte(body), &sent); uerr != nil {
		t.Fatalf("unparseable request body %q: %v", body, uerr)
	}
	props, _ := sent["formProperties"].(map[string]any)
	for _, key := range []string{"locked", "upload", "hidden"} {
		if v, present := props[key]; present && v != nil {
			t.Errorf("%s = %v, want absent-or-null: the form does not accept a value for this field", key, v)
		}
	}
	if props["ordinary"] != "ordinaryDefault" {
		t.Errorf("ordinary = %v, want its default — the guard must not suppress a perfectly submittable field", props["ordinary"])
	}
}

// ...and the caller may still set an UNSUPPORTED or never-shown field explicitly. The guard
// governs what chip volunteers on the caller's behalf, not what the caller asked for; suppressing
// an explicit value would be a silent drop, which is the failure mode this area exists to avoid.
func TestStartWorkflow_AnExplicitValueForAGuardedFieldIsStillSent(t *testing.T) {
	for _, tc := range []struct{ name, col string }{
		{"never visible", `{"id":"f2","type":"text","label":"Hidden","value":"{{guarded}}","defaultValue":"formDefault","visible":false}`},
		{"unsupported", `{"id":"f2","type":"collibra-fileUpload","label":"File","value":"{{guarded}}","designInfo":{"stencilId":"collibra-fileUpload"}}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mux, c := newServer(t)
			handleDefinition(mux, wireDefinition{ID: workflowID, Name: "Guarded", Enabled: true, FormRequired: true, StartFormJSONModelAvailable: true, BusinessItemResourceType: "GLOBAL"})
			handleJSONModelForm(mux, `{"rows":[{"cols":[`+tc.col+`]}]}`)
			var body string
			handleStartRaw(mux, "/rest/2.0/internal/workflow/startWithForm", &body, nil, http.StatusCreated)

			if _, err := start_workflow.NewTool(c).Handler(t.Context(), start_workflow.Input{
				WorkflowDefinitionID: workflowID,
				FormProperties:       map[string]string{"guarded": "chosenByCaller"},
				Confirm:              true,
			}); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			var sent map[string]any
			if uerr := json.Unmarshal([]byte(body), &sent); uerr != nil {
				t.Fatalf("unparseable request body %q: %v", body, uerr)
			}
			props, _ := sent["formProperties"].(map[string]any)
			if props["guarded"] != "chosenByCaller" {
				t.Errorf("guarded = %v, want the caller's own value", props["guarded"])
			}
		})
	}
}

// TestStartWorkflow_AValueForAReadOnlyFieldIsRefusedBeforeTheWrite. Read-only is the one case
// where the caller's own value must NOT be forwarded, and it applies to both form models.
//
// The legacy engine answers a value for a non-writable property with a hard failure — "form
// property '<id>' is not writable" — thrown inside the transactional start, so the whole thing
// rolls back into an opaque 500 after the user already approved the preview. Catching it here
// costs one round trip and names the field; letting it through costs a confirmed write that dies.
//
// It is easy to trip: formFields shows the form's declared value, and echoing that value back is
// the obvious thing for a caller to do.
func TestStartWorkflow_AValueForAReadOnlyFieldIsRefusedBeforeTheWrite(t *testing.T) {
	for _, tc := range []struct {
		name  string
		setup func(*http.ServeMux)
	}{
		{"legacy", func(mux *http.ServeMux) {
			handleDefinition(mux, wireDefinition{ID: workflowID, Name: "RO", Enabled: true, FormRequired: true, BusinessItemResourceType: "GLOBAL"})
			handleLegacyForm(mux, workflowID, `{"formProperties":[{"id":"locked","name":"Locked","type":"string","writable":false,"value":"fromTheForm"}]}`)
		}},
		{"json model", func(mux *http.ServeMux) {
			handleDefinition(mux, wireDefinition{ID: workflowID, Name: "RO", Enabled: true, FormRequired: true, StartFormJSONModelAvailable: true, BusinessItemResourceType: "GLOBAL"})
			handleJSONModelForm(mux, `{"rows":[{"cols":[{"id":"f1","type":"text","label":"Locked","value":"{{locked}}","defaultValue":"fromTheForm","enabled":false}]}]}`)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mux, c := newServer(t)
			tc.setup(mux)
			started := false
			handleStart(mux, nil, &started, http.StatusCreated)
			handleStartWithForm(mux, nil, &started)

			out, err := start_workflow.NewTool(c).Handler(t.Context(), start_workflow.Input{
				WorkflowDefinitionID: workflowID,
				FormProperties:       map[string]string{"locked": "fromTheForm"},
				Confirm:              true,
			})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if out.Status != start_workflow.StatusNeedsInput {
				t.Fatalf("status = %q, want needs_input: a value for a read-only field must be refused, not written", out.Status)
			}
			if started {
				t.Error("nothing may be started when the payload carries a value the engine will reject")
			}
			if !strings.Contains(strings.Join([]string{out.Message, out.Guidance}, " "), "read-only") {
				t.Errorf("the caller must be told WHY: %q", out.Message)
			}
		})
	}
}

// TestStartWorkflow_BooleanFieldAcceptsTheWordsAHumanWrites: the value reaching this tool is
// produced by a language model from a human sentence, so "yes"/"no"/"on"/"off" are at least as
// likely as "true"/"false". strconv.ParseBool rejects all of them, and the field then went out as
// the STRING "yes" where the process expects a boolean — a type mismatch nothing downstream
// reports. Anything genuinely ambiguous must still be refused rather than guessed.
func TestStartWorkflow_BooleanFieldAcceptsTheWordsAHumanWrites(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want any
	}{
		{"true", true}, {"false", false},
		{"yes", true}, {"no", false},
		{"Yes", true}, {"NO", false},
		{"on", true}, {"off", false},
		{"1", true}, {"0", false},
		{" true ", true},
		{"checked", true}, {"unchecked", false},
		{"maybe", "maybe"}, // not a boolean in any reading — passed through verbatim, not guessed
	} {
		t.Run(tc.in, func(t *testing.T) {
			mux, c := newServer(t)
			handleDefinition(mux, wireDefinition{ID: workflowID, Name: "Bools", Enabled: true, FormRequired: true, StartFormJSONModelAvailable: true, BusinessItemResourceType: "GLOBAL"})
			handleJSONModelForm(mux, `{"rows":[{"cols":[{"id":"f1","type":"checkbox","label":"Agree","value":"{{agree}}"}]}]}`)
			var body string
			handleStartRaw(mux, "/rest/2.0/internal/workflow/startWithForm", &body, nil, http.StatusCreated)

			if _, err := start_workflow.NewTool(c).Handler(t.Context(), start_workflow.Input{
				WorkflowDefinitionID: workflowID,
				FormProperties:       map[string]string{"agree": tc.in},
				Confirm:              true,
			}); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			var sent map[string]any
			if uerr := json.Unmarshal([]byte(body), &sent); uerr != nil {
				t.Fatalf("unparseable request body %q: %v", body, uerr)
			}
			props, _ := sent["formProperties"].(map[string]any)
			if props["agree"] != tc.want {
				t.Errorf("agree = %#v (%T), want %#v (%T)", props["agree"], props["agree"], tc.want, tc.want)
			}
		})
	}
}

// TestStartWorkflow_FormFetchFailuresAreToldApart: reading the start form is a separate step from
// starting, and its failures have opposite remedies. All three used to collapse into one generic
// "retry" — which is wrong for a 403 (retrying never helps) and wrong for a 404 (the workflow
// changed; re-list it). Nothing was started in any of these cases, so none may hint otherwise.
func TestStartWorkflow_FormFetchFailuresAreToldApart(t *testing.T) {
	for _, tc := range []struct {
		name       string
		status     int
		wantInMsg  string
		wantGuide  string
		forbidTerm string
	}{
		{"forbidden", http.StatusForbidden, "permission", "Do not retry", ""},
		{"not found", http.StatusNotFound, "could not be found", "list_workflow_definitions", ""},
		{"server error", http.StatusInternalServerError, "500", "Retry", ""},
		{"unreachable", 0, "Could not reach Collibra", "retrying is safe", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mux, c := newServer(t)
			handleDefinition(mux, wireDefinition{ID: workflowID, Name: "FormFails", Enabled: true, FormRequired: true, BusinessItemResourceType: "GLOBAL"})
			mux.HandleFunc("GET /rest/2.0/workflowDefinitions/workflowDefinition/"+workflowID+"/startFormData", func(w http.ResponseWriter, r *http.Request) {
				if tc.status == 0 {
					// A real transport failure rather than a status: the form endpoint is simply
					// unreachable. Writing any status here would exercise a different arm and
					// leave the transport rule unverified.
					hj, ok := w.(http.Hijacker)
					if !ok {
						t.Skip("cannot simulate a dropped connection")
					}
					conn, _, _ := hj.Hijack()
					_ = conn.Close()
					return
				}
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(`{"errorCode":"X","userMessage":"nope"}`))
			})
			out, err := start_workflow.NewTool(c).Handler(t.Context(), start_workflow.Input{WorkflowDefinitionID: workflowID})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if out.Status != start_workflow.StatusError {
				t.Fatalf("status = %q, want error", out.Status)
			}
			if !strings.Contains(out.Message, tc.wantInMsg) {
				t.Errorf("message %q must contain %q", out.Message, tc.wantInMsg)
			}
			if !strings.Contains(out.Guidance, tc.wantGuide) {
				t.Errorf("guidance %q must contain %q", out.Guidance, tc.wantGuide)
			}
			if out.WorkflowInstanceID != "" {
				t.Errorf("nothing was started, so no instance id may be reported: %q", out.WorkflowInstanceID)
			}
		})
	}
}

// TestStartWorkflow_ARequiredFieldTheCallerCannotWriteIsNeverADeadEnd. A required field the
// caller has no way to supply must not come back as "missing" — that is a needs_input nobody can
// satisfy, and the caller loops on it.
//
// The two cases resolve differently, and the difference is the engines', not a preference:
//   - read-only: the value is OMITTED. The legacy engine rejects any value for a non-writable
//     property and resolves its defaultExpression itself, so omitting is both the only legal move
//     and the correct one.
//   - never shown but writable: the form's declared default IS sent, because nothing server-side
//     will fill it in and the variable would otherwise reach the process unset.
func TestStartWorkflow_ARequiredFieldTheCallerCannotWriteIsNeverADeadEnd(t *testing.T) {
	for _, tc := range []struct {
		name, col string
		wantSent  any
	}{
		{"read-only", `{"id":"f2","type":"text","label":"Locked","isRequired":true,"value":"{{locked}}","defaultValue":"fromTheForm","enabled":false}`, nil},
		{"never visible", `{"id":"f2","type":"text","label":"Hidden","isRequired":true,"value":"{{locked}}","defaultValue":"fromTheForm","visible":false}`, "fromTheForm"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mux, c := newServer(t)
			handleDefinition(mux, wireDefinition{ID: workflowID, Name: "Locked", Enabled: true, FormRequired: true, StartFormJSONModelAvailable: true, BusinessItemResourceType: "GLOBAL"})
			handleJSONModelForm(mux, `{"rows":[{"cols":[`+tc.col+`]}]}`)
			var body string
			handleStartRaw(mux, "/rest/2.0/internal/workflow/startWithForm", &body, nil, http.StatusCreated)

			out, err := start_workflow.NewTool(c).Handler(t.Context(), start_workflow.Input{
				WorkflowDefinitionID: workflowID,
				Confirm:              true,
			})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if out.Status == start_workflow.StatusNeedsInput {
				t.Fatalf("dead end: %q is required but the caller cannot supply it — %v", "locked", out.MissingFields)
			}
			var sent map[string]any
			if uerr := json.Unmarshal([]byte(body), &sent); uerr != nil {
				t.Fatalf("unparseable request body %q: %v", body, uerr)
			}
			props, _ := sent["formProperties"].(map[string]any)
			if got := props["locked"]; got != tc.wantSent {
				t.Errorf("locked = %#v, want %#v", got, tc.wantSent)
			}
		})
	}
}

// TestStartWorkflow_AnAbsentWritableFlagMeansWritable pins the direction of the fail-safe. The
// server declares `writable` as a primitive boolean and so always sends it — but reading a missing
// key as false would mark EVERY legacy field read-only, and a read-only field accepts no value.
// One absent key would turn every legacy form into one that cannot be filled in at all, and the
// tool would refuse every start it was asked for while looking perfectly healthy.
func TestStartWorkflow_AnAbsentWritableFlagMeansWritable(t *testing.T) {
	mux, c := newServer(t)
	handleDefinition(mux, wireDefinition{ID: workflowID, Name: "NoFlag", Enabled: true, FormRequired: true, BusinessItemResourceType: "GLOBAL"})
	handleLegacyForm(mux, workflowID, `{"formProperties":[{"id":"subject","name":"Subject","type":"string","required":true}]}`)
	var captured clients.StartWorkflowInstanceRequest
	started := false
	handleStart(mux, &captured, &started, http.StatusCreated)

	out, err := start_workflow.NewTool(c).Handler(t.Context(), start_workflow.Input{
		WorkflowDefinitionID: workflowID,
		FormProperties:       map[string]string{"subject": "hello"},
		Confirm:              true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Status != start_workflow.StatusSuccess {
		t.Fatalf("status = %q, want success — an absent writable flag must not make the field unfillable: %s", out.Status, out.Message)
	}
	if captured.FormProperties["subject"] != "hello" {
		t.Errorf("subject = %q, want it sent", captured.FormProperties["subject"])
	}
}

// TestStartWorkflow_LegacyDefaultsAreLeftToTheEngine pins the one place the two form models are
// deliberately NOT symmetric.
//
// The legacy engine resolves a property's own defaultExpression server-side whenever the key is
// absent from the submission. Sending the value this client happened to read earlier would at best
// duplicate that and at worst overwrite a live expression with a stale render. The JSON model has
// no equivalent step — whatever is omitted there reaches the process unset — which is why defaults
// ARE filled in on that path, and the sibling test below asserts exactly that.
//
// Written because collapsing the two branches into one is an obvious-looking simplification that
// would silently change what the legacy engine receives.
func TestStartWorkflow_LegacyDefaultsAreLeftToTheEngine(t *testing.T) {
	mux, c := newServer(t)
	handleDefinition(mux, wireDefinition{ID: workflowID, Name: "LegacyDefaults", Enabled: true, FormRequired: true, BusinessItemResourceType: "GLOBAL"})
	handleLegacyForm(mux, workflowID, `{"formProperties":[
	  {"id":"subject","name":"Subject","type":"string","required":true,"writable":true},
	  {"id":"priority","name":"Priority","type":"string","required":false,"writable":true,"value":"Normal"}]}`)
	var captured clients.StartWorkflowInstanceRequest
	handleStart(mux, &captured, nil, http.StatusCreated)

	out, err := start_workflow.NewTool(c).Handler(t.Context(), start_workflow.Input{
		WorkflowDefinitionID: workflowID,
		FormProperties:       map[string]string{"subject": "hello"},
		Confirm:              true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Status != start_workflow.StatusSuccess {
		t.Fatalf("status = %q, want success: %s", out.Status, out.Message)
	}
	if _, present := captured.FormProperties["priority"]; present {
		t.Errorf("priority = %q, want absent — the legacy engine resolves its own default, and sending a stale render overwrites a live expression", captured.FormProperties["priority"])
	}
	if captured.FormProperties["subject"] != "hello" {
		t.Errorf("subject = %q, want the caller's value", captured.FormProperties["subject"])
	}
}

// ...and the JSON model does the opposite, for the reason given above. The pair is the contract.
func TestStartWorkflow_JSONDefaultsAreSentBecauseNothingElseWill(t *testing.T) {
	mux, c := newServer(t)
	handleDefinition(mux, wireDefinition{ID: workflowID, Name: "JSONDefaults", Enabled: true, FormRequired: true, StartFormJSONModelAvailable: true, BusinessItemResourceType: "GLOBAL"})
	handleJSONModelForm(mux, `{"rows":[{"cols":[
	  {"id":"f1","type":"text","label":"Subject","isRequired":true,"value":"{{subject}}"},
	  {"id":"f2","type":"select","label":"Priority","value":"{{priority}}","defaultValue":"Normal"}]}]}`)
	var body string
	handleStartRaw(mux, "/rest/2.0/internal/workflow/startWithForm", &body, nil, http.StatusCreated)

	if _, err := start_workflow.NewTool(c).Handler(t.Context(), start_workflow.Input{
		WorkflowDefinitionID: workflowID,
		FormProperties:       map[string]string{"subject": "hello"},
		Confirm:              true,
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var sent map[string]any
	if uerr := json.Unmarshal([]byte(body), &sent); uerr != nil {
		t.Fatalf("unparseable request body %q: %v", body, uerr)
	}
	props, _ := sent["formProperties"].(map[string]any)
	if props["priority"] != "Normal" {
		t.Errorf("priority = %v, want %q — nothing server-side will fill it in on this path", props["priority"], "Normal")
	}
}

// TestStartWorkflow_LegacyRequiredFieldWithADeclaredDefaultIsNotDemanded. Found by running the
// tool against a real instance: a workflow with five required fields that all declare defaults
// came back asking the caller to supply them, including duration strings in the form's own
// notation ("B3D", "B5D") and a vote percentage. A model asked for those will guess, and a guess
// is indistinguishable from an answer.
//
// It cannot be demanded, because the engine already has it. On a START form there is no execution
// to read a variable from, so a rendered value can only have come from the property's
// defaultExpression — and submitFormProperty resolves that expression whenever the key is absent.
// So the field is satisfied by omission, which is also why nothing is sent for it.
func TestStartWorkflow_LegacyRequiredFieldWithADeclaredDefaultIsNotDemanded(t *testing.T) {
	mux, c := newServer(t)
	handleDefinition(mux, wireDefinition{ID: workflowID, Name: "Voting", Enabled: true, FormRequired: true, BusinessItemResourceType: "GLOBAL"})
	handleLegacyForm(mux, workflowID, `{"formProperties":[
	  {"id":"decisionInfo","name":"Decision","type":"string","required":true,"writable":true},
	  {"id":"voteTimeout","name":"Timeout","type":"string","required":true,"writable":true,"value":"B5D"},
	  {"id":"votePercentage","name":"Percentage","type":"long","required":true,"writable":true,"value":"50"}]}`)
	var captured clients.StartWorkflowInstanceRequest
	handleStart(mux, &captured, nil, http.StatusCreated)

	tool := start_workflow.NewTool(c)
	out, err := tool.Handler(t.Context(), start_workflow.Input{WorkflowDefinitionID: workflowID})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got, want := out.MissingFields, []string{"decisionInfo"}; len(got) != 1 || got[0] != want[0] {
		t.Fatalf("missingFields = %v, want only %v — the other two declare defaults the engine resolves", got, want)
	}

	// ...and once the genuinely missing one is supplied, the defaulted fields are still not SENT:
	// the engine owns them, and echoing a stale render would overwrite a live expression.
	if _, err := tool.Handler(t.Context(), start_workflow.Input{
		WorkflowDefinitionID: workflowID,
		FormProperties:       map[string]string{"decisionInfo": "ship it"},
		Confirm:              true,
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, id := range []string{"voteTimeout", "votePercentage"} {
		if v, present := captured.FormProperties[id]; present {
			t.Errorf("%s = %q, want absent", id, v)
		}
	}
}

// TestStartWorkflow_ExplicitlyClearingAFieldWithADefaultIsStillMissing guards the boundary of the
// rule above. "Supplied nothing" and "supplied an empty string" must not collapse into one case,
// or a caller deliberately blanking a field silently gets the default back instead.
func TestStartWorkflow_ExplicitlyClearingAFieldWithADefaultIsStillMissing(t *testing.T) {
	mux, c := newServer(t)
	handleDefinition(mux, wireDefinition{ID: workflowID, Name: "Voting", Enabled: true, FormRequired: true, BusinessItemResourceType: "GLOBAL"})
	handleLegacyForm(mux, workflowID, `{"formProperties":[{"id":"voteTimeout","name":"Timeout","type":"string","required":true,"writable":true,"value":"B5D"}]}`)
	started := false
	handleStart(mux, nil, &started, http.StatusCreated)

	out, err := start_workflow.NewTool(c).Handler(t.Context(), start_workflow.Input{
		WorkflowDefinitionID: workflowID,
		FormProperties:       map[string]string{"voteTimeout": ""},
		Confirm:              true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Status != start_workflow.StatusNeedsInput {
		t.Fatalf("status = %q, want needs_input: an explicit clear is a deliberate act, not an omission", out.Status)
	}
	if started {
		t.Error("nothing may be started")
	}
}

// TestStartWorkflow_LegacyButtonReportsNoDefaultValue. A button's renderer answers "false" for
// every model value it is handed, null included, so the value it reports is an artifact of the
// renderer and not a declared default. Surfacing it invited the caller to submit a "default" the
// form never declared — and since the engine treats a button as an ordinary boolean the process
// may branch on, submitting false is not harmless. The field itself stays visible for that same
// reason: it is real input, just input with no known default.
//
// Six of the nine start forms on the instance this was found on carry such a button.
func TestStartWorkflow_LegacyButtonReportsNoDefaultValue(t *testing.T) {
	for _, buttonType := range []string{"button", "activityButton", "taskButton"} {
		t.Run(buttonType, func(t *testing.T) {
			mux, c := newServer(t)
			handleDefinition(mux, wireDefinition{ID: workflowID, Name: "Buttons", Enabled: true, FormRequired: true, BusinessItemResourceType: "GLOBAL"})
			handleLegacyForm(mux, workflowID, `{"formProperties":[
			  {"id":"submit","name":"Propose","type":"`+buttonType+`","required":false,"writable":true,"value":"false"},
			  {"id":"agree","name":"Agree","type":"boolean","required":false,"writable":true,"value":"false"}]}`)
			out, err := start_workflow.NewTool(c).Handler(t.Context(), start_workflow.Input{WorkflowDefinitionID: workflowID})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			var button, boolean *start_workflow.FormField
			for i := range out.FormFields {
				switch out.FormFields[i].ID {
				case "submit":
					button = &out.FormFields[i]
				case "agree":
					boolean = &out.FormFields[i]
				}
			}
			if button == nil {
				t.Fatal("the button must still be offered — the process may branch on it")
			}
			if button.DefaultValue != "" {
				t.Errorf("button defaultValue = %q, want empty: the renderer reports \"false\" regardless of state", button.DefaultValue)
			}
			// A real boolean field is NOT a button: its renderer returns null for null, so a
			// reported "false" is a genuine declared default and must survive.
			if boolean == nil || boolean.DefaultValue != "false" {
				t.Errorf("boolean defaultValue = %+v, want \"false\" kept", boolean)
			}
		})
	}
}

// TestStartWorkflow_LegacyFieldWithNoLabelFallsBackToItsID: a form property may carry no label at
// all (seen live). Handing the caller a nameless field is worse than handing it the id, and the
// JSON path already falls back this way — the two must not disagree.
func TestStartWorkflow_LegacyFieldWithNoLabelFallsBackToItsID(t *testing.T) {
	mux, c := newServer(t)
	handleDefinition(mux, wireDefinition{ID: workflowID, Name: "NoLabel", Enabled: true, FormRequired: true, BusinessItemResourceType: "GLOBAL"})
	handleLegacyForm(mux, workflowID, `{"formProperties":[{"id":"myField","name":"","type":"string","required":true,"writable":true}]}`)
	out, err := start_workflow.NewTool(c).Handler(t.Context(), start_workflow.Input{WorkflowDefinitionID: workflowID})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out.FormFields) != 1 || out.FormFields[0].Name != "myField" {
		t.Errorf("name = %+v, want the id as a fallback", out.FormFields)
	}
}

// TestStartWorkflow_JSONMultiValueGoesOutAsAnArray. The two form models encode a list differently
// and chip must too — this is the sharpest of the deliberate asymmetries between them.
//
// The legacy endpoint takes map[string]string and the server splits the commas itself, so the
// comma-joined string the caller writes is exactly right there. The JSON endpoint takes
// map[string]any and nothing splits anything: the string arrives at the process as a string, and
// a Groovy String iterates per CHARACTER. Measured live on the OOTB "Issue Creation" workflow,
// whose start script does `relatedAssets.each { string2Uuid(it) }` — one asset id was handed to it
// 36 times, one character at a time, and the start died with an opaque HTTP 500 AFTER the user had
// confirmed. Sending ["<id>"] instead, the identical call returns 201.
//
// A single value is wrapped too, not passed bare: a one-element list is what the field means.
func TestStartWorkflow_JSONMultiValueGoesOutAsAnArray(t *testing.T) {
	for _, tc := range []struct {
		name, supplied string
		want           []any
	}{
		{"two values", "id-a,id-b", []any{"id-a", "id-b"}},
		{"one value", "id-a", []any{"id-a"}},
		{"padded", " id-a , id-b ", []any{"id-a", "id-b"}},
		{"explicit clear", "", []any{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mux, c := newServer(t)
			handleDefinition(mux, wireDefinition{ID: workflowID, Name: "Multi", Enabled: true, FormRequired: true, StartFormJSONModelAvailable: true, BusinessItemResourceType: "GLOBAL"})
			handleJSONModelForm(mux, `{"rows":[{"cols":[{"id":"f1","type":"asset","label":"Related","value":"{{relatedAssets}}","extraSettings":{"storage":"Id","multiValue":true}}]}]}`)
			var body string
			handleStartRaw(mux, "/rest/2.0/internal/workflow/startWithForm", &body, nil, http.StatusCreated)

			if _, err := start_workflow.NewTool(c).Handler(t.Context(), start_workflow.Input{
				WorkflowDefinitionID: workflowID,
				FormProperties:       map[string]string{"relatedAssets": tc.supplied},
				Confirm:              true,
			}); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			var sent map[string]any
			if uerr := json.Unmarshal([]byte(body), &sent); uerr != nil {
				t.Fatalf("unparseable request body %q: %v", body, uerr)
			}
			props, _ := sent["formProperties"].(map[string]any)
			got, isList := props["relatedAssets"].([]any)
			if !isList {
				t.Fatalf("relatedAssets = %#v (%T), want a JSON array — a string is iterated per character by the process", props["relatedAssets"], props["relatedAssets"])
			}
			if len(got) != len(tc.want) {
				t.Fatalf("relatedAssets = %#v, want %#v", got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Errorf("relatedAssets[%d] = %v, want %v", i, got[i], tc.want[i])
				}
			}
		})
	}
}

// ...and the legacy path must NOT be "fixed" the same way: the server splits the commas itself, so
// the string is already correct there. Its request type is map[string]string, so an array cannot
// physically leak onto that wire — this test pins the VALUE (trimmed, comma-joined), and the type
// pins the shape.
func TestStartWorkflow_LegacyMultiValueStaysACommaSeparatedString(t *testing.T) {
	mux, c := newServer(t)
	handleDefinition(mux, wireDefinition{ID: workflowID, Name: "Multi", Enabled: true, FormRequired: true, BusinessItemResourceType: "GLOBAL"})
	handleLegacyForm(mux, workflowID, `{"formProperties":[{"id":"tags","name":"Tags","type":"term","required":false,"writable":true,"multiValue":true}]}`)
	var captured clients.StartWorkflowInstanceRequest
	handleStart(mux, &captured, nil, http.StatusCreated)

	if _, err := start_workflow.NewTool(c).Handler(t.Context(), start_workflow.Input{
		WorkflowDefinitionID: workflowID,
		FormProperties:       map[string]string{"tags": "id-a, id-b"},
		Confirm:              true,
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if captured.FormProperties["tags"] != "id-a,id-b" {
		t.Errorf("tags = %q, want the trimmed comma-separated string the legacy engine splits itself", captured.FormProperties["tags"])
	}
}
