package tools_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/collibra/chip/pkg/clients"
	"github.com/collibra/chip/pkg/tools"
)

func TestGetLineageEntity(t *testing.T) {
	handler := http.NewServeMux()
	handler.Handle("/technical_lineage_resource/rest/lineageGraphRead/v1/entities/entity-1", JsonHandlerOut(func(r *http.Request) (int, clients.LineageEntity) {
		return http.StatusOK, clients.LineageEntity{
			Id:   "entity-1",
			Name: "my_table",
			Type: "table",
		}
	}))

	server := httptest.NewServer(handler)
	defer server.Close()

	client := newClient(server)
	output, err := tools.NewGetLineageEntityTool(client).Handler(t.Context(), tools.GetLineageEntityInput{
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

func TestGetLineageEntityMissingId(t *testing.T) {
	server := httptest.NewServer(http.NewServeMux())
	defer server.Close()

	client := newClient(server)
	output, err := tools.NewGetLineageEntityTool(client).Handler(t.Context(), tools.GetLineageEntityInput{})
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
