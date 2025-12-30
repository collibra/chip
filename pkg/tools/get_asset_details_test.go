package tools_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/collibra/chip/pkg/clients"
	"github.com/collibra/chip/pkg/tools"
	"github.com/google/uuid"
)

func TestGetAssetDetails(t *testing.T) {
	assetId, _ := uuid.NewUUID()
	handler := http.NewServeMux()
	handler.Handle("/graphql/knowledgeGraph/v1", JsonHandlerInOut(func(httpRequest *http.Request, request clients.Request) (int, clients.Response) {
		return http.StatusOK, clients.Response{
			Data: &clients.AssetQueryData{
				Assets: []clients.Asset{
					{
						ID:          assetId.String(),
						DisplayName: "My Asset Name",
					},
				},
			},
		}
	}))
	server := httptest.NewServer(handler)
	defer server.Close()

	client := newClient(server)

	output, err := tools.NewAssetDetailsTool(client).Handler(t.Context(), tools.AssetDetailsInput{
		AssetID: assetId.String(),
	})
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if !output.Found {
		t.Fatalf("Asset not found")
	}
	expectedAnswer := "My Asset Name"
	if output.Asset.DisplayName != expectedAnswer {
		t.Fatalf("Expected answer '%s', got: '%s'", expectedAnswer, output.Asset.DisplayName)
	}
}
