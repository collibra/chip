package tools_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/collibra/chip/pkg/clients"
	"github.com/collibra/chip/pkg/tools"
	"github.com/google/uuid"
)

func TestKeywordSearch(t *testing.T) {
	assetId, _ := uuid.NewUUID()
	handler := http.NewServeMux()
	handler.Handle("/rest/2.0/search", JsonHandlerInOut(func(httpRequest *http.Request, request clients.SearchRequest) (int, clients.SearchResponse) {
		return http.StatusOK, clients.SearchResponse{
			Total: 1,
			Results: []clients.SearchResult{
				{
					Resource: clients.SearchResource{
						ResourceType: "Asset",
						ID:           assetId.String(),
						Name:         "My Asset Name",
					},
				},
			},
		}
	}))

	server := httptest.NewServer(handler)
	defer server.Close()

	client := newClient(server)
	output, err := tools.NewSearchKeywordTool(client).Handler(t.Context(), tools.SearchKeywordInput{
		Query: "revenue",
	})
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if output.Total != 1 {
		t.Fatalf("No results found")
	}
	expectedAnswer := "My Asset Name"
	asset := output.Results[0]
	if asset.Name != expectedAnswer {
		t.Fatalf("Expected answer '%s', got: '%s'", expectedAnswer, asset.Name)
	}
}
