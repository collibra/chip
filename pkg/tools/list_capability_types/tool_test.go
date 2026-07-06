package list_capability_types_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/collibra/chip/pkg/clients"
	tools "github.com/collibra/chip/pkg/tools/list_capability_types"
	"github.com/collibra/chip/pkg/tools/testutil"
	"github.com/google/uuid"
)

func TestListCapabilityTypes(t *testing.T) {
	siteID, _ := uuid.NewUUID()

	handler := http.NewServeMux()
	handler.Handle("GET /edge/api/rest/v2/sites/"+siteID.String()+"/capabilityTypes", testutil.JsonHandlerOut(func(r *http.Request) (int, []clients.CapabilityType) {
		return http.StatusOK, []clients.CapabilityType{{ID: "jdbc-ingestion"}}
	}))
	handler.Handle("GET /edge/api/rest/v2/sites/"+siteID.String()+"/connectionTypes", testutil.JsonHandlerOut(func(r *http.Request) (int, []clients.ConnectionType) {
		return http.StatusOK, []clients.ConnectionType{{ID: "Generic"}}
	}))

	server := httptest.NewServer(handler)
	defer server.Close()

	client := testutil.NewClient(server)
	output, err := tools.NewTool(client).Handler(t.Context(), tools.Input{EdgeSiteID: siteID.String()})
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if len(output.CapabilityTypes) != 1 || output.CapabilityTypes[0].ID != "jdbc-ingestion" {
		t.Fatalf("unexpected capability types: %+v", output.CapabilityTypes)
	}
	if len(output.ConnectionTypes) != 1 || output.ConnectionTypes[0].ID != "Generic" {
		t.Fatalf("unexpected connection types: %+v", output.ConnectionTypes)
	}
}

func TestListCapabilityTypes_InvalidEdgeSiteID(t *testing.T) {
	client := testutil.NewClient(httptest.NewServer(http.NewServeMux()))
	_, err := tools.NewTool(client).Handler(t.Context(), tools.Input{EdgeSiteID: "not-a-uuid"})
	if err == nil {
		t.Fatalf("expected an error for invalid edgeSiteId")
	}
}
