package search_assets_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/collibra/chip/pkg/clients"
	"github.com/collibra/chip/pkg/tools/search_assets"
	"github.com/collibra/chip/pkg/tools/testutil"
)

func TestSearchAssets_ByName(t *testing.T) {
	handler := http.NewServeMux()
	handler.Handle("/rest/2.0/assets", testutil.JsonHandlerOut(
		func(r *http.Request) (int, clients.SearchAssetsResponse) {
			name := r.URL.Query().Get("name")
			if name != "Customer" {
				t.Errorf("expected name=Customer, got %q", name)
			}
			sortField := r.URL.Query().Get("sortField")
			if sortField != "NAME" {
				t.Errorf("expected sortField=NAME, got %q", sortField)
			}
			return http.StatusOK, clients.SearchAssetsResponse{
				Total:  1,
				Offset: 0,
				Limit:  1000,
				Results: []clients.SearchAssetsResult{
					{
						ID:          "asset-1",
						Name:        "Customer",
						DisplayName: "Customer",
						DomainID:    "domain-1",
						TypeID:      "type-1",
						Status:      clients.SearchAssetsStatusField{Name: "Accepted"},
					},
				},
			}
		},
	))

	server := httptest.NewServer(handler)
	defer server.Close()

	client := testutil.NewClient(server)
	output, err := search_assets.NewTool(client).Handler(t.Context(), search_assets.Input{
		Name: "Customer",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if output.Total != 1 {
		t.Errorf("got total %d, want 1", output.Total)
	}
	if len(output.Results) != 1 {
		t.Fatalf("got %d results, want 1", len(output.Results))
	}
	if output.Results[0].ID != "asset-1" {
		t.Errorf("got ID %q, want %q", output.Results[0].ID, "asset-1")
	}
	if output.Results[0].Name != "Customer" {
		t.Errorf("got Name %q, want %q", output.Results[0].Name, "Customer")
	}
	if output.Results[0].DomainID != "domain-1" {
		t.Errorf("got DomainID %q, want %q", output.Results[0].DomainID, "domain-1")
	}
	if output.Results[0].TypeID != "type-1" {
		t.Errorf("got TypeID %q, want %q", output.Results[0].TypeID, "type-1")
	}
	if output.Results[0].Status != "Accepted" {
		t.Errorf("got Status %q, want %q", output.Results[0].Status, "Accepted")
	}
}

func TestSearchAssets_ByDomainAndType(t *testing.T) {
	handler := http.NewServeMux()
	handler.Handle("/rest/2.0/assets", testutil.JsonHandlerOut(
		func(r *http.Request) (int, clients.SearchAssetsResponse) {
			domainID := r.URL.Query().Get("domainId")
			if domainID != "domain-abc" {
				t.Errorf("expected domainId=domain-abc, got %q", domainID)
			}
			typeIDs := r.URL.Query()["typeIds"]
			if len(typeIDs) != 2 || typeIDs[0] != "type-1" || typeIDs[1] != "type-2" {
				t.Errorf("expected typeIds=[type-1, type-2], got %v", typeIDs)
			}
			return http.StatusOK, clients.SearchAssetsResponse{
				Total:  2,
				Offset: 0,
				Limit:  1000,
				Results: []clients.SearchAssetsResult{
					{ID: "asset-1", Name: "Asset One", DisplayName: "Asset One", DomainID: "domain-abc", TypeID: "type-1", Status: clients.SearchAssetsStatusField{Name: "Active"}},
					{ID: "asset-2", Name: "Asset Two", DisplayName: "Asset Two", DomainID: "domain-abc", TypeID: "type-2", Status: clients.SearchAssetsStatusField{Name: "Active"}},
				},
			}
		},
	))

	server := httptest.NewServer(handler)
	defer server.Close()

	client := testutil.NewClient(server)
	output, err := search_assets.NewTool(client).Handler(t.Context(), search_assets.Input{
		DomainID: "domain-abc",
		TypeIDs:  []string{"type-1", "type-2"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if output.Total != 2 {
		t.Errorf("got total %d, want 2", output.Total)
	}
	if len(output.Results) != 2 {
		t.Fatalf("got %d results, want 2", len(output.Results))
	}
	if output.Results[0].ID != "asset-1" {
		t.Errorf("got first result ID %q, want %q", output.Results[0].ID, "asset-1")
	}
	if output.Results[1].ID != "asset-2" {
		t.Errorf("got second result ID %q, want %q", output.Results[1].ID, "asset-2")
	}
}

func TestSearchAssets_Pagination(t *testing.T) {
	handler := http.NewServeMux()
	handler.Handle("/rest/2.0/assets", testutil.JsonHandlerOut(
		func(r *http.Request) (int, clients.SearchAssetsResponse) {
			offset := r.URL.Query().Get("offset")
			limit := r.URL.Query().Get("limit")
			if offset != "10" {
				t.Errorf("expected offset=10, got %q", offset)
			}
			if limit != "5" {
				t.Errorf("expected limit=5, got %q", limit)
			}
			return http.StatusOK, clients.SearchAssetsResponse{
				Total:  50,
				Offset: 10,
				Limit:  5,
				Results: []clients.SearchAssetsResult{
					{ID: "asset-11", Name: "Asset 11", DisplayName: "Asset 11", DomainID: "d1", TypeID: "t1", Status: clients.SearchAssetsStatusField{Name: "Active"}},
					{ID: "asset-12", Name: "Asset 12", DisplayName: "Asset 12", DomainID: "d1", TypeID: "t1", Status: clients.SearchAssetsStatusField{Name: "Active"}},
				},
			}
		},
	))

	server := httptest.NewServer(handler)
	defer server.Close()

	client := testutil.NewClient(server)
	output, err := search_assets.NewTool(client).Handler(t.Context(), search_assets.Input{
		Offset: 10,
		Limit:  5,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if output.Total != 50 {
		t.Errorf("got total %d, want 50", output.Total)
	}
	if len(output.Results) != 2 {
		t.Errorf("got %d results, want 2", len(output.Results))
	}
}

func TestSearchAssets_EmptyResults(t *testing.T) {
	handler := http.NewServeMux()
	handler.Handle("/rest/2.0/assets", testutil.JsonHandlerOut(
		func(_ *http.Request) (int, clients.SearchAssetsResponse) {
			return http.StatusOK, clients.SearchAssetsResponse{
				Total:   0,
				Offset:  0,
				Limit:   1000,
				Results: []clients.SearchAssetsResult{},
			}
		},
	))

	server := httptest.NewServer(handler)
	defer server.Close()

	client := testutil.NewClient(server)
	output, err := search_assets.NewTool(client).Handler(t.Context(), search_assets.Input{
		Name: "NonExistentAsset",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if output.Total != 0 {
		t.Errorf("got total %d, want 0", output.Total)
	}
	if len(output.Results) != 0 {
		t.Errorf("got %d results, want 0", len(output.Results))
	}
}

func TestSearchAssets_NameMatchMode(t *testing.T) {
	handler := http.NewServeMux()
	handler.Handle("/rest/2.0/assets", testutil.JsonHandlerOut(
		func(r *http.Request) (int, clients.SearchAssetsResponse) {
			matchMode := r.URL.Query().Get("nameMatchMode")
			if matchMode != "ANYWHERE" {
				t.Errorf("expected nameMatchMode=ANYWHERE, got %q", matchMode)
			}
			return http.StatusOK, clients.SearchAssetsResponse{
				Total:  1,
				Offset: 0,
				Limit:  1000,
				Results: []clients.SearchAssetsResult{
					{ID: "asset-1", Name: "My Customer Data", DisplayName: "My Customer Data", DomainID: "d1", TypeID: "t1", Status: clients.SearchAssetsStatusField{Name: "Active"}},
				},
			}
		},
	))

	server := httptest.NewServer(handler)
	defer server.Close()

	client := testutil.NewClient(server)
	output, err := search_assets.NewTool(client).Handler(t.Context(), search_assets.Input{
		Name:          "Customer",
		NameMatchMode: "ANYWHERE",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if output.Total != 1 {
		t.Errorf("got total %d, want 1", output.Total)
	}
	if output.Results[0].Name != "My Customer Data" {
		t.Errorf("got Name %q, want %q", output.Results[0].Name, "My Customer Data")
	}
}

func TestSearchAssets_ServerError(t *testing.T) {
	handler := http.NewServeMux()
	handler.Handle("/rest/2.0/assets", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))

	server := httptest.NewServer(handler)
	defer server.Close()

	client := testutil.NewClient(server)
	_, err := search_assets.NewTool(client).Handler(t.Context(), search_assets.Input{
		Name: "Test",
	})
	if err == nil {
		t.Fatal("expected error for server error response, got nil")
	}
}

func TestSearchAssets_ToolMetadata(t *testing.T) {
	tool := search_assets.NewTool(http.DefaultClient)

	if tool.Name != "search_assets" {
		t.Errorf("got tool name %q, want %q", tool.Name, "search_assets")
	}
	if tool.Description == "" {
		t.Error("tool description should not be empty")
	}
	if len(tool.Permissions) != 1 || tool.Permissions[0] != "dgc.ai-copilot" {
		t.Errorf("got permissions %v, want [dgc.ai-copilot]", tool.Permissions)
	}
}
