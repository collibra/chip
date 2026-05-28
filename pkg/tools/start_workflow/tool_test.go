package start_workflow_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/collibra/chip/pkg/clients"
	tools "github.com/collibra/chip/pkg/tools/start_workflow"
	"github.com/collibra/chip/pkg/tools/testutil"
	"github.com/google/uuid"
)

const workflowID = "11111111-1111-1111-1111-111111111111"

func formDataHandler(t *testing.T, props []clients.FormProperty) http.Handler {
	t.Helper()
	return testutil.JsonHandlerOut(func(*http.Request) (int, clients.StartFormData) {
		return http.StatusOK, clients.StartFormData{FormProperties: props}
	})
}

// Phase 1: no formProperties supplied — the tool should return the schema and
// list every required key as missing, and must NOT POST to /workflowInstances.
func TestStartWorkflow_FormRequired_WhenNoPropertiesProvided(t *testing.T) {
	handler := http.NewServeMux()
	handler.Handle("/rest/2.0/workflowDefinitions/workflowDefinition/"+workflowID+"/startFormData",
		formDataHandler(t, []clients.FormProperty{
			{ID: "name", Name: "Data product name", Type: "string", Required: true},
			{ID: "description", Name: "Description", Type: "string"},
			{ID: "community", Name: "Community", Type: "enum", Required: true,
				EnumValues: []clients.DropdownValue{
					{IDAsString: "aaaa-1", Text: "Marketing"},
					{IDAsString: "aaaa-2", Text: "Finance"},
				}},
		}))
	handler.HandleFunc("/rest/2.0/workflowInstances", func(http.ResponseWriter, *http.Request) {
		t.Fatal("workflow should not have started while required fields are missing")
	})

	server := httptest.NewServer(handler)
	defer server.Close()

	client := testutil.NewClient(server)
	out, err := tools.NewTool(client).Handler(t.Context(), tools.Input{
		WorkflowDefinitionID: workflowID,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if out.Status != tools.StatusFormRequired {
		t.Fatalf("expected status %q, got %q", tools.StatusFormRequired, out.Status)
	}
	if want := []string{"name", "community"}; !equalStringSlices(out.MissingRequired, want) {
		t.Fatalf("expected missing %v, got %v", want, out.MissingRequired)
	}
	if len(out.FormProperties) != 3 {
		t.Fatalf("expected 3 form properties, got %d", len(out.FormProperties))
	}
	// Verify enum options surfaced for the community field.
	var community tools.FormProperty
	for _, p := range out.FormProperties {
		if p.ID == "community" {
			community = p
		}
	}
	if len(community.Options) != 2 || community.Options[0].Value != "aaaa-1" || community.Options[0].Label != "Marketing" {
		t.Fatalf("community options not mapped correctly: %+v", community.Options)
	}
}

// Phase 1.5: caller supplied SOME properties but is still missing a required
// one — same behavior: schema returned, no POST.
func TestStartWorkflow_FormRequired_WhenSomeRequiredMissing(t *testing.T) {
	handler := http.NewServeMux()
	handler.Handle("/rest/2.0/workflowDefinitions/workflowDefinition/"+workflowID+"/startFormData",
		formDataHandler(t, []clients.FormProperty{
			{ID: "name", Required: true},
			{ID: "community", Required: true},
		}))
	handler.HandleFunc("/rest/2.0/workflowInstances", func(http.ResponseWriter, *http.Request) {
		t.Fatal("workflow should not have started")
	})

	server := httptest.NewServer(handler)
	defer server.Close()

	client := testutil.NewClient(server)
	out, err := tools.NewTool(client).Handler(t.Context(), tools.Input{
		WorkflowDefinitionID: workflowID,
		FormProperties:       map[string]string{"name": "Customer 360"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Status != tools.StatusFormRequired {
		t.Fatalf("expected status %q, got %q", tools.StatusFormRequired, out.Status)
	}
	if want := []string{"community"}; !equalStringSlices(out.MissingRequired, want) {
		t.Fatalf("expected missing %v, got %v", want, out.MissingRequired)
	}
}

// Phase 2: every required property supplied — the tool POSTs and returns the
// instance.
func TestStartWorkflow_Started_WhenAllRequiredProvided(t *testing.T) {
	instanceID := uuid.New().String()
	createdAsset := uuid.New().String()

	var capturedRequest clients.StartWorkflowInstancesRequest
	handler := http.NewServeMux()
	handler.Handle("/rest/2.0/workflowDefinitions/workflowDefinition/"+workflowID+"/startFormData",
		formDataHandler(t, []clients.FormProperty{
			{ID: "name", Required: true},
			{ID: "community", Required: true},
		}))
	handler.HandleFunc("/rest/2.0/workflowInstances", func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		if err := json.Unmarshal(body, &capturedRequest); err != nil {
			t.Fatalf("unmarshal request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode([]clients.WorkflowInstance{{
			ID:             instanceID,
			Ended:          false,
			StartDate:      1700000000000,
			CreatedAssetID: createdAsset,
		}})
	})

	server := httptest.NewServer(handler)
	defer server.Close()

	client := testutil.NewClient(server)
	out, err := tools.NewTool(client).Handler(t.Context(), tools.Input{
		WorkflowDefinitionID: workflowID,
		BusinessItemType:     "GLOBAL",
		FormProperties: map[string]string{
			"name":      "Customer 360",
			"community": "aaaa-1",
		},
		SendNotification: true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if out.Status != tools.StatusStarted {
		t.Fatalf("expected status %q, got %q", tools.StatusStarted, out.Status)
	}
	if len(out.Instances) != 1 || out.Instances[0].ID != instanceID {
		t.Fatalf("expected one instance with id %q, got %+v", instanceID, out.Instances)
	}
	if out.Instances[0].CreatedAssetID != createdAsset {
		t.Fatalf("expected createdAssetId %q, got %q", createdAsset, out.Instances[0].CreatedAssetID)
	}

	if capturedRequest.WorkflowDefinitionID != workflowID {
		t.Fatalf("expected workflowDefinitionId %q, got %q", workflowID, capturedRequest.WorkflowDefinitionID)
	}
	if capturedRequest.BusinessItemType != "GLOBAL" {
		t.Fatalf("expected businessItemType GLOBAL, got %q", capturedRequest.BusinessItemType)
	}
	if capturedRequest.FormProperties["name"] != "Customer 360" {
		t.Fatalf("expected form property name to be forwarded, got %q", capturedRequest.FormProperties["name"])
	}
	if !capturedRequest.SendNotification {
		t.Fatal("expected sendNotification to be forwarded")
	}
}

// Marketplace / Global-Create workflows: the task-style /startFormData returns
// an empty list, but /configurationStartFormData has the actual fields. The
// tool should fall back and surface those, not POST blindly.
func TestStartWorkflow_FallsBackToConfigurationStartFormData(t *testing.T) {
	handler := http.NewServeMux()
	handler.Handle("/rest/2.0/workflowDefinitions/workflowDefinition/"+workflowID+"/startFormData",
		formDataHandler(t, nil))
	handler.Handle("/rest/2.0/workflowDefinitions/workflowDefinition/"+workflowID+"/configurationStartFormData",
		formDataHandler(t, []clients.FormProperty{
			{ID: "dataProductName", Name: "Data product name", Type: "string", Required: true},
		}))
	handler.HandleFunc("/rest/2.0/workflowInstances", func(http.ResponseWriter, *http.Request) {
		t.Fatal("workflow should not have started while required fields are missing")
	})

	server := httptest.NewServer(handler)
	defer server.Close()

	client := testutil.NewClient(server)
	out, err := tools.NewTool(client).Handler(t.Context(), tools.Input{
		WorkflowDefinitionID: workflowID,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Status != tools.StatusFormRequired {
		t.Fatalf("expected status %q, got %q", tools.StatusFormRequired, out.Status)
	}
	if out.FormSource != "configurationStartFormData" {
		t.Fatalf("expected formSource configurationStartFormData, got %q", out.FormSource)
	}
	if want := []string{"dataProductName"}; !equalStringSlices(out.MissingRequired, want) {
		t.Fatalf("expected missing %v, got %v", want, out.MissingRequired)
	}
}

// dryRun=true short-circuits before any POST and returns whatever schema the
// platform exposed. Useful for diagnosing 'workflowNotStarted' errors.
func TestStartWorkflow_DryRunReturnsSchemaAndNeverPosts(t *testing.T) {
	handler := http.NewServeMux()
	handler.Handle("/rest/2.0/workflowDefinitions/workflowDefinition/"+workflowID+"/startFormData",
		formDataHandler(t, []clients.FormProperty{
			{ID: "name", Required: true},
			{ID: "owner", Required: true},
		}))
	handler.HandleFunc("/rest/2.0/workflowInstances", func(http.ResponseWriter, *http.Request) {
		t.Fatal("dryRun must not POST to /workflowInstances")
	})

	server := httptest.NewServer(handler)
	defer server.Close()

	client := testutil.NewClient(server)
	out, err := tools.NewTool(client).Handler(t.Context(), tools.Input{
		WorkflowDefinitionID: workflowID,
		// Even with all required values supplied, dryRun must not POST.
		FormProperties: map[string]string{"name": "x", "owner": "y"},
		DryRun:         true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Status != tools.StatusDryRun {
		t.Fatalf("expected status %q, got %q", tools.StatusDryRun, out.Status)
	}
	if len(out.FormProperties) != 2 {
		t.Fatalf("expected 2 form properties in dry-run output, got %d", len(out.FormProperties))
	}
	if out.FormSource != "startFormData" {
		t.Fatalf("expected formSource startFormData, got %q", out.FormSource)
	}
}

// dryRun=true when both endpoints return empty: tool should report it instead
// of pretending the workflow is ready. This is the case that diagnoses
// "Create Data Product (Simple)" — neither endpoint exposes anything.
func TestStartWorkflow_DryRunWhenBothEndpointsEmpty(t *testing.T) {
	handler := http.NewServeMux()
	handler.Handle("/rest/2.0/workflowDefinitions/workflowDefinition/"+workflowID+"/startFormData",
		formDataHandler(t, nil))
	handler.Handle("/rest/2.0/workflowDefinitions/workflowDefinition/"+workflowID+"/configurationStartFormData",
		formDataHandler(t, nil))

	server := httptest.NewServer(handler)
	defer server.Close()

	client := testutil.NewClient(server)
	out, err := tools.NewTool(client).Handler(t.Context(), tools.Input{
		WorkflowDefinitionID: workflowID,
		DryRun:               true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Status != tools.StatusDryRun {
		t.Fatalf("expected status %q, got %q", tools.StatusDryRun, out.Status)
	}
	if len(out.FormProperties) != 0 {
		t.Fatalf("expected zero form properties, got %d", len(out.FormProperties))
	}
	if out.Message == "" {
		t.Fatal("expected a diagnostic message when both endpoints are empty")
	}
}

func TestStartWorkflow_RequiresWorkflowDefinitionID(t *testing.T) {
	client := testutil.NewClient(httptest.NewServer(http.NewServeMux()))
	_, err := tools.NewTool(client).Handler(t.Context(), tools.Input{})
	if err == nil {
		t.Fatal("expected error when workflowDefinitionId is missing")
	}
}

func equalStringSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
