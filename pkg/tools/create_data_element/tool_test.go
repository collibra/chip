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
			if req.Name != "Customer Email Address" {
				t.Errorf("expected name %q, got %q", "Customer Email Address", req.Name)
			}
			if req.DomainID != "a1b2c3d4-e5f6-7890-abcd-ef1234567890" {
				t.Errorf("expected domainId %q, got %q", "a1b2c3d4-e5f6-7890-abcd-ef1234567890", req.DomainID)
			}
			if req.TypeID != "00000000-0000-0000-0000-000000031008" {
				t.Errorf("expected typeId %q, got %q", "00000000-0000-0000-0000-000000031008", req.TypeID)
			}
			return http.StatusCreated, clients.CreateDataElementResponse{
				ID:           "new-asset-uuid-1234",
				ResourceType: "Asset",
				Name:         req.Name,
			}
		},
	))

	server := httptest.NewServer(handler)
	defer server.Close()

	client := testutil.NewClient(server)
	output, err := create_data_element.NewTool(client).Handler(t.Context(), create_data_element.Input{
		Name:     "Customer Email Address",
		DomainID: "a1b2c3d4-e5f6-7890-abcd-ef1234567890",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if output.ID != "new-asset-uuid-1234" {
		t.Errorf("got ID %q, want %q", output.ID, "new-asset-uuid-1234")
	}
	if output.ResourceType != "Asset" {
		t.Errorf("got ResourceType %q, want %q", output.ResourceType, "Asset")
	}
}

func TestCreateDataElement_WithOptionalFields(t *testing.T) {
	handler := http.NewServeMux()
	handler.Handle("/rest/2.0/assets", testutil.JsonHandlerInOut(
		func(_ *http.Request, req clients.CreateDataElementRequest) (int, clients.CreateDataElementResponse) {
			if req.DisplayName != "Customer Email" {
				t.Errorf("expected displayName %q, got %q", "Customer Email", req.DisplayName)
			}
			if req.StatusID != "b2c3d4e5-f6a7-8901-bcde-f23456789012" {
				t.Errorf("expected statusId %q, got %q", "b2c3d4e5-f6a7-8901-bcde-f23456789012", req.StatusID)
			}
			return http.StatusCreated, clients.CreateDataElementResponse{
				ID:           "new-asset-uuid-5678",
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
		Name:        "Customer Email Address",
		DomainID:    "a1b2c3d4-e5f6-7890-abcd-ef1234567890",
		DisplayName: "Customer Email",
		StatusID:    "b2c3d4e5-f6a7-8901-bcde-f23456789012",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if output.ID != "new-asset-uuid-5678" {
		t.Errorf("got ID %q, want %q", output.ID, "new-asset-uuid-5678")
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
		DomainID: "a1b2c3d4-e5f6-7890-abcd-ef1234567890",
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
		Name: "Customer Email Address",
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
	handler.Handle("/rest/2.0/assets", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"message": "Asset with name 'Customer Email Address' already exists in this domain",
		})
	}))

	server := httptest.NewServer(handler)
	defer server.Close()

	client := testutil.NewClient(server)
	_, err := create_data_element.NewTool(client).Handler(t.Context(), create_data_element.Input{
		Name:     "Customer Email Address",
		DomainID: "a1b2c3d4-e5f6-7890-abcd-ef1234567890",
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
	handler.Handle("/rest/2.0/assets", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"message": "Authentication required",
		})
	}))

	server := httptest.NewServer(handler)
	defer server.Close()

	client := testutil.NewClient(server)
	_, err := create_data_element.NewTool(client).Handler(t.Context(), create_data_element.Input{
		Name:     "Customer Email Address",
		DomainID: "a1b2c3d4-e5f6-7890-abcd-ef1234567890",
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
	handler.Handle("/rest/2.0/assets", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"message": "Insufficient permissions",
		})
	}))

	server := httptest.NewServer(handler)
	defer server.Close()

	client := testutil.NewClient(server)
	_, err := create_data_element.NewTool(client).Handler(t.Context(), create_data_element.Input{
		Name:     "Customer Email Address",
		DomainID: "a1b2c3d4-e5f6-7890-abcd-ef1234567890",
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
	handler.Handle("/rest/2.0/assets", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))

	server := httptest.NewServer(handler)
	defer server.Close()

	client := testutil.NewClient(server)
	_, err := create_data_element.NewTool(client).Handler(t.Context(), create_data_element.Input{
		Name:     "Customer Email Address",
		DomainID: "a1b2c3d4-e5f6-7890-abcd-ef1234567890",
	})
	if err == nil {
		t.Fatal("expected error for server error, got nil")
	}
	if !strings.Contains(err.Error(), "500") {
		t.Errorf("expected error to contain status code 500, got %q", err.Error())
	}
}

func TestCreateDataElement_SetsCorrectTypeID(t *testing.T) {
	var capturedTypeID string
	handler := http.NewServeMux()
	handler.Handle("/rest/2.0/assets", testutil.JsonHandlerInOut(
		func(_ *http.Request, req clients.CreateDataElementRequest) (int, clients.CreateDataElementResponse) {
			capturedTypeID = req.TypeID
			return http.StatusCreated, clients.CreateDataElementResponse{
				ID:           "new-asset-uuid",
				ResourceType: "Asset",
				Name:         req.Name,
			}
		},
	))

	server := httptest.NewServer(handler)
	defer server.Close()

	client := testutil.NewClient(server)
	_, err := create_data_element.NewTool(client).Handler(t.Context(), create_data_element.Input{
		Name:     "Test Element",
		DomainID: "a1b2c3d4-e5f6-7890-abcd-ef1234567890",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expectedTypeID := "00000000-0000-0000-0000-000000031008"
	if capturedTypeID != expectedTypeID {
		t.Errorf("got typeId %q, want %q", capturedTypeID, expectedTypeID)
	}
}
