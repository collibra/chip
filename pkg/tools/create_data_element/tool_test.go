package create_data_element_test

import (
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
			return http.StatusCreated, clients.CreateDataElementResponse{
				ID:           "asset-uuid-123",
				Name:         req.Name,
				DisplayName:  req.DisplayName,
				ResourceType: "Asset",
				Type: clients.CreateDataElementTypeRef{
					Name: "Data Element",
					ID:   req.TypeID,
				},
				Domain: clients.CreateDataElementDomainRef{
					Name: "Test Domain",
					ID:   req.DomainID,
				},
			}
		},
	))

	server := httptest.NewServer(handler)
	defer server.Close()

	client := testutil.NewClient(server)
	output, err := create_data_element.NewTool(client).Handler(t.Context(), create_data_element.Input{
		Name:     "My Data Element",
		DomainID: "domain-uuid-456",
		TypeID:   "type-uuid-789",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if output.ID != "asset-uuid-123" {
		t.Errorf("got ID %q, want %q", output.ID, "asset-uuid-123")
	}
	if output.ResourceType != "Asset" {
		t.Errorf("got ResourceType %q, want %q", output.ResourceType, "Asset")
	}
}

func TestCreateDataElement_WithOptionalFields(t *testing.T) {
	handler := http.NewServeMux()
	handler.Handle("/rest/2.0/assets", testutil.JsonHandlerInOut(
		func(_ *http.Request, req clients.CreateDataElementRequest) (int, clients.CreateDataElementResponse) {
			if req.DisplayName != "Display Name" {
				t.Errorf("got DisplayName %q, want %q", req.DisplayName, "Display Name")
			}
			if req.StatusID != "status-uuid-111" {
				t.Errorf("got StatusID %q, want %q", req.StatusID, "status-uuid-111")
			}
			return http.StatusCreated, clients.CreateDataElementResponse{
				ID:           "asset-uuid-999",
				Name:         req.Name,
				DisplayName:  req.DisplayName,
				ResourceType: "Asset",
				Status: clients.CreateDataElementStatusRef{
					Name: "Approved",
					ID:   req.StatusID,
				},
			}
		},
	))

	server := httptest.NewServer(handler)
	defer server.Close()

	client := testutil.NewClient(server)
	output, err := create_data_element.NewTool(client).Handler(t.Context(), create_data_element.Input{
		Name:        "My Data Element",
		DomainID:    "domain-uuid-456",
		TypeID:      "type-uuid-789",
		DisplayName: "Display Name",
		StatusID:    "status-uuid-111",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if output.ID != "asset-uuid-999" {
		t.Errorf("got ID %q, want %q", output.ID, "asset-uuid-999")
	}
	if output.ResourceType != "Asset" {
		t.Errorf("got ResourceType %q, want %q", output.ResourceType, "Asset")
	}
}

func TestCreateDataElement_MissingName(t *testing.T) {
	server := httptest.NewServer(http.NewServeMux())
	defer server.Close()

	client := testutil.NewClient(server)
	_, err := create_data_element.NewTool(client).Handler(t.Context(), create_data_element.Input{
		DomainID: "domain-uuid-456",
		TypeID:   "type-uuid-789",
	})
	if err == nil {
		t.Fatal("expected error for missing name, got nil")
	}
	if !strings.Contains(err.Error(), "name is required") {
		t.Errorf("expected error to contain %q, got %q", "name is required", err.Error())
	}
}

func TestCreateDataElement_MissingDomainID(t *testing.T) {
	server := httptest.NewServer(http.NewServeMux())
	defer server.Close()

	client := testutil.NewClient(server)
	_, err := create_data_element.NewTool(client).Handler(t.Context(), create_data_element.Input{
		Name:   "My Data Element",
		TypeID: "type-uuid-789",
	})
	if err == nil {
		t.Fatal("expected error for missing domain_id, got nil")
	}
	if !strings.Contains(err.Error(), "domain_id is required") {
		t.Errorf("expected error to contain %q, got %q", "domain_id is required", err.Error())
	}
}

func TestCreateDataElement_DuplicateNameConflict(t *testing.T) {
	handler := http.NewServeMux()
	handler.Handle("/rest/2.0/assets", testutil.JsonHandlerInOut(
		func(_ *http.Request, _ clients.CreateDataElementRequest) (int, map[string]any) {
			return http.StatusBadRequest, map[string]any{
				"message": "Asset with name 'My Data Element' already exists in this domain",
			}
		},
	))

	server := httptest.NewServer(handler)
	defer server.Close()

	client := testutil.NewClient(server)
	_, err := create_data_element.NewTool(client).Handler(t.Context(), create_data_element.Input{
		Name:     "My Data Element",
		DomainID: "domain-uuid-456",
		TypeID:   "type-uuid-789",
	})
	if err == nil {
		t.Fatal("expected error for duplicate name, got nil")
	}
	if !strings.Contains(err.Error(), "already exists") {
		t.Errorf("expected error to contain %q, got %q", "already exists", err.Error())
	}
}

func TestCreateDataElement_Unauthorized(t *testing.T) {
	handler := http.NewServeMux()
	handler.Handle("/rest/2.0/assets", testutil.JsonHandlerInOut(
		func(_ *http.Request, _ clients.CreateDataElementRequest) (int, map[string]any) {
			return http.StatusUnauthorized, map[string]any{
				"message": "Authentication failed",
			}
		},
	))

	server := httptest.NewServer(handler)
	defer server.Close()

	client := testutil.NewClient(server)
	_, err := create_data_element.NewTool(client).Handler(t.Context(), create_data_element.Input{
		Name:     "My Data Element",
		DomainID: "domain-uuid-456",
		TypeID:   "type-uuid-789",
	})
	if err == nil {
		t.Fatal("expected error for unauthorized, got nil")
	}
	if !strings.Contains(err.Error(), "401") {
		t.Errorf("expected error to contain status code 401, got %q", err.Error())
	}
}

func TestCreateDataElement_Forbidden(t *testing.T) {
	handler := http.NewServeMux()
	handler.Handle("/rest/2.0/assets", testutil.JsonHandlerInOut(
		func(_ *http.Request, _ clients.CreateDataElementRequest) (int, map[string]any) {
			return http.StatusForbidden, map[string]any{
				"message": "Insufficient permissions",
			}
		},
	))

	server := httptest.NewServer(handler)
	defer server.Close()

	client := testutil.NewClient(server)
	_, err := create_data_element.NewTool(client).Handler(t.Context(), create_data_element.Input{
		Name:     "My Data Element",
		DomainID: "domain-uuid-456",
		TypeID:   "type-uuid-789",
	})
	if err == nil {
		t.Fatal("expected error for forbidden, got nil")
	}
	if !strings.Contains(err.Error(), "403") {
		t.Errorf("expected error to contain status code 403, got %q", err.Error())
	}
}

func TestCreateDataElement_ServerError(t *testing.T) {
	handler := http.NewServeMux()
	handler.Handle("/rest/2.0/assets", testutil.JsonHandlerInOut(
		func(_ *http.Request, _ clients.CreateDataElementRequest) (int, map[string]any) {
			return http.StatusInternalServerError, map[string]any{}
		},
	))

	server := httptest.NewServer(handler)
	defer server.Close()

	client := testutil.NewClient(server)
	_, err := create_data_element.NewTool(client).Handler(t.Context(), create_data_element.Input{
		Name:     "My Data Element",
		DomainID: "domain-uuid-456",
		TypeID:   "type-uuid-789",
	})
	if err == nil {
		t.Fatal("expected error for server error, got nil")
	}
	if !strings.Contains(err.Error(), "500") {
		t.Errorf("expected error to contain status code 500, got %q", err.Error())
	}
}
