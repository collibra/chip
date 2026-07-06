package list_edge_sites_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/collibra/chip/pkg/clients"
	tools "github.com/collibra/chip/pkg/tools/list_edge_sites"
	"github.com/collibra/chip/pkg/tools/testutil"
	"github.com/google/uuid"
)

func TestListEdgeSites(t *testing.T) {
	siteID, _ := uuid.NewUUID()

	handler := http.NewServeMux()
	handler.Handle("GET /edge/api/rest/v2/sites", testutil.JsonHandlerOut(func(r *http.Request) (int, []clients.EdgeSite) {
		return http.StatusOK, []clients.EdgeSite{
			{ID: siteID.String(), Name: "local-dev", Status: "HEALTHY"},
		}
	}))

	server := httptest.NewServer(handler)
	defer server.Close()

	client := testutil.NewClient(server)
	output, err := tools.NewTool(client).Handler(t.Context(), tools.Input{})
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if len(output.Sites) != 1 {
		t.Fatalf("expected 1 site, got: %d", len(output.Sites))
	}
	if output.Sites[0].Name != "local-dev" {
		t.Fatalf("expected name 'local-dev', got: '%s'", output.Sites[0].Name)
	}
}
