package get_lineage_entity_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/collibra/chip/pkg/clients"
	tools "github.com/collibra/chip/pkg/tools/get_lineage_entity"
	"github.com/collibra/chip/pkg/tools/testutil"
)

func TestGetLineageEntity(t *testing.T) {
	handler := http.NewServeMux()
	handler.Handle("/technical_lineage_resource/rest/lineageGraphRead/v1/entities/entity-1", testutil.JsonHandlerOut(func(r *http.Request) (int, clients.LineageEntity) {
		return http.StatusOK, clients.LineageEntity{
			Id:   "entity-1",
			Name: "my_table",
			Type: "table",
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

	if !output.Found {
		t.Fatalf("Expected entity to be found")
	}

	if output.Entity.Id != "entity-1" {
		t.Fatalf("Expected entity ID 'entity-1', got: '%s'", output.Entity.Id)
	}

	if output.Entity.Name != "my_table" {
		t.Fatalf("Expected entity name 'my_table', got: '%s'", output.Entity.Name)
	}

	if output.Entity.Type != "table" {
		t.Fatalf("Expected entity type 'table', got: '%s'", output.Entity.Type)
	}
}

func TestGetLineageEntityNotFound(t *testing.T) {
	handler := http.NewServeMux()
	handler.Handle("/technical_lineage_resource/rest/lineageGraphRead/v1/entities/entity-unknown", testutil.JsonHandlerOut(func(r *http.Request) (int, string) {
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

	if output.Found {
		t.Fatalf("Expected entity not to be found")
	}

	if output.Error == "" {
		t.Fatalf("Expected an error message")
	}
}

func TestGetLineageEntityMissingId(t *testing.T) {
	server := httptest.NewServer(http.NewServeMux())
	defer server.Close()

	client := testutil.NewClient(server)
	output, err := tools.NewTool(client).Handler(t.Context(), tools.Input{})
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if output.Found {
		t.Fatalf("Expected entity not to be found")
	}

	if output.Error == "" {
		t.Fatalf("Expected an error message")
	}
}
