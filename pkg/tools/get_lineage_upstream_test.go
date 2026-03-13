package tools_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/collibra/chip/pkg/clients"
	"github.com/collibra/chip/pkg/tools"
)

func TestGetLineageUpstream(t *testing.T) {
	handler := http.NewServeMux()
	handler.Handle("/rest/lineage/v1/entities/entity-1/upstream", JsonHandlerOut(func(r *http.Request) (int, map[string]any) {
		return http.StatusOK, map[string]any{
			"relations": []map[string]any{
				{
					"sourceEntityId":    "source-1",
					"targetEntityId":    "entity-1",
					"transformationIds": []string{"transform-1"},
				},
			},
			"pagination": map[string]any{
				"nextCursor": "cursor-abc",
			},
		}
	}))

	server := httptest.NewServer(handler)
	defer server.Close()

	client := newClient(server)
	output, err := tools.NewGetLineageUpstreamTool(client).Handler(t.Context(), tools.GetLineageUpstreamInput{
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

func TestGetLineageUpstreamMissingId(t *testing.T) {
	server := httptest.NewServer(http.NewServeMux())
	defer server.Close()

	client := newClient(server)
	output, err := tools.NewGetLineageUpstreamTool(client).Handler(t.Context(), tools.GetLineageUpstreamInput{})
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if output.Error == "" {
		t.Fatalf("Expected an error message")
	}
}
