package create_data_element_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/collibra/chip/pkg/clients"
	"github.com/collibra/chip/pkg/tools/create_data_element"
	"github.com/collibra/chip/pkg/tools/testutil"
)

func TestCreateDataElement_Success(t *testing.T) {
	handler := http.NewServeMux()
	handler.Handle("/rest/2.0/assets", testutil.JsonHandlerInOut(
		func(_ *http.Request, req clients.CreateDataElementRequest) (int, clients.CreateDataElementResponse) {
			if req.Name != "my_data_element" {
				t.Errorf("expected name %q, got %q", "my_data_element", req.Name)
			}
			if req.DomainID != "d0000000-0000-0000-0000-000000000001" {
				t.Errorf("expected domainId %q, got %q", "d0000000-0000-0000-0000-000000000001", req.DomainID)
			}
			if req.TypeID != clients.DataElementAssetTypeID {
				t.Errorf("expected typeId %q, got %q", clients.DataElementAssetTypeID, req.TypeID)
			}
			return http.StatusCreated, clients.CreateDataElementResponse{
				ID:           "a1111111-1111-1111-1111-111111111111",
				ResourceType: "Asset",
				Name:         req.Name,
				DisplayName:  req.Name,
				Domain: clients.CreateDataElementDomainRef{
					ID:   req.DomainID,
					Name: "Test Domain",
				},
				Type: clients.CreateDataElementTypeRef{
					ID:   req.TypeID,
					Name: "Data Element",
				},
				Status: clients.CreateDataElementStatusRef{
					ID:   "00000000-0000-0000-0000-000000000002",
					Name: "Approved",
				},
			}
		},
	))

	server := httptest.NewServer(handler)
	defer server.Close()

	client := testutil.NewClient(server)
	output, err := create_data_element.NewTool(client).Handler(t.Context(), create_data_element.Input{
		Name:     "my_data_element",
		DomainID: "d0000000-0000-0000-0000-000000000001",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if output.ID != "a1111111-1111-1111-1111-111111111111" {
		t.Errorf("got ID %q, want %q", output.ID, "a1111111-1111-1111-1111-111111111111")
	}
	if output.ResourceType != "Asset" {
		t.Errorf("got ResourceType %q, want %q", output.ResourceType, "Asset")
	}
}

func TestCreateDataElement_WithOptionalFields(t *testing.T) {
	handler := http.NewServeMux()
	handler.Handle("/rest/2.0/assets", testutil.JsonHandlerInOut(
		func(_ *http.Request, req clients.CreateDataElementRequest) (int, clients.CreateDataElementResponse) {
			if req.DisplayName != "My Display Name" {
				t.Errorf("expected displayName %q, got %q", "My Display Name", req.DisplayName)
			}
			if req.StatusID != "s0000000-0000-0000-0000-000000000003" {
				t.Errorf("expected statusId %q, got %q", "s0000000-0000-0000-0000-000000000003", req.StatusID)
			}
			return http.StatusCreated, clients.CreateDataElementResponse{
				ID:           "a2222222-2222-2222-2222-222222222222",
				ResourceType: "Asset",
				Name:         req.Name,
				DisplayName:  req.DisplayName,
			}
		},
	))

	server := httptest.NewServer(handler)
	defer server.Close()

	client := testutil.NewClient(server)
	output, err := create_data_element.NewTool(client).Handler(t.Context(), create_data_element.Input{
		Name:        "optional_test",
		DomainID:    "d0000000-0000-0000-0000-000000000001",
		DisplayName: "My Display Name",
		StatusID:    "s0000000-0000-0000-0000-000000000003",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if output.ID != "a2222222-2222-2222-2222-222222222222" {
		t.Errorf("got ID %q, want %q", output.ID, "a2222222-2222-2222-2222-222222222222")
	}
}

func TestCreateDataElement_MissingName(t *testing.T) {
	server := httptest.NewServer(http.NewServeMux())
	defer server.Close()

	client := testutil.NewClient(server)
	_, err := create_data_element.NewTool(client).Handler(t.Context(), create_data_element.Input{
		DomainID: "d0000000-0000-0000-0000-000000000001",
	})
	if err == nil {
		t.Fatal("expected error for missing name, got nil")
	}
	if !strings.Contains(err.Error(), "name is required") {
		t.Errorf("expected error to mention 'name is required', got: %v", err)
	}
}

func TestCreateDataElement_MissingDomainID(t *testing.T) {
	server := httptest.NewServer(http.NewServeMux())
	defer server.Close()

	client := testutil.NewClient(server)
	_, err := create_data_element.NewTool(client).Handler(t.Context(), create_data_element.Input{
		Name: "some_element",
	})
	if err == nil {
		t.Fatal("expected error for missing domain_id, got nil")
	}
	if !strings.Contains(err.Error(), "domain_id is required") {
		t.Errorf("expected error to mention 'domain_id is required', got: %v", err)
	}
}

func TestCreateDataElement_DuplicateNameConflict(t *testing.T) {
	handler := http.NewServeMux()
	handler.Handle("/rest/2.0/assets", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"message": "Asset with name 'duplicate_element' already exists in domain",
		})
	}))

	server := httptest.NewServer(handler)
	defer server.Close()

	client := testutil.NewClient(server)
	_, err := create_data_element.NewTool(client).Handler(t.Context(), create_data_element.Input{
		Name:     "duplicate_element",
		DomainID: "d0000000-0000-0000-0000-000000000001",
	})
	if err == nil {
		t.Fatal("expected error for duplicate name, got nil")
	}
	if !strings.Contains(err.Error(), "already exists") {
		t.Errorf("expected error to mention 'already exists', got: %v", err)
	}
}

func TestCreateDataElement_ServerError(t *testing.T) {
	handler := http.NewServeMux()
	handler.Handle("/rest/2.0/assets", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))

	server := httptest.NewServer(handler)
	defer server.Close()

	client := testutil.NewClient(server)
	_, err := create_data_element.NewTool(client).Handler(t.Context(), create_data_element.Input{
		Name:     "some_element",
		DomainID: "d0000000-0000-0000-0000-000000000001",
	})
	if err == nil {
		t.Fatal("expected error for server error, got nil")
	}
	if !strings.Contains(err.Error(), "500") {
		t.Errorf("expected error to mention status 500, got: %v", err)
	}
}

func TestCreateDataElement_Unauthorized(t *testing.T) {
	handler := http.NewServeMux()
	handler.Handle("/rest/2.0/assets", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))

	server := httptest.NewServer(handler)
	defer server.Close()

	client := testutil.NewClient(server)
	_, err := create_data_element.NewTool(client).Handler(t.Context(), create_data_element.Input{
		Name:     "some_element",
		DomainID: "d0000000-0000-0000-0000-000000000001",
	})
	if err == nil {
		t.Fatal("expected error for unauthorized, got nil")
	}
	if !strings.Contains(err.Error(), "401") {
		t.Errorf("expected error to mention status 401, got: %v", err)
	}
}

func TestCreateDataElement_TypeIDIsSet(t *testing.T) {
	var capturedTypeID string

	handler := http.NewServeMux()
	handler.Handle("/rest/2.0/assets", testutil.JsonHandlerInOut(
		func(_ *http.Request, req clients.CreateDataElementRequest) (int, clients.CreateDataElementResponse) {
			capturedTypeID = req.TypeID
			return http.StatusCreated, clients.CreateDataElementResponse{
				ID:           "a3333333-3333-3333-3333-333333333333",
				ResourceType: "Asset",
			}
		},
	))

	server := httptest.NewServer(handler)
	defer server.Close()

	client := testutil.NewClient(server)
	_, err := create_data_element.NewTool(client).Handler(t.Context(), create_data_element.Input{
		Name:     "type_check",
		DomainID: "d0000000-0000-0000-0000-000000000001",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if capturedTypeID != clients.DataElementAssetTypeID {
		t.Errorf("got typeId %q, want %q", capturedTypeID, clients.DataElementAssetTypeID)
	}
}

func TestCreateDataElement_ToolMetadata(t *testing.T) {
	server := httptest.NewServer(http.NewServeMux())
	defer server.Close()

	client := testutil.NewClient(server)
	tool := create_data_element.NewTool(client)

	if tool.Name != "create_data_element" {
		t.Errorf("got Name %q, want %q", tool.Name, "create_data_element")
	}
	if tool.Description == "" {
		t.Error("expected non-empty Description")
	}
	if len(tool.Permissions) != 1 || tool.Permissions[0] != "dgc.ai-copilot" {
		t.Errorf("got Permissions %v, want [dgc.ai-copilot]", tool.Permissions)
	}
}
