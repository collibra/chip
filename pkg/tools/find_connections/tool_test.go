package find_connections_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/collibra/chip/pkg/clients"
	tools "github.com/collibra/chip/pkg/tools/find_connections"
	"github.com/collibra/chip/pkg/tools/testutil"
	"github.com/google/uuid"
)

func TestFindConnections(t *testing.T) {
	connID, _ := uuid.NewUUID()
	siteID, _ := uuid.NewUUID()

	handler := http.NewServeMux()
	handler.Handle("POST /edge/api/rest/v2/connections/find", testutil.JsonHandlerInOut(func(r *http.Request, in clients.ConnectionFindRequest) (int, []clients.EdgeConnection) {
		if in.Name != "snowflake-manual" {
			t.Fatalf("unexpected name: %s", in.Name)
		}
		return http.StatusOK, []clients.EdgeConnection{
			{Id: connID.String(), Name: "snowflake-manual", EdgeSiteId: siteID.String()},
		}
	}))

	server := httptest.NewServer(handler)
	defer server.Close()

	client := testutil.NewClient(server)
	output, err := tools.NewTool(client).Handler(t.Context(), tools.Input{Name: "snowflake-manual"})
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if len(output.Connections) != 1 || output.Connections[0].Id != connID.String() {
		t.Fatalf("unexpected output: %+v", output)
	}
}

func TestFindConnections_InvalidEdgeSiteID(t *testing.T) {
	client := testutil.NewClient(httptest.NewServer(http.NewServeMux()))
	_, err := tools.NewTool(client).Handler(t.Context(), tools.Input{EdgeSiteID: "not-a-uuid"})
	if err == nil {
		t.Fatalf("expected an error for invalid edgeSiteId")
	}
}
