package search_lineage_entities_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	tools "github.com/collibra/chip/pkg/tools/search_lineage_entities"
	"github.com/collibra/chip/pkg/tools/testutil"
)

func TestSearchLineageEntities(t *testing.T) {
	handler := http.NewServeMux()
	handler.Handle("/technical_lineage_resource/rest/lineageGraphRead/v1/entities", testutil.JsonHandlerOut(func(r *http.Request) (int, map[string]any) {
		return http.StatusOK, map[string]any{
			"results": []map[string]any{
				{
					"id":   "entity-1",
					"name": "sales_table",
					"type": "table",
				},
			},
			"nextCursor": "cursor-abc",
		}
	}))

	server := httptest.NewServer(handler)
	defer server.Close()

	client := testutil.NewClient(server)
	output, err := tools.NewTool(client).Handler(t.Context(), tools.Input{
		NameContains: "sales",
	})
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if len(output.Results) != 1 {
		t.Fatalf("Expected 1 result, got: %d", len(output.Results))
	}

	entity := output.Results[0]
	if entity.Id != "entity-1" {
		t.Fatalf("Expected entity ID 'entity-1', got: '%s'", entity.Id)
	}

	if entity.Name != "sales_table" {
		t.Fatalf("Expected entity name 'sales_table', got: '%s'", entity.Name)
	}

	if entity.Type != "table" {
		t.Fatalf("Expected entity type 'table', got: '%s'", entity.Type)
	}

	if output.Pagination == nil || output.Pagination.NextCursor != "cursor-abc" {
		t.Fatalf("Expected nextCursor 'cursor-abc'")
	}
}

func TestSearchLineageEntitiesNotFound(t *testing.T) {
	handler := http.NewServeMux()
	handler.Handle("/technical_lineage_resource/rest/lineageGraphRead/v1/entities", testutil.JsonHandlerOut(func(r *http.Request) (int, map[string]any) {
		return http.StatusOK, map[string]any{
			"results": []map[string]any{},
		}
	}))

	server := httptest.NewServer(handler)
	defer server.Close()

	client := testutil.NewClient(server)
	output, err := tools.NewTool(client).Handler(t.Context(), tools.Input{
		NameContains: "nonexistent_table",
	})
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if len(output.Results) != 0 {
		t.Fatalf("Expected 0 results, got: %d", len(output.Results))
	}
}
