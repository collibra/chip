package tools_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/collibra/chip/pkg/clients"
	"github.com/collibra/chip/pkg/tools"
)

func TestGetLineageDownstream(t *testing.T) {
	handler := http.NewServeMux()
	handler.Handle("/technical_lineage_resource/rest/lineageGraphRead/v1/entities/entity-1/downstream", JsonHandlerOut(func(r *http.Request) (int, map[string]any) {
		return http.StatusOK, map[string]any{
			"relations": []map[string]any{
				{
					"sourceEntityId":    "entity-1",
					"targetEntityId":    "target-1",
					"transformationIds": []string{"transform-2"},
				},
			},
			"nextCursor": "cursor-xyz",
		}
	}))

	server := httptest.NewServer(handler)
	defer server.Close()

	client := newClient(server)
	output, err := tools.NewGetLineageDownstreamTool(client).Handler(t.Context(), tools.GetLineageDownstreamInput{
		EntityId: "entity-1",
	})
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if output.Error != "" {
		t.Fatalf("Expected no error in output, got: %s", output.Error)
	}

	if output.EntityId != "entity-1" {
		t.Fatalf("Expected entityId 'entity-1', got: '%s'", output.EntityId)
	}

	if output.Direction != string(clients.LineageDirectionDownstream) {
		t.Fatalf("Expected direction 'downstream', got: '%s'", output.Direction)
	}

	if len(output.Relations) != 1 {
		t.Fatalf("Expected 1 relation, got: %d", len(output.Relations))
	}

	relation := output.Relations[0]
	if relation.TargetEntityId != "target-1" {
		t.Fatalf("Expected targetEntityId 'target-1', got: '%s'", relation.TargetEntityId)
	}

	if output.Pagination == nil || output.Pagination.NextCursor != "cursor-xyz" {
		t.Fatalf("Expected nextCursor 'cursor-xyz'")
	}
}

func TestGetLineageDownstreamNotFound(t *testing.T) {
	handler := http.NewServeMux()
	handler.Handle("/technical_lineage_resource/rest/lineageGraphRead/v1/entities/entity-unknown/downstream", JsonHandlerOut(func(r *http.Request) (int, string) {
		return http.StatusNotFound, "entity not found"
	}))

	server := httptest.NewServer(handler)
	defer server.Close()

	client := newClient(server)
	output, err := tools.NewGetLineageDownstreamTool(client).Handler(t.Context(), tools.GetLineageDownstreamInput{
		EntityId: "entity-unknown",
	})
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if output.Error == "" {
		t.Fatalf("Expected an error message")
	}

	if output.Relations == nil {
		t.Fatalf("Expected Relations to be a non-nil slice, got nil")
	}
}

func TestGetLineageDownstreamMissingId(t *testing.T) {
	server := httptest.NewServer(http.NewServeMux())
	defer server.Close()

	client := newClient(server)
	output, err := tools.NewGetLineageDownstreamTool(client).Handler(t.Context(), tools.GetLineageDownstreamInput{})
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if output.Error == "" {
		t.Fatalf("Expected an error message")
	}
}
