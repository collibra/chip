package get_lineage_upstream_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/collibra/chip/pkg/clients"
	tools "github.com/collibra/chip/pkg/tools/get_lineage_upstream"
	"github.com/collibra/chip/pkg/tools/testutil"
)

func TestGetLineageUpstream(t *testing.T) {
	handler := http.NewServeMux()
	handler.Handle("/technical_lineage_resource/rest/lineageGraphRead/v1/entities/entity-1/upstream", testutil.JsonHandlerOut(func(r *http.Request) (int, map[string]any) {
		return http.StatusOK, map[string]any{
			"relations": []map[string]any{
				{
					"sourceEntityId":    "source-1",
					"targetEntityId":    "entity-1",
					"transformationIds": []string{"transform-1"},
				},
			},
			"nextCursor": "cursor-abc",
		}
	}))

	server := httptest.NewServer(handler)
	defer server.Close()

	client := testutil.NewClient(server)
	output, err := tools.NewTool(client).Handler(t.Context(), tools.Input{
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

	if output.Direction != clients.LineageDirectionUpstream {
		t.Fatalf("Expected direction 'upstream', got: '%s'", output.Direction)
	}

	if len(output.Relations) != 1 {
		t.Fatalf("Expected 1 relation, got: %d", len(output.Relations))
	}

	relation := output.Relations[0]
	if relation.SourceEntityId != "source-1" {
		t.Fatalf("Expected sourceEntityId 'source-1', got: '%s'", relation.SourceEntityId)
	}

	if output.Pagination == nil || output.Pagination.NextCursor != "cursor-abc" {
		t.Fatalf("Expected nextCursor 'cursor-abc'")
	}
}

func TestGetLineageUpstreamNotFound(t *testing.T) {
	handler := http.NewServeMux()
	handler.Handle("/technical_lineage_resource/rest/lineageGraphRead/v1/entities/entity-unknown/upstream", testutil.JsonHandlerOut(func(r *http.Request) (int, string) {
		return http.StatusNotFound, "entity not found"
	}))

	server := httptest.NewServer(handler)
	defer server.Close()

	client := testutil.NewClient(server)
	output, err := tools.NewTool(client).Handler(t.Context(), tools.Input{
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

func TestGetLineageUpstreamMissingId(t *testing.T) {
	server := httptest.NewServer(http.NewServeMux())
	defer server.Close()

	client := testutil.NewClient(server)
	output, err := tools.NewTool(client).Handler(t.Context(), tools.Input{})
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if output.Error == "" {
		t.Fatalf("Expected an error message")
	}
}
