package edge_list_connections_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/collibra/chip/pkg/clients"
	tools "github.com/collibra/chip/pkg/tools/edge_list_connections"
	"github.com/collibra/chip/pkg/tools/testutil"
)

func TestEdgeListConnections(t *testing.T) {
	handler := http.NewServeMux()
	handler.Handle("/edge/api/rest/v2/connections", testutil.JsonHandlerOut(func(r *http.Request) (int, []clients.EdgeConnection) {
		return http.StatusOK, []clients.EdgeConnection{
			{Id: "conn-1", Name: "Databricks Conn"},
		}
	}))

	server := httptest.NewServer(handler)
	defer server.Close()

	output, err := tools.NewTool(testutil.NewClient(server)).Handler(t.Context(), tools.Input{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if output.Error != "" {
		t.Fatalf("unexpected output error: %s", output.Error)
	}
	if len(output.Connections) != 1 || output.Connections[0].Id != "conn-1" {
		t.Fatalf("expected conn-1, got %+v", output.Connections)
	}
	if output.Count != 1 {
		t.Fatalf("expected count 1, got %d", output.Count)
	}
}

func TestEdgeListConnectionsAPIError(t *testing.T) {
	handler := http.NewServeMux()
	handler.Handle("/edge/api/rest/v2/connections", testutil.JsonHandlerOut(func(r *http.Request) (int, string) {
		return http.StatusInternalServerError, "error"
	}))

	server := httptest.NewServer(handler)
	defer server.Close()

	output, err := tools.NewTool(testutil.NewClient(server)).Handler(t.Context(), tools.Input{})
	if err != nil {
		t.Fatalf("unexpected go error: %v", err)
	}
	if output.Error == "" {
		t.Fatal("expected output error")
	}
}
