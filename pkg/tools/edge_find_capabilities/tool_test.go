package edge_find_capabilities_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/collibra/chip/pkg/clients"
	tools "github.com/collibra/chip/pkg/tools/edge_find_capabilities"
	"github.com/collibra/chip/pkg/tools/testutil"
)

func TestEdgeFindCapabilities(t *testing.T) {
	handler := http.NewServeMux()
	handler.Handle("/edge/api/rest/v2/capabilities/find", testutil.JsonHandlerInOut(func(r *http.Request, in clients.CapabilityFindRequest) (int, []clients.EdgeCapability) {
		return http.StatusOK, []clients.EdgeCapability{
			{Id: "cap-1", Name: "Databricks Cap", EdgeSiteId: in.EdgeSiteId},
		}
	}))

	server := httptest.NewServer(handler)
	defer server.Close()

	output, err := tools.NewTool(testutil.NewClient(server)).Handler(t.Context(), tools.Input{
		EdgeSiteId: "00000000-0000-0000-0000-000000000001",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if output.Error != "" {
		t.Fatalf("unexpected output error: %s", output.Error)
	}
	if len(output.Capabilities) != 1 || output.Capabilities[0].Id != "cap-1" {
		t.Fatalf("expected cap-1, got %+v", output.Capabilities)
	}
}

func TestEdgeFindCapabilitiesAPIError(t *testing.T) {
	handler := http.NewServeMux()
	handler.Handle("/edge/api/rest/v2/capabilities/find", testutil.JsonHandlerOut(func(r *http.Request) (int, string) {
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

func TestEdgeFindCapabilitiesInvalidSiteId(t *testing.T) {
	server := httptest.NewServer(http.NewServeMux())
	defer server.Close()

	_, err := tools.NewTool(testutil.NewClient(server)).Handler(t.Context(), tools.Input{
		EdgeSiteId: "not-a-uuid",
	})
	if err == nil {
		t.Fatal("expected validation error")
	}
}
