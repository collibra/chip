package create_data_element_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/collibra/chip/pkg/clients"
	"github.com/collibra/chip/pkg/tools/create_data_element"
	"github.com/collibra/chip/pkg/tools/testutil"
)

const (
	testDomainID = "12345678-1234-1234-1234-123456789012"
	testAssetID  = "99999999-8888-7777-6666-555555555555"
	testStatusID = "11111111-2222-3333-4444-555555555555"
)

func TestCreateDataElement_Success(t *testing.T) {
	mux := http.NewServeMux()
	mux.Handle("/rest/2.0/assets", testutil.JsonHandlerInOut(
		func(_ *http.Request, req clients.CreateDataElementRequest) (int, clients.CreateDataElementResponse) {
			if req.Name != "Customer Email Address" {
				t.Errorf("expected name 'Customer Email Address', got %q", req.Name)
			}
			if req.DomainID != testDomainID {
				t.Errorf("expected domainId %q, got %q", testDomainID, req.DomainID)
			}
			if req.TypeID != create_data_element.DataElementTypeID {
				t.Errorf("expected typeId %q, got %q", create_data_element.DataElementTypeID, req.TypeID)
			}
			return http.StatusCreated, clients.CreateDataElementResponse{
				ID:           testAssetID,
				Name:         req.Name,
				ResourceType: "Asset",
				Domain: clients.CreateDataElementRef{
					ID:   req.DomainID,
					Name: "Customer Data Domain",
				},
				Type: clients.CreateDataElementRef{
					ID:   req.TypeID,
					Name: "Data Element",
				},
			}
		},
	))

	server := httptest.NewServer(mux)
	defer server.Close()

	client := testutil.NewClient(server)
	output, err := create_data_element.NewTool(client).Handler(t.Context(), create_data_element.Input{
		Name:     "Customer Email Address",
		DomainID: testDomainID,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if output.ID != testAssetID {
		t.Errorf("got ID %q, want %q", output.ID, testAssetID)
	}
	if output.ResourceType != "Asset" {
		t.Errorf("got ResourceType %q, want %q", output.ResourceType, "Asset")
	}
}

func TestCreateDataElement_WithOptionalFields(t *testing.T) {
	mux := http.NewServeMux()
	mux.Handle("/rest/2.0/assets", testutil.JsonHandlerInOut(
		func(_ *http.Request, req clients.CreateDataElementRequest) (int, clients.CreateDataElementResponse) {
			if req.DisplayName != "Customer Email" {
				t.Errorf("expected displayName 'Customer Email', got %q", req.DisplayName)
			}
			if req.StatusID != testStatusID {
				t.Errorf("expected statusId %q, got %q", testStatusID, req.StatusID)
			}
			return http.StatusCreated, clients.CreateDataElementResponse{
				ID:           testAssetID,
				Name:         req.Name,
				DisplayName:  req.DisplayName,
				ResourceType: "Asset",
				Domain: clients.CreateDataElementRef{
					ID:   req.DomainID,
					Name: "Customer Data Domain",
				},
				Type: clients.CreateDataElementRef{
					ID:   req.TypeID,
					Name: "Data Element",
				},
				Status: &clients.CreateDataElementRef{
					ID:   req.StatusID,
					Name: "Approved",
				},
			}
		},
	))

	server := httptest.NewServer(mux)
	defer server.Close()

	client := testutil.NewClient(server)
	output, err := create_data_element.NewTool(client).Handler(t.Context(), create_data_element.Input{
		Name:        "Customer Email Address",
		DomainID:    testDomainID,
		DisplayName: "Customer Email",
		StatusID:    testStatusID,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if output.ID != testAssetID {
		t.Errorf("got ID %q, want %q", output.ID, testAssetID)
	}
	if output.ResourceType != "Asset" {
		t.Errorf("got ResourceType %q, want %q", output.ResourceType, "Asset")
	}
}

func TestCreateDataElement_MissingName(t *testing.T) {
	client := &http.Client{}
	_, err := create_data_element.NewTool(client).Handler(t.Context(), create_data_element.Input{
		DomainID: testDomainID,
	})
	if err == nil {
		t.Fatal("expected error for missing name, got nil")
	}
	if err.Error() != "name is required" {
		t.Errorf("got error %q, want %q", err.Error(), "name is required")
	}
}

func TestCreateDataElement_MissingDomainID(t *testing.T) {
	client := &http.Client{}
	_, err := create_data_element.NewTool(client).Handler(t.Context(), create_data_element.Input{
		Name: "Customer Email Address",
	})
	if err == nil {
		t.Fatal("expected error for missing domain_id, got nil")
	}
	if err.Error() != "domain_id is required" {
		t.Errorf("got error %q, want %q", err.Error(), "domain_id is required")
	}
}

func TestCreateDataElement_DuplicateNameConflict(t *testing.T) {
	mux := http.NewServeMux()
	mux.Handle("/rest/2.0/assets", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))

	server := httptest.NewServer(mux)
	defer server.Close()

	client := testutil.NewClient(server)
	_, err := create_data_element.NewTool(client).Handler(t.Context(), create_data_element.Input{
		Name:     "Duplicate Name",
		DomainID: testDomainID,
	})
	if err == nil {
		t.Fatal("expected error for duplicate name, got nil")
	}
}

func TestCreateDataElement_Unauthorized(t *testing.T) {
	mux := http.NewServeMux()
	mux.Handle("/rest/2.0/assets", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))

	server := httptest.NewServer(mux)
	defer server.Close()

	client := testutil.NewClient(server)
	_, err := create_data_element.NewTool(client).Handler(t.Context(), create_data_element.Input{
		Name:     "Customer Email Address",
		DomainID: testDomainID,
	})
	if err == nil {
		t.Fatal("expected error for unauthorized request, got nil")
	}
}

func TestCreateDataElement_Forbidden(t *testing.T) {
	mux := http.NewServeMux()
	mux.Handle("/rest/2.0/assets", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))

	server := httptest.NewServer(mux)
	defer server.Close()

	client := testutil.NewClient(server)
	_, err := create_data_element.NewTool(client).Handler(t.Context(), create_data_element.Input{
		Name:     "Customer Email Address",
		DomainID: testDomainID,
	})
	if err == nil {
		t.Fatal("expected error for forbidden request, got nil")
	}
}

func TestCreateDataElement_ServerError(t *testing.T) {
	mux := http.NewServeMux()
	mux.Handle("/rest/2.0/assets", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))

	server := httptest.NewServer(mux)
	defer server.Close()

	client := testutil.NewClient(server)
	_, err := create_data_element.NewTool(client).Handler(t.Context(), create_data_element.Input{
		Name:     "Customer Email Address",
		DomainID: testDomainID,
	})
	if err == nil {
		t.Fatal("expected error for server error, got nil")
	}
}

func TestCreateDataElement_SetsTypeID(t *testing.T) {
	var capturedTypeID string
	mux := http.NewServeMux()
	mux.Handle("/rest/2.0/assets", testutil.JsonHandlerInOut(
		func(_ *http.Request, req clients.CreateDataElementRequest) (int, clients.CreateDataElementResponse) {
			capturedTypeID = req.TypeID
			return http.StatusCreated, clients.CreateDataElementResponse{
				ID:           testAssetID,
				Name:         req.Name,
				ResourceType: "Asset",
			}
		},
	))

	server := httptest.NewServer(mux)
	defer server.Close()

	client := testutil.NewClient(server)
	_, err := create_data_element.NewTool(client).Handler(t.Context(), create_data_element.Input{
		Name:     "Test Element",
		DomainID: testDomainID,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if capturedTypeID != create_data_element.DataElementTypeID {
		t.Errorf("got typeId %q, want %q", capturedTypeID, create_data_element.DataElementTypeID)
	}
}
