package tools_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/collibra/chip/pkg/clients"
	"github.com/collibra/chip/pkg/tools"
	"github.com/google/uuid"
)

func TestListAssetTypes(t *testing.T) {
	assetTypeId, _ := uuid.NewUUID()
	handler := http.NewServeMux()
	handler.Handle("/rest/2.0/assetTypes", JsonHandlerOut(func(httpRequest *http.Request) (int, clients.AssetTypePagedResponse) {
		return http.StatusOK, clients.AssetTypePagedResponse{
			Total:  1,
			Offset: 0,
			Limit:  100,
			Results: []clients.AssetTypeDetails{
				{
					ID:                 assetTypeId.String(),
					Name:               "Data Element",
					Description:        "A data element asset type",
					PublicId:           "DataElement",
					DisplayNameEnabled: true,
					RatingEnabled:      false,
					FinalType:          false,
					System:             false,
					Product:            "Data Governance Center",
				},
			},
		}
	}))

	server := httptest.NewServer(handler)
	defer server.Close()

	client := newClient(server)
	output, err := tools.NewListAssetTypesTool(client).Handler(t.Context(), tools.ListAssetTypesInput{
		Limit: 100,
	})
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if output.Total != 1 {
		t.Fatalf("Expected 1 result, got: %d", output.Total)
	}

	if len(output.AssetTypes) != 1 {
		t.Fatalf("Expected 1 asset type, got: %d", len(output.AssetTypes))
	}

	assetType := output.AssetTypes[0]
	if assetType.Name != "Data Element" {
		t.Fatalf("Expected name 'Data Element', got: '%s'", assetType.Name)
	}

	if assetType.ID != assetTypeId.String() {
		t.Fatalf("Expected ID '%s', got: '%s'", assetTypeId.String(), assetType.ID)
	}

	if assetType.PublicId != "DataElement" {
		t.Fatalf("Expected publicId 'DataElement', got: '%s'", assetType.PublicId)
	}
}
