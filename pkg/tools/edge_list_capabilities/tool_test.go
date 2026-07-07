package edge_list_capabilities_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/collibra/chip/pkg/clients"
	tools "github.com/collibra/chip/pkg/tools/edge_list_capabilities"
	"github.com/collibra/chip/pkg/tools/testutil"
)

var testCapabilities = []clients.EdgeCapability{
	{Id: "cap-1", Name: "vb-databricks-prod", Type: &clients.EdgeCapabilityType{Id: "databricks-edge-capability"}},
	{Id: "cap-2", Name: "vb-databricks-staging", Type: &clients.EdgeCapabilityType{Id: "databricks-edge-capability"}},
	{Id: "cap-3", Name: "finance-dataplex", Type: &clients.EdgeCapabilityType{Id: "dataplex-edge-capability"}},
	{Id: "cap-4", Name: "sales-databricks", Type: &clients.EdgeCapabilityType{Id: "databricks-edge-capability"}},
}

func newServer(t *testing.T) *httptest.Server {
	handler := http.NewServeMux()
	handler.Handle("/edge/api/rest/v2/capabilities", testutil.JsonHandlerOut(func(r *http.Request) (int, []clients.EdgeCapability) {
		return http.StatusOK, testCapabilities
	}))
	return httptest.NewServer(handler)
}

func TestEdgeListCapabilities(t *testing.T) {
	server := newServer(t)
	defer server.Close()

	output, err := tools.NewTool(testutil.NewClient(server)).Handler(t.Context(), tools.Input{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if output.Error != "" {
		t.Fatalf("unexpected output error: %s", output.Error)
	}
	if output.Count != 4 {
		t.Fatalf("expected 4 capabilities, got %d", output.Count)
	}
}

func TestEdgeListCapabilitiesFilterByName(t *testing.T) {
	server := newServer(t)
	defer server.Close()

	output, err := tools.NewTool(testutil.NewClient(server)).Handler(t.Context(), tools.Input{NameContains: "vb"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if output.Count != 2 {
		t.Fatalf("expected 2 capabilities, got %d", output.Count)
	}
}

func TestEdgeListCapabilitiesFilterByType(t *testing.T) {
	server := newServer(t)
	defer server.Close()

	output, err := tools.NewTool(testutil.NewClient(server)).Handler(t.Context(), tools.Input{TypeId: "databricks-edge-capability"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if output.Count != 3 {
		t.Fatalf("expected 3 databricks capabilities, got %d", output.Count)
	}
}

func TestEdgeListCapabilitiesFilterByNameAndType(t *testing.T) {
	server := newServer(t)
	defer server.Close()

	output, err := tools.NewTool(testutil.NewClient(server)).Handler(t.Context(), tools.Input{
		NameContains: "vb",
		TypeId:       "databricks-edge-capability",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if output.Count != 2 {
		t.Fatalf("expected 2 capabilities, got %d", output.Count)
	}
}

func TestEdgeListCapabilitiesFilterByNameCaseInsensitive(t *testing.T) {
	server := newServer(t)
	defer server.Close()

	output, err := tools.NewTool(testutil.NewClient(server)).Handler(t.Context(), tools.Input{NameContains: "VB"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if output.Count != 2 {
		t.Fatalf("expected 2 capabilities (case-insensitive), got %d", output.Count)
	}
}

func TestEdgeListCapabilitiesLimit(t *testing.T) {
	server := newServer(t)
	defer server.Close()

	output, err := tools.NewTool(testutil.NewClient(server)).Handler(t.Context(), tools.Input{Limit: 2})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if output.Count != 2 {
		t.Fatalf("expected 2 capabilities with limit, got %d", output.Count)
	}
}

func TestEdgeListCapabilitiesAPIError(t *testing.T) {
	handler := http.NewServeMux()
	handler.Handle("/edge/api/rest/v2/capabilities", testutil.JsonHandlerOut(func(r *http.Request) (int, string) {
		return http.StatusInternalServerError, "internal server error"
	}))
	server := httptest.NewServer(handler)
	defer server.Close()

	output, err := tools.NewTool(testutil.NewClient(server)).Handler(t.Context(), tools.Input{})
	if err != nil {
		t.Fatalf("unexpected go error: %v", err)
	}
	if output.Error == "" {
		t.Fatal("expected output error to be set")
	}
}

func TestEdgeListCapabilitiesEmpty(t *testing.T) {
	handler := http.NewServeMux()
	handler.Handle("/edge/api/rest/v2/capabilities", testutil.JsonHandlerOut(func(r *http.Request) (int, []clients.EdgeCapability) {
		return http.StatusOK, []clients.EdgeCapability{}
	}))
	server := httptest.NewServer(handler)
	defer server.Close()

	output, err := tools.NewTool(testutil.NewClient(server)).Handler(t.Context(), tools.Input{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if output.Count != 0 {
		t.Fatalf("expected count 0, got %d", output.Count)
	}
}
