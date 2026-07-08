package edge_get_capability_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/collibra/chip/pkg/clients"
	tools "github.com/collibra/chip/pkg/tools/edge_get_capability"
	"github.com/collibra/chip/pkg/tools/testutil"
)

func TestEdgeGetCapability(t *testing.T) {
	handler := http.NewServeMux()
	handler.Handle("/edge/api/rest/v2/capabilities/00000000-0000-0000-0000-000000000001", testutil.JsonHandlerOut(func(r *http.Request) (int, clients.EdgeCapability) {
		return http.StatusOK, clients.EdgeCapability{Id: "00000000-0000-0000-0000-000000000001", Name: "Test Cap"}
	}))

	server := httptest.NewServer(handler)
	defer server.Close()

	output, err := tools.NewTool(testutil.NewClient(server)).Handler(t.Context(), tools.Input{
		Id: "00000000-0000-0000-0000-000000000001",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !output.Found {
		t.Fatalf("expected found, got error: %s", output.Error)
	}
	if output.Capability == nil || output.Capability.Name != "Test Cap" {
		t.Fatal("expected capability name Test Cap")
	}
}

func TestEdgeGetCapabilityNotFound(t *testing.T) {
	handler := http.NewServeMux()
	handler.Handle("/edge/api/rest/v2/capabilities/00000000-0000-0000-0000-000000000002", testutil.JsonHandlerOut(func(r *http.Request) (int, string) {
		return http.StatusNotFound, "not found"
	}))

	server := httptest.NewServer(handler)
	defer server.Close()

	output, err := tools.NewTool(testutil.NewClient(server)).Handler(t.Context(), tools.Input{
		Id: "00000000-0000-0000-0000-000000000002",
	})
	if err != nil {
		t.Fatalf("unexpected go error: %v", err)
	}
	if output.Found {
		t.Fatal("expected not found")
	}
	if output.Error == "" {
		t.Fatal("expected error message")
	}
}

func TestEdgeGetCapabilityInvalidId(t *testing.T) {
	server := httptest.NewServer(http.NewServeMux())
	defer server.Close()

	_, err := tools.NewTool(testutil.NewClient(server)).Handler(t.Context(), tools.Input{Id: "bad-uuid"})
	if err == nil {
		t.Fatal("expected validation error")
	}
}
