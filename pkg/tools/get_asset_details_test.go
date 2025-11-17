package tools_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/collibra/chip/pkg/chip"
	"github.com/collibra/chip/pkg/clients"
	"github.com/collibra/chip/pkg/tools"
	"github.com/google/uuid"
)

func TestGetAssetDetails(t *testing.T) {
	assetId, _ := uuid.NewUUID()
	server := httptest.NewServer(&testServer{
		"/graphql/knowledgeGraph/v1": JsonHandlerInOut(func(httpRequest *http.Request, request clients.Request) clients.Response {
			return clients.Response{
				Data: &clients.AssetQueryData{
					Assets: []clients.Asset{
						{
							ID:          assetId.String(),
							DisplayName: "My Asset Name",
						},
					},
				},
			}
		}),
	})
	defer server.Close()

	client := newClient(server)

	config := &chip.ToolConfig{
		CollibraUrl: server.URL,
	}
	ctx := context.WithValue(context.Background(), chip.ToolConfigKey, config)

	output, err := tools.NewAssetDetailsTool().ToolHandler(ctx, client, tools.AssetDetailsInput{
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
