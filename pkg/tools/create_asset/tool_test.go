package create_asset_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/collibra/chip/pkg/clients"
	"github.com/collibra/chip/pkg/tools/create_asset"
	"github.com/collibra/chip/pkg/tools/testutil"
)

func TestCreateAssetSuccess(t *testing.T) {
	handler := http.NewServeMux()
	handler.Handle("POST /rest/2.0/assets", testutil.JsonHandlerInOut(func(_ *http.Request, req clients.CreateAssetRequest) (int, clients.CreateAssetResponse) {
		return http.StatusCreated, clients.CreateAssetResponse{
			ID:          "asset-uuid-123",
			Name:        req.Name,
			DisplayName: req.Name,
			Type: clients.CreateAssetTypeRef{
				ID:   req.TypeID,
				Name: "Business Term",
			},
			Domain: clients.CreateAssetDomainRef{
				ID:   req.DomainID,
				Name: "Test Domain",
			},
		}
	}))

	server := httptest.NewServer(handler)
	defer server.Close()

	client := testutil.NewClient(server)
	output, err := create_asset.NewTool(client).Handler(t.Context(), create_asset.Input{
		Name:        "My New Asset",
		AssetTypeID: "00000000-0000-0000-0000-000000000456",
		DomainID:    "00000000-0000-0000-0000-000000000789",
	})
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if output.AssetID != "asset-uuid-123" {
		t.Errorf("Expected asset ID 'asset-uuid-123', got: '%s'", output.AssetID)
	}
}

func TestCreateAssetWithAttributes(t *testing.T) {
	attributesCreated := 0

	handler := http.NewServeMux()
	handler.Handle("POST /rest/2.0/assets", testutil.JsonHandlerInOut(func(_ *http.Request, req clients.CreateAssetRequest) (int, clients.CreateAssetResponse) {
		return http.StatusCreated, clients.CreateAssetResponse{
			ID:          "asset-uuid-123",
			Name:        req.Name,
			DisplayName: req.Name,
			Type: clients.CreateAssetTypeRef{
				ID:   req.TypeID,
				Name: "Business Term",
			},
			Domain: clients.CreateAssetDomainRef{
				ID:   req.DomainID,
				Name: "Test Domain",
			},
		}
	}))
	handler.Handle("POST /rest/2.0/attributes", testutil.JsonHandlerInOut(func(_ *http.Request, req clients.CreateAttributeRequest) (int, clients.CreateAttributeResponse) {
		attributesCreated++
		return http.StatusCreated, clients.CreateAttributeResponse{
			ID: "attr-uuid-" + req.TypeID,
			Type: clients.CreateAttributeTypeRef{
				ID:   req.TypeID,
				Name: "Description",
			},
			Asset: clients.CreateAttributeAssetRef{
				ID: req.AssetID,
			},
			Value: req.Value,
		}
	}))

	server := httptest.NewServer(handler)
	defer server.Close()

	client := testutil.NewClient(server)
	output, err := create_asset.NewTool(client).Handler(t.Context(), create_asset.Input{
		Name:        "Asset With Attrs",
		AssetTypeID: "00000000-0000-0000-0000-000000000456",
		DomainID:    "00000000-0000-0000-0000-000000000789",
		Attributes: map[string]string{
			"00000000-0000-0000-0000-0000000000a1": "Description value",
			"00000000-0000-0000-0000-0000000000a2": "Definition value",
		},
	})
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if output.AssetID != "asset-uuid-123" {
		t.Errorf("Expected asset ID 'asset-uuid-123', got: '%s'", output.AssetID)
	}

	if attributesCreated != 2 {
		t.Errorf("Expected 2 attributes created, got: %d", attributesCreated)
	}
}

func TestCreateAssetWithDisplayName(t *testing.T) {
	var receivedDisplayName string

	handler := http.NewServeMux()
	handler.Handle("POST /rest/2.0/assets", testutil.JsonHandlerInOut(func(_ *http.Request, req clients.CreateAssetRequest) (int, clients.CreateAssetResponse) {
		receivedDisplayName = req.DisplayName
		return http.StatusCreated, clients.CreateAssetResponse{
			ID:          "asset-uuid-123",
			Name:        req.Name,
			DisplayName: req.DisplayName,
			Type: clients.CreateAssetTypeRef{
				ID:   req.TypeID,
				Name: "Business Term",
			},
			Domain: clients.CreateAssetDomainRef{
				ID:   req.DomainID,
				Name: "Test Domain",
			},
		}
	}))

	server := httptest.NewServer(handler)
	defer server.Close()

	client := testutil.NewClient(server)
	_, err := create_asset.NewTool(client).Handler(t.Context(), create_asset.Input{
		Name:        "My Asset",
		AssetTypeID: "00000000-0000-0000-0000-000000000456",
		DomainID:    "00000000-0000-0000-0000-000000000789",
		DisplayName: "My Display Name",
	})
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if receivedDisplayName != "My Display Name" {
		t.Errorf("Expected display name 'My Display Name', got: '%s'", receivedDisplayName)
	}
}

func TestCreateAssetBadRequest(t *testing.T) {
	handler := http.NewServeMux()
	handler.Handle("POST /rest/2.0/assets", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"message": "duplicate asset name"})
	}))

	server := httptest.NewServer(handler)
	defer server.Close()

	client := testutil.NewClient(server)
	_, err := create_asset.NewTool(client).Handler(t.Context(), create_asset.Input{
		Name:        "Duplicate Asset",
		AssetTypeID: "00000000-0000-0000-0000-000000000456",
		DomainID:    "00000000-0000-0000-0000-000000000789",
	})
	if err == nil {
		t.Fatal("Expected error for bad request, got nil")
	}

	expectedSubstring := "bad request"
	if got := err.Error(); !containsSubstring(got, expectedSubstring) {
		t.Errorf("Expected error to contain '%s', got: '%s'", expectedSubstring, got)
	}
}

func TestCreateAssetNotFound(t *testing.T) {
	handler := http.NewServeMux()
	handler.Handle("POST /rest/2.0/assets", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]string{"message": "asset type not found"})
	}))

	server := httptest.NewServer(handler)
	defer server.Close()

	client := testutil.NewClient(server)
	_, err := create_asset.NewTool(client).Handler(t.Context(), create_asset.Input{
		Name:        "Test Asset",
		AssetTypeID: "00000000-0000-0000-0000-000000000999",
		DomainID:    "00000000-0000-0000-0000-000000000789",
	})
	if err == nil {
		t.Fatal("Expected error for not found, got nil")
	}

	expectedSubstring := "invalid assetTypeId or domainId"
	if got := err.Error(); !containsSubstring(got, expectedSubstring) {
		t.Errorf("Expected error to contain '%s', got: '%s'", expectedSubstring, got)
	}
}

func TestCreateAssetForbidden(t *testing.T) {
	handler := http.NewServeMux()
	handler.Handle("POST /rest/2.0/assets", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		_ = json.NewEncoder(w).Encode(map[string]string{"message": "type not allowed in domain"})
	}))

	server := httptest.NewServer(handler)
	defer server.Close()

	client := testutil.NewClient(server)
	_, err := create_asset.NewTool(client).Handler(t.Context(), create_asset.Input{
		Name:        "Test Asset",
		AssetTypeID: "00000000-0000-0000-0000-000000000456",
		DomainID:    "00000000-0000-0000-0000-000000000789",
	})
	if err == nil {
		t.Fatal("Expected error for forbidden, got nil")
	}

	expectedSubstring := "type not allowed in domain"
	if got := err.Error(); !containsSubstring(got, expectedSubstring) {
		t.Errorf("Expected error to contain '%s', got: '%s'", expectedSubstring, got)
	}
}

func TestCreateAssetEmptyAttributes(t *testing.T) {
	handler := http.NewServeMux()
	handler.Handle("POST /rest/2.0/assets", testutil.JsonHandlerInOut(func(_ *http.Request, req clients.CreateAssetRequest) (int, clients.CreateAssetResponse) {
		return http.StatusCreated, clients.CreateAssetResponse{
			ID:   "asset-uuid-empty-attrs",
			Name: req.Name,
			Type: clients.CreateAssetTypeRef{
				ID:   req.TypeID,
				Name: "Business Term",
			},
			Domain: clients.CreateAssetDomainRef{
				ID:   req.DomainID,
				Name: "Test Domain",
			},
		}
	}))

	server := httptest.NewServer(handler)
	defer server.Close()

	client := testutil.NewClient(server)
	output, err := create_asset.NewTool(client).Handler(t.Context(), create_asset.Input{
		Name:        "Asset No Attrs",
		AssetTypeID: "00000000-0000-0000-0000-000000000456",
		DomainID:    "00000000-0000-0000-0000-000000000789",
		Attributes:  map[string]string{},
	})
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if output.AssetID != "asset-uuid-empty-attrs" {
		t.Errorf("Expected asset ID 'asset-uuid-empty-attrs', got: '%s'", output.AssetID)
	}
}

func TestCreateAssetAttributeFailure(t *testing.T) {
	handler := http.NewServeMux()
	handler.Handle("POST /rest/2.0/assets", testutil.JsonHandlerInOut(func(_ *http.Request, req clients.CreateAssetRequest) (int, clients.CreateAssetResponse) {
		return http.StatusCreated, clients.CreateAssetResponse{
			ID:   "asset-uuid-123",
			Name: req.Name,
			Type: clients.CreateAssetTypeRef{
				ID:   req.TypeID,
				Name: "Business Term",
			},
			Domain: clients.CreateAssetDomainRef{
				ID:   req.DomainID,
				Name: "Test Domain",
			},
		}
	}))
	handler.Handle("POST /rest/2.0/attributes", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]string{"message": "attribute type not found"})
	}))

	server := httptest.NewServer(handler)
	defer server.Close()

	client := testutil.NewClient(server)
	_, err := create_asset.NewTool(client).Handler(t.Context(), create_asset.Input{
		Name:        "Asset With Bad Attr",
		AssetTypeID: "00000000-0000-0000-0000-000000000456",
		DomainID:    "00000000-0000-0000-0000-000000000789",
		Attributes: map[string]string{
			"00000000-0000-0000-0000-0000000000bb": "some value",
		},
	})
	if err == nil {
		t.Fatal("Expected error for attribute creation failure, got nil")
	}

	expectedSubstring := "failed to add attribute"
	if got := err.Error(); !containsSubstring(got, expectedSubstring) {
		t.Errorf("Expected error to contain '%s', got: '%s'", expectedSubstring, got)
	}
}

func containsSubstring(s, substr string) bool {
	return len(s) >= len(substr) && searchSubstring(s, substr)
}

func searchSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
