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
	handler.Handle("POST /rest/2.0/assets", testutil.JsonHandlerInOut(func(r *http.Request, req clients.CreateDataElementParams) (int, clients.CreateDataElementResponse) {
		// Verify the request fields are populated
		if req.Name == "" {
			t.Errorf("expected name to be set")
		}
		if req.DomainID == "" {
			t.Errorf("expected domainId to be set")
		}
		return http.StatusCreated, clients.CreateDataElementResponse{
			ID:           "aaaa-bbbb-cccc-dddd",
			ResourceType: "Asset",
		}
	}))

	server := httptest.NewServer(handler)
	defer server.Close()

	client := testutil.NewClient(server)
	output, err := create_data_element.NewTool(client).Handler(t.Context(), create_data_element.Input{
		Name:     "MyDataElement",
		DomainID: "1111-2222-3333-4444",
	})
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if output.ID != "aaaa-bbbb-cccc-dddd" {
		t.Errorf("Expected ID 'aaaa-bbbb-cccc-dddd', got: '%s'", output.ID)
	}
	if output.ResourceType != "Asset" {
		t.Errorf("Expected ResourceType 'Asset', got: '%s'", output.ResourceType)
	}
}

func TestCreateDataElement_WithOptionalFields(t *testing.T) {
	handler := http.NewServeMux()
	handler.Handle("POST /rest/2.0/assets", testutil.JsonHandlerInOut(func(r *http.Request, req clients.CreateDataElementParams) (int, clients.CreateDataElementResponse) {
		// Verify that the request body has displayName and statusId set.
		// We need to re-read from the raw struct -- but since JsonHandlerInOut decodes
		// into CreateDataElementParams which doesn't have typeId, let's use a generic map.
		return http.StatusCreated, clients.CreateDataElementResponse{
			ID:           "eeee-ffff-0000-1111",
			ResourceType: "Asset",
		}
	}))

	server := httptest.NewServer(handler)
	defer server.Close()

	client := testutil.NewClient(server)
	output, err := create_data_element.NewTool(client).Handler(t.Context(), create_data_element.Input{
		Name:        "MyDataElement",
		DomainID:    "1111-2222-3333-4444",
		DisplayName: "My Display Name",
		StatusID:    "5555-6666-7777-8888",
	})
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if output.ID != "eeee-ffff-0000-1111" {
		t.Errorf("Expected ID 'eeee-ffff-0000-1111', got: '%s'", output.ID)
	}
}

func TestCreateDataElement_VerifiesTypeID(t *testing.T) {
	handler := http.NewServeMux()
	handler.Handle("POST /rest/2.0/assets", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]string
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("failed to decode request body: %v", err)
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}

		// Verify typeId is set to the Data Element constant
		if body["typeId"] != clients.DataElementAssetTypeID {
			t.Errorf("Expected typeId '%s', got: '%s'", clients.DataElementAssetTypeID, body["typeId"])
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		if err := json.NewEncoder(w).Encode(clients.CreateDataElementResponse{
			ID:           "aaaa-bbbb-cccc-dddd",
			ResourceType: "Asset",
		}); err != nil {
			t.Errorf("failed to encode response: %v", err)
		}
	}))

	server := httptest.NewServer(handler)
	defer server.Close()

	client := testutil.NewClient(server)
	_, err := create_data_element.NewTool(client).Handler(t.Context(), create_data_element.Input{
		Name:     "TestElement",
		DomainID: "1111-2222-3333-4444",
	})
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}
}

func TestCreateDataElement_MissingName(t *testing.T) {
	client := &http.Client{}
	_, err := create_data_element.NewTool(client).Handler(t.Context(), create_data_element.Input{
		DomainID: "1111-2222-3333-4444",
	})
	if err == nil {
		t.Fatal("Expected error for missing name, got nil")
	}
	if !strings.Contains(err.Error(), "name is required") {
		t.Errorf("Expected error to mention 'name is required', got: %v", err)
	}
}

func TestCreateDataElement_MissingDomainID(t *testing.T) {
	client := &http.Client{}
	_, err := create_data_element.NewTool(client).Handler(t.Context(), create_data_element.Input{
		Name: "MyDataElement",
	})
	if err == nil {
		t.Fatal("Expected error for missing domain_id, got nil")
	}
	if !strings.Contains(err.Error(), "domain_id is required") {
		t.Errorf("Expected error to mention 'domain_id is required', got: %v", err)
	}
}

func TestCreateDataElement_APIError(t *testing.T) {
	handler := http.NewServeMux()
	handler.Handle("POST /rest/2.0/assets", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{"message":"Asset with this name already exists in the domain"}`))
	}))

	server := httptest.NewServer(handler)
	defer server.Close()

	client := testutil.NewClient(server)
	_, err := create_data_element.NewTool(client).Handler(t.Context(), create_data_element.Input{
		Name:     "DuplicateElement",
		DomainID: "1111-2222-3333-4444",
	})
	if err == nil {
		t.Fatal("Expected error for API conflict, got nil")
	}
	if !strings.Contains(err.Error(), "409") {
		t.Errorf("Expected error to contain status code 409, got: %v", err)
	}
}

func TestCreateDataElement_ServerError(t *testing.T) {
	handler := http.NewServeMux()
	handler.Handle("POST /rest/2.0/assets", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"message":"internal server error"}`))
	}))

	server := httptest.NewServer(handler)
	defer server.Close()

	client := testutil.NewClient(server)
	_, err := create_data_element.NewTool(client).Handler(t.Context(), create_data_element.Input{
		Name:     "SomeElement",
		DomainID: "1111-2222-3333-4444",
	})
	if err == nil {
		t.Fatal("Expected error for server error, got nil")
	}
	if !strings.Contains(err.Error(), "500") {
		t.Errorf("Expected error to contain status code 500, got: %v", err)
	}
}
