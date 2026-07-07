package edge_get_connection_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/collibra/chip/pkg/clients"
	tools "github.com/collibra/chip/pkg/tools/edge_get_connection"
	"github.com/collibra/chip/pkg/tools/testutil"
)

func TestEdgeGetConnection(t *testing.T) {
	handler := http.NewServeMux()
	handler.Handle("/edge/api/rest/v2/connections/00000000-0000-0000-0000-000000000001", testutil.JsonHandlerOut(func(r *http.Request) (int, clients.EdgeConnection) {
		return http.StatusOK, clients.EdgeConnection{Id: "00000000-0000-0000-0000-000000000001", Name: "Test Conn"}
	}))

	server := httptest.NewServer(handler)
	defer server.Close()

	output, err := tools.NewTool(testutil.NewClient(server)).Handler(t.Context(), tools.Input{
		ConnectionId: "00000000-0000-0000-0000-000000000001",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !output.Found {
		t.Fatalf("expected found, got error: %s", output.Error)
	}
	if output.Connection.Name != "Test Conn" {
		t.Fatalf("expected name Test Conn, got %s", output.Connection.Name)
	}
}

func TestEdgeGetConnectionNotFound(t *testing.T) {
	handler := http.NewServeMux()
	handler.Handle("/edge/api/rest/v2/connections/00000000-0000-0000-0000-000000000002", testutil.JsonHandlerOut(func(r *http.Request) (int, string) {
		return http.StatusNotFound, "not found"
	}))

	server := httptest.NewServer(handler)
	defer server.Close()

	output, err := tools.NewTool(testutil.NewClient(server)).Handler(t.Context(), tools.Input{
		ConnectionId: "00000000-0000-0000-0000-000000000002",
	})
	if err != nil {
		t.Fatalf("unexpected go error: %v", err)
	}
	if output.Found {
		t.Fatal("expected not found")
	}
}

func TestEdgeGetConnectionInvalidId(t *testing.T) {
	server := httptest.NewServer(http.NewServeMux())
	defer server.Close()

	_, err := tools.NewTool(testutil.NewClient(server)).Handler(t.Context(), tools.Input{ConnectionId: "bad"})
	if err == nil {
		t.Fatal("expected validation error")
	}
}
