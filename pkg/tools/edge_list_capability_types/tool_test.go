package edge_list_capability_types_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/collibra/chip/pkg/clients"
	tools "github.com/collibra/chip/pkg/tools/edge_list_capability_types"
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

func TestListCapabilityTypes_NoQueryStripsManifests(t *testing.T) {
	siteID, _ := uuid.NewUUID()

	handler := http.NewServeMux()
	handler.Handle("GET /edge/api/rest/v2/sites/"+siteID.String()+"/capabilityTypes", testutil.JsonHandlerOut(func(r *http.Request) (int, []clients.CapabilityType) {
		return http.StatusOK, []clients.CapabilityType{
			{ID: "jdbc-ingestion", Manifest: map[string]any{"huge": "manifest"}},
			{ID: "snowflake-synchronization", Manifest: map[string]any{"huge": "manifest"}},
		}
	}))
	handler.Handle("GET /edge/api/rest/v2/sites/"+siteID.String()+"/connectionTypes", testutil.JsonHandlerOut(func(r *http.Request) (int, []clients.ConnectionType) {
		return http.StatusOK, []clients.ConnectionType{{ID: "Generic", Manifest: map[string]any{"huge": "manifest"}}}
	}))

	server := httptest.NewServer(handler)
	defer server.Close()

	client := testutil.NewClient(server)
	output, err := tools.NewTool(client).Handler(t.Context(), tools.Input{EdgeSiteID: siteID.String()})
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if len(output.CapabilityTypes) != 2 {
		t.Fatalf("expected 2 capability types, got: %+v", output.CapabilityTypes)
	}
	for _, c := range output.CapabilityTypes {
		if c.Manifest != nil {
			t.Fatalf("expected manifest to be stripped without a query, got: %+v", c)
		}
	}
}

func TestListCapabilityTypes_QueryFiltersAndKeepsManifest(t *testing.T) {
	siteID, _ := uuid.NewUUID()

	handler := http.NewServeMux()
	handler.Handle("GET /edge/api/rest/v2/sites/"+siteID.String()+"/capabilityTypes", testutil.JsonHandlerOut(func(r *http.Request) (int, []clients.CapabilityType) {
		return http.StatusOK, []clients.CapabilityType{
			{ID: "jdbc-ingestion", Manifest: map[string]any{"connection": "type"}},
			{ID: "snowflake-synchronization", Manifest: map[string]any{"other": "type"}},
		}
	}))
	handler.Handle("GET /edge/api/rest/v2/sites/"+siteID.String()+"/connectionTypes", testutil.JsonHandlerOut(func(r *http.Request) (int, []clients.ConnectionType) {
		return http.StatusOK, []clients.ConnectionType{{ID: "Generic"}}
	}))

	server := httptest.NewServer(handler)
	defer server.Close()

	client := testutil.NewClient(server)
	output, err := tools.NewTool(client).Handler(t.Context(), tools.Input{EdgeSiteID: siteID.String(), Query: "jdbc"})
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if len(output.CapabilityTypes) != 1 || output.CapabilityTypes[0].ID != "jdbc-ingestion" {
		t.Fatalf("expected only jdbc-ingestion to match, got: %+v", output.CapabilityTypes)
	}
	if output.CapabilityTypes[0].Manifest == nil {
		t.Fatalf("expected manifest to be kept when query filters results")
	}
	if len(output.ConnectionTypes) != 0 {
		t.Fatalf("expected no connection types to match 'jdbc', got: %+v", output.ConnectionTypes)
	}
	if output.ConnectionTypes == nil {
		t.Fatalf("expected an empty slice, not nil — a nil slice marshals to JSON null, which fails the tool's array-typed output schema")
	}
}

func TestListCapabilityTypes_InvalidEdgeSiteID(t *testing.T) {
	client := testutil.NewClient(httptest.NewServer(http.NewServeMux()))
	_, err := tools.NewTool(client).Handler(t.Context(), tools.Input{EdgeSiteID: "not-a-uuid"})
	if err == nil {
		t.Fatalf("expected an error for invalid edgeSiteId")
	}
}
