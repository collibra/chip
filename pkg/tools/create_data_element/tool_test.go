package create_data_element_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/collibra/chip/pkg/clients"
	"github.com/collibra/chip/pkg/tools/create_data_element"
	"github.com/collibra/chip/pkg/tools/testutil"
)

func TestCreateDataElement_Success(t *testing.T) {
	handler := http.NewServeMux()
	handler.Handle("POST /rest/2.0/assets", testutil.JsonHandlerInOut(
		func(_ *http.Request, req clients.CreateDataElementRequest) (int, clients.CreateDataElementResponse) {
			if req.Name != "Customer Email Address" {
				t.Errorf("expected name 'Customer Email Address', got %q", req.Name)
			}
			if req.DomainID != "12345678-1234-5678-9012-123456789012" {
				t.Errorf("expected domainId '12345678-1234-5678-9012-123456789012', got %q", req.DomainID)
			}
			if req.TypeID != create_data_element.DataElementTypeID {
				t.Errorf("expected typeId %q, got %q", create_data_element.DataElementTypeID, req.TypeID)
			}
			return http.StatusCreated, clients.CreateDataElementResponse{
				ID:           "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee",
				ResourceType: "Asset",
				Name:         req.Name,
				DisplayName:  req.Name,
				Domain: clients.CreateDataElementReference{
					ID:   req.DomainID,
					Name: "Test Domain",
				},
				Type: clients.CreateDataElementReference{
					ID:   req.TypeID,
					Name: "Data Element",
				},
			}
		},
	))

	server := httptest.NewServer(handler)
	defer server.Close()

	client := testutil.NewClient(server)
	output, err := create_data_element.NewTool(client).Handler(t.Context(), create_data_element.Input{
		Name:     "Customer Email Address",
		DomainID: "12345678-1234-5678-9012-123456789012",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if output.ID != "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee" {
		t.Errorf("got ID %q, want %q", output.ID, "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee")
	}
	if output.ResourceType != "Asset" {
		t.Errorf("got ResourceType %q, want %q", output.ResourceType, "Asset")
	}
}

func TestCreateDataElement_WithOptionalFields(t *testing.T) {
	handler := http.NewServeMux()
	handler.Handle("POST /rest/2.0/assets", testutil.JsonHandlerInOut(
		func(_ *http.Request, req clients.CreateDataElementRequest) (int, clients.CreateDataElementResponse) {
			if req.DisplayName != "Customer Email" {
				t.Errorf("expected displayName 'Customer Email', got %q", req.DisplayName)
			}
			if req.StatusID != "11111111-2222-3333-4444-555555555555" {
				t.Errorf("expected statusId '11111111-2222-3333-4444-555555555555', got %q", req.StatusID)
			}
			return http.StatusCreated, clients.CreateDataElementResponse{
				ID:           "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee",
				ResourceType: "Asset",
				Name:         req.Name,
				DisplayName:  req.DisplayName,
				Domain: clients.CreateDataElementReference{
					ID:   req.DomainID,
					Name: "Test Domain",
				},
				Type: clients.CreateDataElementReference{
					ID:   req.TypeID,
					Name: "Data Element",
				},
				Status: clients.CreateDataElementReference{
					ID:   req.StatusID,
					Name: "Approved",
				},
			}
		},
	))

	server := httptest.NewServer(handler)
	defer server.Close()

	client := testutil.NewClient(server)
	output, err := create_data_element.NewTool(client).Handler(t.Context(), create_data_element.Input{
		Name:        "Customer Email Address",
		DomainID:    "12345678-1234-5678-9012-123456789012",
		DisplayName: "Customer Email",
		StatusID:    "11111111-2222-3333-4444-555555555555",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if output.ID != "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee" {
		t.Errorf("got ID %q, want %q", output.ID, "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee")
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
		DomainID: "12345678-1234-5678-9012-123456789012",
	})
	if err == nil {
		t.Fatal("expected error for missing name, got nil")
	}
	if err.Error() != "name is required" {
		t.Errorf("got error %q, want %q", err.Error(), "name is required")
	}
}

func TestCreateDataElement_MissingDomainID(t *testing.T) {
	server := httptest.NewServer(http.NewServeMux())
	defer server.Close()

	client := testutil.NewClient(server)
	_, err := create_data_element.NewTool(client).Handler(t.Context(), create_data_element.Input{
		Name: "Customer Email Address",
	})
	if err == nil {
		t.Fatal("expected error for missing domain_id, got nil")
	}
	if err.Error() != "domainId is required" {
		t.Errorf("got error %q, want %q", err.Error(), "domainId is required")
	}
}

func TestCreateDataElement_WhitespaceOnlyName(t *testing.T) {
	server := httptest.NewServer(http.NewServeMux())
	defer server.Close()

	client := testutil.NewClient(server)
	_, err := create_data_element.NewTool(client).Handler(t.Context(), create_data_element.Input{
		Name:     "   ",
		DomainID: "12345678-1234-5678-9012-123456789012",
	})
	if err == nil {
		t.Fatal("expected error for whitespace-only name, got nil")
	}
	if err.Error() != "name is required" {
		t.Errorf("got error %q, want %q", err.Error(), "name is required")
	}
}

func TestCreateDataElement_InvalidDomainID(t *testing.T) {
	server := httptest.NewServer(http.NewServeMux())
	defer server.Close()

	client := testutil.NewClient(server)
	_, err := create_data_element.NewTool(client).Handler(t.Context(), create_data_element.Input{
		Name:     "Customer Email Address",
		DomainID: "not-a-valid-uuid",
	})
	if err == nil {
		t.Fatal("expected error for invalid domainId, got nil")
	}
	expected := "domainId is not a valid UUID: not-a-valid-uuid"
	if err.Error() != expected {
		t.Errorf("got error %q, want %q", err.Error(), expected)
	}
}

func TestCreateDataElement_InvalidStatusID(t *testing.T) {
	server := httptest.NewServer(http.NewServeMux())
	defer server.Close()

	client := testutil.NewClient(server)
	_, err := create_data_element.NewTool(client).Handler(t.Context(), create_data_element.Input{
		Name:     "Customer Email Address",
		DomainID: "12345678-1234-5678-9012-123456789012",
		StatusID: "bad-status-id",
	})
	if err == nil {
		t.Fatal("expected error for invalid statusId, got nil")
	}
	expected := "statusId is not a valid UUID: bad-status-id"
	if err.Error() != expected {
		t.Errorf("got error %q, want %q", err.Error(), expected)
	}
}

func TestCreateDataElement_DuplicateNameConflict(t *testing.T) {
	handler := http.NewServeMux()
	handler.Handle("POST /rest/2.0/assets", testutil.JsonHandlerInOut(
		func(_ *http.Request, req clients.CreateDataElementRequest) (int, clients.CreateDataElementErrorResponse) {
			return http.StatusBadRequest, clients.CreateDataElementErrorResponse{
				ErrorCode: "DUPLICATE_NAME",
				Message:   "Asset with name 'Customer Email Address' already exists in the domain",
			}
		},
	))

	server := httptest.NewServer(handler)
	defer server.Close()

	client := testutil.NewClient(server)
	_, err := create_data_element.NewTool(client).Handler(t.Context(), create_data_element.Input{
		Name:     "Customer Email Address",
		DomainID: "12345678-1234-5678-9012-123456789012",
	})
	if err == nil {
		t.Fatal("expected error for duplicate name, got nil")
	}
	expected := "API error (status 400): Asset with name 'Customer Email Address' already exists in the domain"
	if err.Error() != expected {
		t.Errorf("got error %q, want %q", err.Error(), expected)
	}
}

func TestCreateDataElement_Unauthorized(t *testing.T) {
	handler := http.NewServeMux()
	handler.Handle("POST /rest/2.0/assets", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))

	server := httptest.NewServer(handler)
	defer server.Close()

	client := testutil.NewClient(server)
	_, err := create_data_element.NewTool(client).Handler(t.Context(), create_data_element.Input{
		Name:     "Customer Email Address",
		DomainID: "12345678-1234-5678-9012-123456789012",
	})
	if err == nil {
		t.Fatal("expected error for unauthorized request, got nil")
	}
}

func TestCreateDataElement_Forbidden(t *testing.T) {
	handler := http.NewServeMux()
	handler.Handle("POST /rest/2.0/assets", testutil.JsonHandlerInOut(
		func(_ *http.Request, req clients.CreateDataElementRequest) (int, clients.CreateDataElementErrorResponse) {
			return http.StatusForbidden, clients.CreateDataElementErrorResponse{
				ErrorCode: "FORBIDDEN",
				Message:   "User lacks permission to create assets in the specified domain",
			}
		},
	))

	server := httptest.NewServer(handler)
	defer server.Close()

	client := testutil.NewClient(server)
	_, err := create_data_element.NewTool(client).Handler(t.Context(), create_data_element.Input{
		Name:     "Customer Email Address",
		DomainID: "12345678-1234-5678-9012-123456789012",
	})
	if err == nil {
		t.Fatal("expected error for forbidden request, got nil")
	}
	expected := "API error (status 403): User lacks permission to create assets in the specified domain"
	if err.Error() != expected {
		t.Errorf("got error %q, want %q", err.Error(), expected)
	}
}

func TestCreateDataElement_ServerError(t *testing.T) {
	handler := http.NewServeMux()
	handler.Handle("POST /rest/2.0/assets", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))

	server := httptest.NewServer(handler)
	defer server.Close()

	client := testutil.NewClient(server)
	_, err := create_data_element.NewTool(client).Handler(t.Context(), create_data_element.Input{
		Name:     "Customer Email Address",
		DomainID: "12345678-1234-5678-9012-123456789012",
	})
	if err == nil {
		t.Fatal("expected error for server error, got nil")
	}
}
