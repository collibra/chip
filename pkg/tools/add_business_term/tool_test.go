package add_business_term_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/collibra/chip/pkg/clients"
	"github.com/collibra/chip/pkg/tools/add_business_term"
	"github.com/collibra/chip/pkg/tools/testutil"
)

func TestAddBusinessTermSuccess(t *testing.T) {
	handler := http.NewServeMux()

	handler.Handle("/rest/2.0/assets", testutil.JsonHandlerInOut(func(_ *http.Request, req clients.AddBusinessTermAssetRequest) (int, clients.AddBusinessTermAssetResponse) {
		if req.Name != "Revenue" {
			t.Errorf("expected name 'Revenue', got '%s'", req.Name)
		}
		if req.TypePublicId != "BusinessTerm" {
			t.Errorf("expected typePublicId 'BusinessTerm', got '%s'", req.TypePublicId)
		}
		if req.DomainId != "00000000-0000-0000-0000-000000000123" {
			t.Errorf("expected domainId '00000000-0000-0000-0000-000000000123', got '%s'", req.DomainId)
		}
		return http.StatusCreated, clients.AddBusinessTermAssetResponse{Id: "new-asset-uuid-456"}
	}))

	handler.Handle("/rest/2.0/attributes", testutil.JsonHandlerInOut(func(_ *http.Request, req clients.AddBusinessTermAttributeRequest) (int, clients.AddBusinessTermAttributeResponse) {
		if req.AssetId != "new-asset-uuid-456" {
			t.Errorf("expected assetId 'new-asset-uuid-456', got '%s'", req.AssetId)
		}
		if req.TypeId != "00000000-0000-0000-0000-000000000202" {
			t.Errorf("expected definition typeId '00000000-0000-0000-0000-000000000202', got '%s'", req.TypeId)
		}
		if req.Value != "Total income generated from sales" {
			t.Errorf("expected value 'Total income generated from sales', got '%s'", req.Value)
		}
		return http.StatusCreated, clients.AddBusinessTermAttributeResponse{Id: "attr-uuid-789"}
	}))

	server := httptest.NewServer(handler)
	defer server.Close()

	client := testutil.NewClient(server)
	output, err := add_business_term.NewTool(client).Handler(t.Context(), add_business_term.Input{
		Name:       "Revenue",
		DomainId:   "00000000-0000-0000-0000-000000000123",
		Definition: "Total income generated from sales",
	})
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if output.AssetId != "new-asset-uuid-456" {
		t.Errorf("expected asset_id 'new-asset-uuid-456', got '%s'", output.AssetId)
	}
}

func TestAddBusinessTermAssetCreationError(t *testing.T) {
	handler := http.NewServeMux()

	handler.Handle("/rest/2.0/assets", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"message":"duplicate term name"}`))
	}))

	server := httptest.NewServer(handler)
	defer server.Close()

	client := testutil.NewClient(server)
	_, err := add_business_term.NewTool(client).Handler(t.Context(), add_business_term.Input{
		Name:     "Revenue",
		DomainId: "invalid-domain",
	})
	if err == nil {
		t.Fatal("expected error for bad request, got nil")
	}
}

func TestAddBusinessTermNoDefinitionNoAttributes(t *testing.T) {
	handler := http.NewServeMux()

	handler.Handle("/rest/2.0/assets", testutil.JsonHandlerInOut(func(_ *http.Request, req clients.AddBusinessTermAssetRequest) (int, clients.AddBusinessTermAssetResponse) {
		return http.StatusCreated, clients.AddBusinessTermAssetResponse{Id: "asset-no-def-123"}
	}))

	attributeCalled := false
	handler.Handle("/rest/2.0/attributes", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attributeCalled = true
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":"should-not-be-called"}`))
	}))

	server := httptest.NewServer(handler)
	defer server.Close()

	client := testutil.NewClient(server)
	output, err := add_business_term.NewTool(client).Handler(t.Context(), add_business_term.Input{
		Name:     "Simple Term",
		DomainId: "00000000-0000-0000-0000-000000000456",
	})
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if output.AssetId != "asset-no-def-123" {
		t.Errorf("expected asset_id 'asset-no-def-123', got '%s'", output.AssetId)
	}
	if attributeCalled {
		t.Error("expected attributes endpoint not to be called when no definition or attributes provided")
	}
}

func TestAddBusinessTermWithAdditionalAttributes(t *testing.T) {
	handler := http.NewServeMux()

	handler.Handle("/rest/2.0/assets", testutil.JsonHandlerInOut(func(_ *http.Request, req clients.AddBusinessTermAssetRequest) (int, clients.AddBusinessTermAssetResponse) {
		return http.StatusCreated, clients.AddBusinessTermAssetResponse{Id: "asset-with-attrs-789"}
	}))

	attrCount := 0
	handler.Handle("/rest/2.0/attributes", testutil.JsonHandlerInOut(func(_ *http.Request, req clients.AddBusinessTermAttributeRequest) (int, clients.AddBusinessTermAttributeResponse) {
		attrCount++
		if req.AssetId != "asset-with-attrs-789" {
			t.Errorf("expected assetId 'asset-with-attrs-789', got '%s'", req.AssetId)
		}
		return http.StatusCreated, clients.AddBusinessTermAttributeResponse{Id: "attr-" + req.TypeId}
	}))

	server := httptest.NewServer(handler)
	defer server.Close()

	client := testutil.NewClient(server)
	output, err := add_business_term.NewTool(client).Handler(t.Context(), add_business_term.Input{
		Name:       "Complex Term",
		DomainId:   "00000000-0000-0000-0000-000000000789",
		Definition: "A complex business term",
		Attributes: []add_business_term.InputAttribute{
			{TypeId: "00000000-0000-0000-0000-0000000000c1", Value: "custom value 1"},
			{TypeId: "00000000-0000-0000-0000-0000000000c2", Value: "custom value 2"},
		},
	})
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if output.AssetId != "asset-with-attrs-789" {
		t.Errorf("expected asset_id 'asset-with-attrs-789', got '%s'", output.AssetId)
	}
	// 1 definition + 2 additional attributes = 3 total
	if attrCount != 3 {
		t.Errorf("expected 3 attribute calls, got %d", attrCount)
	}
}

func TestAddBusinessTermAttributeCreationError(t *testing.T) {
	handler := http.NewServeMux()

	handler.Handle("/rest/2.0/assets", testutil.JsonHandlerInOut(func(_ *http.Request, req clients.AddBusinessTermAssetRequest) (int, clients.AddBusinessTermAssetResponse) {
		return http.StatusCreated, clients.AddBusinessTermAssetResponse{Id: "asset-attr-err-123"}
	}))

	handler.Handle("/rest/2.0/attributes", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"message":"internal server error"}`))
	}))

	server := httptest.NewServer(handler)
	defer server.Close()

	client := testutil.NewClient(server)
	_, err := add_business_term.NewTool(client).Handler(t.Context(), add_business_term.Input{
		Name:       "Failing Term",
		DomainId:   "00000000-0000-0000-0000-000000000123",
		Definition: "This should fail on attribute creation",
	})
	if err == nil {
		t.Fatal("expected error when attribute creation fails, got nil")
	}
}
