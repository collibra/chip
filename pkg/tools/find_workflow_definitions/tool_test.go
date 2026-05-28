package find_workflow_definitions_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/collibra/chip/pkg/clients"
	tools "github.com/collibra/chip/pkg/tools/find_workflow_definitions"
	"github.com/collibra/chip/pkg/tools/testutil"
	"github.com/google/uuid"
)

func TestFindWorkflowDefinitionsByName(t *testing.T) {
	workflowID, _ := uuid.NewUUID()

	var capturedQuery string
	handler := http.NewServeMux()
	handler.Handle("/rest/2.0/workflowDefinitions", testutil.JsonHandlerOut(func(r *http.Request) (int, clients.WorkflowDefinitionPagedResponse) {
		capturedQuery = r.URL.RawQuery
		return http.StatusOK, clients.WorkflowDefinitionPagedResponse{
			Total:  1,
			Offset: 0,
			Limit:  100,
			Results: []clients.WorkflowDefinition{
				{
					ID:                        workflowID.String(),
					Name:                      "Approve Asset",
					Description:               "Approval workflow",
					ProcessID:                 "approveAsset",
					Enabled:                   true,
					FormRequired:              true,
					BusinessItemDiscriminator: "ASSET",
				},
			},
		}
	}))

	server := httptest.NewServer(handler)
	defer server.Close()

	client := testutil.NewClient(server)
	enabled := true
	output, err := tools.NewTool(client).Handler(t.Context(), tools.Input{
		Name:    "Approve",
		Enabled: &enabled,
	})
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if got, want := capturedQuery, "enabled=true&limit=100&name=Approve"; got != want {
		t.Fatalf("Expected query %q, got: %q", want, got)
	}

	if output.Total != 1 {
		t.Fatalf("Expected total 1, got: %d", output.Total)
	}
	if len(output.Definitions) != 1 {
		t.Fatalf("Expected 1 definition, got: %d", len(output.Definitions))
	}

	def := output.Definitions[0]
	if def.ID != workflowID.String() {
		t.Fatalf("Expected ID %q, got: %q", workflowID.String(), def.ID)
	}
	if def.Name != "Approve Asset" {
		t.Fatalf("Expected name 'Approve Asset', got: %q", def.Name)
	}
	if !def.Enabled {
		t.Fatal("Expected enabled=true")
	}
	if !def.FormRequired {
		t.Fatal("Expected formRequired=true")
	}
	if def.BusinessItemDiscriminator != "ASSET" {
		t.Fatalf("Expected businessItemDiscriminator 'ASSET', got: %q", def.BusinessItemDiscriminator)
	}
}

func TestFindWorkflowDefinitionsDefaultLimit(t *testing.T) {
	var capturedQuery string
	handler := http.NewServeMux()
	handler.Handle("/rest/2.0/workflowDefinitions", testutil.JsonHandlerOut(func(r *http.Request) (int, clients.WorkflowDefinitionPagedResponse) {
		capturedQuery = r.URL.RawQuery
		return http.StatusOK, clients.WorkflowDefinitionPagedResponse{Limit: 100}
	}))

	server := httptest.NewServer(handler)
	defer server.Close()

	client := testutil.NewClient(server)
	_, err := tools.NewTool(client).Handler(t.Context(), tools.Input{})
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if capturedQuery != "limit=100" {
		t.Fatalf("Expected default limit=100 query, got: %q", capturedQuery)
	}
}
