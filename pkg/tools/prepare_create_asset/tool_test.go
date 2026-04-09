package prepare_create_asset_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/collibra/chip/pkg/clients"
	"github.com/collibra/chip/pkg/tools/prepare_create_asset"
	"github.com/collibra/chip/pkg/tools/testutil"
)

func TestReadyStatus(t *testing.T) {
	handler := http.NewServeMux()

	// Resolve asset type by publicId
	handler.Handle("/rest/2.0/assetTypes/publicId/DataSet", testutil.JsonHandlerOut(func(_ *http.Request) (int, clients.PrepareCreateAssetType) {
		return http.StatusOK, clients.PrepareCreateAssetType{
			ID: "at-123", PublicID: "DataSet", Name: "Data Set",
		}
	}))

	// Validate domain
	handler.Handle("/rest/2.0/domains/dom-456", testutil.JsonHandlerOut(func(_ *http.Request) (int, clients.PrepareCreateDomain) {
		return http.StatusOK, clients.PrepareCreateDomain{
			ID: "dom-456", Name: "Marketing",
		}
	}))

	// Available asset types for domain - asset type is allowed
	handler.Handle("/rest/2.0/assignments/domain/dom-456/assetTypes", testutil.JsonHandlerOut(func(_ *http.Request) (int, []clients.PrepareCreateAssetType) {
		return http.StatusOK, []clients.PrepareCreateAssetType{
			{ID: "at-123", PublicID: "DataSet", Name: "Data Set"},
		}
	}))

	// No duplicates found
	handler.Handle("/rest/2.0/assets", testutil.JsonHandlerOut(func(_ *http.Request) (int, clients.PrepareCreateAssetSearchResponse) {
		return http.StatusOK, clients.PrepareCreateAssetSearchResponse{
			Results: []clients.PrepareCreateAssetResult{},
			Total:   0,
		}
	}))

	// Scoped assignments for auto-hydration
	handler.Handle("/rest/2.0/assignments/assetType/at-123", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// Raw API format with nested assignedCharacteristicTypeReferences
		_ = json.NewEncoder(w).Encode([]map[string]interface{}{
			{
				"id": "assign-1",
				"assignedCharacteristicTypeReferences": []map[string]interface{}{
					{
						"id": "ref-1",
						"assignedResourceReference": map[string]interface{}{
							"id":                    "attr-1",
							"name":                  "Description",
							"resourceDiscriminator": "StringAttributeType",
						},
						"minimumOccurrences": 1,
					},
				},
			},
		})
	}))

	// Attribute type hydration
	handler.Handle("/rest/2.0/attributeTypes/attr-1", testutil.JsonHandlerOut(func(_ *http.Request) (int, clients.PrepareCreateAttributeType) {
		return http.StatusOK, clients.PrepareCreateAttributeType{
			ID: "attr-1", Name: "Description", Kind: "STRING", Required: false,
			AllowedValues: []string{"A", "B"},
		}
	}))

	server := httptest.NewServer(handler)
	defer server.Close()

	client := testutil.NewClient(server)
	output, err := prepare_create_asset.NewTool(client).Handler(t.Context(), prepare_create_asset.Input{
		AssetName:   "Campaign Data",
		AssetTypeID: "DataSet",
		DomainID:    "dom-456",
	})
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if output.Status != "ready" {
		t.Errorf("Expected status 'ready', got: %s", output.Status)
	}
	if len(output.AttributeSchema) != 1 {
		t.Fatalf("Expected 1 attribute schema, got: %d", len(output.AttributeSchema))
	}
	if output.AttributeSchema[0].Kind != "STRING" {
		t.Errorf("Expected kind 'STRING', got: %s", output.AttributeSchema[0].Kind)
	}
	// Required should be true because assignment.Min > 0
	if !output.AttributeSchema[0].Required {
		t.Errorf("Expected attribute to be required (min occurrences = 1)")
	}
	if len(output.AttributeSchema[0].AllowedValues) != 2 {
		t.Errorf("Expected 2 allowed values, got: %d", len(output.AttributeSchema[0].AllowedValues))
	}
}

func TestIncompleteNoAssetType(t *testing.T) {
	handler := http.NewServeMux()

	handler.Handle("/rest/2.0/assetTypes", testutil.JsonHandlerOut(func(_ *http.Request) (int, clients.PrepareCreateAssetTypeListResponse) {
		return http.StatusOK, clients.PrepareCreateAssetTypeListResponse{
			Results: []clients.PrepareCreateAssetType{
				{ID: "at-1", PublicID: "DataSet", Name: "Data Set"},
				{ID: "at-2", PublicID: "Report", Name: "Report"},
			},
			Total: 2,
		}
	}))

	server := httptest.NewServer(handler)
	defer server.Close()

	client := testutil.NewClient(server)
	output, err := prepare_create_asset.NewTool(client).Handler(t.Context(), prepare_create_asset.Input{
		AssetName: "My Asset",
	})
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if output.Status != "incomplete" {
		t.Errorf("Expected status 'incomplete', got: %s", output.Status)
	}
	if len(output.AssetTypeOptions) != 2 {
		t.Errorf("Expected 2 asset type options, got: %d", len(output.AssetTypeOptions))
	}
	if output.OptionsTruncated {
		t.Errorf("Expected options_truncated to be false")
	}
}

func TestIncompleteNoDomain(t *testing.T) {
	handler := http.NewServeMux()

	handler.Handle("/rest/2.0/domains", testutil.JsonHandlerOut(func(_ *http.Request) (int, clients.PrepareCreateDomainListResponse) {
		return http.StatusOK, clients.PrepareCreateDomainListResponse{
			Results: []clients.PrepareCreateDomain{
				{ID: "dom-1", Name: "Marketing"},
			},
			Total: 1,
		}
	}))

	server := httptest.NewServer(handler)
	defer server.Close()

	client := testutil.NewClient(server)
	output, err := prepare_create_asset.NewTool(client).Handler(t.Context(), prepare_create_asset.Input{
		AssetName:   "My Asset",
		AssetTypeID: "DataSet",
	})
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if output.Status != "incomplete" {
		t.Errorf("Expected status 'incomplete', got: %s", output.Status)
	}
	if len(output.DomainOptions) != 1 {
		t.Errorf("Expected 1 domain option, got: %d", len(output.DomainOptions))
	}
}

func TestOptionsTruncated(t *testing.T) {
	handler := http.NewServeMux()

	// Build 21 asset types to trigger truncation
	types := make([]clients.PrepareCreateAssetType, 21)
	for i := 0; i < 21; i++ {
		types[i] = clients.PrepareCreateAssetType{
			ID: "at-id", PublicID: "pub", Name: "Type",
		}
	}

	handler.Handle("/rest/2.0/assetTypes", testutil.JsonHandlerOut(func(_ *http.Request) (int, clients.PrepareCreateAssetTypeListResponse) {
		return http.StatusOK, clients.PrepareCreateAssetTypeListResponse{
			Results: types,
			Total:   25,
		}
	}))

	server := httptest.NewServer(handler)
	defer server.Close()

	client := testutil.NewClient(server)
	output, err := prepare_create_asset.NewTool(client).Handler(t.Context(), prepare_create_asset.Input{
		AssetName: "My Asset",
	})
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if output.Status != "incomplete" {
		t.Errorf("Expected status 'incomplete', got: %s", output.Status)
	}
	if !output.OptionsTruncated {
		t.Errorf("Expected options_truncated to be true")
	}
	if len(output.AssetTypeOptions) != 20 {
		t.Errorf("Expected 20 asset type options, got: %d", len(output.AssetTypeOptions))
	}
}

func TestNeedsClarificationInvalidAssetType(t *testing.T) {
	handler := http.NewServeMux()

	handler.Handle("/rest/2.0/assetTypes/publicId/BadType", testutil.JsonHandlerOut(func(_ *http.Request) (int, map[string]string) {
		return http.StatusNotFound, map[string]string{"error": "not found"}
	}))

	server := httptest.NewServer(handler)
	defer server.Close()

	client := testutil.NewClient(server)
	output, err := prepare_create_asset.NewTool(client).Handler(t.Context(), prepare_create_asset.Input{
		AssetName:   "My Asset",
		AssetTypeID: "BadType",
		DomainID:    "dom-1",
	})
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if output.Status != "needs_clarification" {
		t.Errorf("Expected status 'needs_clarification', got: %s", output.Status)
	}
	if !strings.Contains(output.Message, "Could not resolve asset type") {
		t.Errorf("Expected message about asset type resolution, got: %s", output.Message)
	}
}

func TestNeedsClarificationDomainNotAllowed(t *testing.T) {
	handler := http.NewServeMux()

	handler.Handle("/rest/2.0/assetTypes/publicId/DataSet", testutil.JsonHandlerOut(func(_ *http.Request) (int, clients.PrepareCreateAssetType) {
		return http.StatusOK, clients.PrepareCreateAssetType{
			ID: "at-123", PublicID: "DataSet", Name: "Data Set",
		}
	}))

	handler.Handle("/rest/2.0/domains/dom-999", testutil.JsonHandlerOut(func(_ *http.Request) (int, clients.PrepareCreateDomain) {
		return http.StatusOK, clients.PrepareCreateDomain{
			ID: "dom-999", Name: "Restricted Domain",
		}
	}))

	// Available asset types for domain - asset type NOT in the list
	handler.Handle("/rest/2.0/assignments/domain/dom-999/assetTypes", testutil.JsonHandlerOut(func(_ *http.Request) (int, []clients.PrepareCreateAssetType) {
		return http.StatusOK, []clients.PrepareCreateAssetType{
			{ID: "at-other", PublicID: "Report", Name: "Report"},
		}
	}))

	server := httptest.NewServer(handler)
	defer server.Close()

	client := testutil.NewClient(server)
	output, err := prepare_create_asset.NewTool(client).Handler(t.Context(), prepare_create_asset.Input{
		AssetName:   "My Asset",
		AssetTypeID: "DataSet",
		DomainID:    "dom-999",
	})
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if output.Status != "needs_clarification" {
		t.Errorf("Expected status 'needs_clarification', got: %s", output.Status)
	}
	if !strings.Contains(output.Message, "not allowed") {
		t.Errorf("Expected message about domain not allowed, got: %s", output.Message)
	}
}

func TestDuplicateFound(t *testing.T) {
	handler := http.NewServeMux()

	handler.Handle("/rest/2.0/assetTypes/publicId/DataSet", testutil.JsonHandlerOut(func(_ *http.Request) (int, clients.PrepareCreateAssetType) {
		return http.StatusOK, clients.PrepareCreateAssetType{
			ID: "at-123", PublicID: "DataSet", Name: "Data Set",
		}
	}))

	handler.Handle("/rest/2.0/domains/dom-456", testutil.JsonHandlerOut(func(_ *http.Request) (int, clients.PrepareCreateDomain) {
		return http.StatusOK, clients.PrepareCreateDomain{
			ID: "dom-456", Name: "Marketing",
		}
	}))

	handler.Handle("/rest/2.0/assignments/domain/dom-456/assetTypes", testutil.JsonHandlerOut(func(_ *http.Request) (int, []clients.PrepareCreateAssetType) {
		return http.StatusOK, []clients.PrepareCreateAssetType{
			{ID: "at-123", PublicID: "DataSet", Name: "Data Set"},
		}
	}))

	handler.Handle("/rest/2.0/assets", testutil.JsonHandlerOut(func(_ *http.Request) (int, clients.PrepareCreateAssetSearchResponse) {
		return http.StatusOK, clients.PrepareCreateAssetSearchResponse{
			Results: []clients.PrepareCreateAssetResult{
				{ID: "existing-1", Name: "Campaign Data"},
			},
			Total: 1,
		}
	}))

	server := httptest.NewServer(handler)
	defer server.Close()

	client := testutil.NewClient(server)
	output, err := prepare_create_asset.NewTool(client).Handler(t.Context(), prepare_create_asset.Input{
		AssetName:   "Campaign Data",
		AssetTypeID: "DataSet",
		DomainID:    "dom-456",
	})
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if output.Status != "duplicate_found" {
		t.Errorf("Expected status 'duplicate_found', got: %s", output.Status)
	}
	if len(output.Duplicates) != 1 {
		t.Fatalf("Expected 1 duplicate, got: %d", len(output.Duplicates))
	}
	if output.Duplicates[0].ID != "existing-1" {
		t.Errorf("Expected duplicate ID 'existing-1', got: %s", output.Duplicates[0].ID)
	}
}

func TestReadyWithRelationAttribute(t *testing.T) {
	handler := http.NewServeMux()

	handler.Handle("/rest/2.0/assetTypes/publicId/DataSet", testutil.JsonHandlerOut(func(_ *http.Request) (int, clients.PrepareCreateAssetType) {
		return http.StatusOK, clients.PrepareCreateAssetType{
			ID: "at-123", PublicID: "DataSet", Name: "Data Set",
		}
	}))

	handler.Handle("/rest/2.0/domains/dom-456", testutil.JsonHandlerOut(func(_ *http.Request) (int, clients.PrepareCreateDomain) {
		return http.StatusOK, clients.PrepareCreateDomain{
			ID: "dom-456", Name: "Marketing",
		}
	}))

	handler.Handle("/rest/2.0/assignments/domain/dom-456/assetTypes", testutil.JsonHandlerOut(func(_ *http.Request) (int, []clients.PrepareCreateAssetType) {
		return http.StatusOK, []clients.PrepareCreateAssetType{
			{ID: "at-123", PublicID: "DataSet", Name: "Data Set"},
		}
	}))

	handler.Handle("/rest/2.0/assets", testutil.JsonHandlerOut(func(_ *http.Request) (int, clients.PrepareCreateAssetSearchResponse) {
		return http.StatusOK, clients.PrepareCreateAssetSearchResponse{
			Results: []clients.PrepareCreateAssetResult{},
			Total:   0,
		}
	}))

	// Scoped assignments returning a relation attribute
	handler.Handle("/rest/2.0/assignments/assetType/at-123", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]map[string]interface{}{
			{
				"id": "assign-1",
				"assignedCharacteristicTypeReferences": []map[string]interface{}{
					{
						"id": "ref-1",
						"assignedResourceReference": map[string]interface{}{
							"id":                    "rel-1",
							"name":                  "Owner",
							"resourceDiscriminator": "StringAttributeType",
						},
						"minimumOccurrences": 0,
					},
				},
			},
		})
	}))

	handler.Handle("/rest/2.0/attributeTypes/rel-1", testutil.JsonHandlerOut(func(_ *http.Request) (int, clients.PrepareCreateAttributeType) {
		return http.StatusOK, clients.PrepareCreateAttributeType{
			ID: "rel-1", Name: "Owner", Kind: "RELATION", Required: false,
			Direction: "OUTGOING",
			TargetAssetType: &clients.PrepareCreateAssetType{
				ID: "at-999", PublicID: "Person", Name: "Person",
			},
		}
	}))

	server := httptest.NewServer(handler)
	defer server.Close()

	client := testutil.NewClient(server)
	output, err := prepare_create_asset.NewTool(client).Handler(t.Context(), prepare_create_asset.Input{
		AssetName:   "Campaign Data",
		AssetTypeID: "DataSet",
		DomainID:    "dom-456",
	})
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if output.Status != "ready" {
		t.Errorf("Expected status 'ready', got: %s", output.Status)
	}
	if len(output.AttributeSchema) != 1 {
		t.Fatalf("Expected 1 attribute schema, got: %d", len(output.AttributeSchema))
	}
	schema := output.AttributeSchema[0]
	if schema.Direction != "OUTGOING" {
		t.Errorf("Expected direction 'OUTGOING', got: %s", schema.Direction)
	}
	if schema.TargetAssetType == nil {
		t.Fatal("Expected target asset type to be set")
	}
	if schema.TargetAssetType.Name != "Person" {
		t.Errorf("Expected target asset type name 'Person', got: %s", schema.TargetAssetType.Name)
	}
}

func TestReadyNoAttributes(t *testing.T) {
	handler := http.NewServeMux()

	handler.Handle("/rest/2.0/assetTypes/publicId/DataSet", testutil.JsonHandlerOut(func(_ *http.Request) (int, clients.PrepareCreateAssetType) {
		return http.StatusOK, clients.PrepareCreateAssetType{
			ID: "at-123", PublicID: "DataSet", Name: "Data Set",
		}
	}))

	handler.Handle("/rest/2.0/domains/dom-456", testutil.JsonHandlerOut(func(_ *http.Request) (int, clients.PrepareCreateDomain) {
		return http.StatusOK, clients.PrepareCreateDomain{
			ID: "dom-456", Name: "Marketing",
		}
	}))

	handler.Handle("/rest/2.0/assignments/domain/dom-456/assetTypes", testutil.JsonHandlerOut(func(_ *http.Request) (int, []clients.PrepareCreateAssetType) {
		return http.StatusOK, []clients.PrepareCreateAssetType{
			{ID: "at-123", PublicID: "DataSet", Name: "Data Set"},
		}
	}))

	handler.Handle("/rest/2.0/assets", testutil.JsonHandlerOut(func(_ *http.Request) (int, clients.PrepareCreateAssetSearchResponse) {
		return http.StatusOK, clients.PrepareCreateAssetSearchResponse{
			Results: []clients.PrepareCreateAssetResult{},
			Total:   0,
		}
	}))

	// Empty scoped assignments
	handler.Handle("/rest/2.0/assignments/assetType/at-123", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]map[string]interface{}{})
	}))

	server := httptest.NewServer(handler)
	defer server.Close()

	client := testutil.NewClient(server)
	output, err := prepare_create_asset.NewTool(client).Handler(t.Context(), prepare_create_asset.Input{
		AssetName:   "Campaign Data",
		AssetTypeID: "DataSet",
		DomainID:    "dom-456",
	})
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if output.Status != "ready" {
		t.Errorf("Expected status 'ready', got: %s", output.Status)
	}
	if len(output.AttributeSchema) != 0 {
		t.Errorf("Expected 0 attribute schemas, got: %d", len(output.AttributeSchema))
	}
}

func TestAPIErrorOnAssetTypeList(t *testing.T) {
	handler := http.NewServeMux()

	handler.Handle("/rest/2.0/assetTypes", testutil.JsonHandlerOut(func(_ *http.Request) (int, map[string]string) {
		return http.StatusInternalServerError, map[string]string{"error": "server error"}
	}))

	server := httptest.NewServer(handler)
	defer server.Close()

	client := testutil.NewClient(server)
	_, err := prepare_create_asset.NewTool(client).Handler(t.Context(), prepare_create_asset.Input{
		AssetName: "My Asset",
	})
	if err == nil {
		t.Fatal("Expected error for server error response")
	}
	if !strings.Contains(err.Error(), "500") {
		t.Errorf("Expected error to contain status code 500, got: %s", err.Error())
	}
}
