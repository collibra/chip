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
	handler.Handle("/rest/2.0/assets", testutil.JsonHandlerInOut(
		func(_ *http.Request, req clients.CreateDataElementRequest) (int, clients.CreateDataElementResponse) {
			if req.Name != "test_element" {
				t.Errorf("expected name %q, got %q", "test_element", req.Name)
			}
			if req.DomainId != "d1234567-abcd-1234-abcd-1234567890ab" {
				t.Errorf("expected domainId %q, got %q", "d1234567-abcd-1234-abcd-1234567890ab", req.DomainId)
			}
			if req.TypeId != create_data_element.DataElementAssetTypeId {
				t.Errorf("expected typeId %q, got %q", create_data_element.DataElementAssetTypeId, req.TypeId)
			}
			return http.StatusCreated, clients.CreateDataElementResponse{
				Id:           "a1234567-abcd-1234-abcd-1234567890ab",
				ResourceType: "Asset",
				Name:         req.Name,
				DisplayName:  req.Name,
				CreatedOn:    1700000000000,
				CreatedBy:    "user-uuid",
			}
		},
	))

	server := httptest.NewServer(handler)
	defer server.Close()

	client := testutil.NewClient(server)
	output, err := create_data_element.NewTool(client).Handler(t.Context(), create_data_element.Input{
		Name:     "test_element",
		DomainId: "d1234567-abcd-1234-abcd-1234567890ab",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if output.Id != "a1234567-abcd-1234-abcd-1234567890ab" {
		t.Errorf("got id %q, want %q", output.Id, "a1234567-abcd-1234-abcd-1234567890ab")
	}
	if output.ResourceType != "Asset" {
		t.Errorf("got resource_type %q, want %q", output.ResourceType, "Asset")
	}
}

func TestCreateDataElement_WithOptionalFields(t *testing.T) {
	handler := http.NewServeMux()
	handler.Handle("/rest/2.0/assets", testutil.JsonHandlerInOut(
		func(_ *http.Request, req clients.CreateDataElementRequest) (int, clients.CreateDataElementResponse) {
			if req.DisplayName != "My Display Name" {
				t.Errorf("expected displayName %q, got %q", "My Display Name", req.DisplayName)
			}
			if req.StatusId != "s1234567-abcd-1234-abcd-1234567890ab" {
				t.Errorf("expected statusId %q, got %q", "s1234567-abcd-1234-abcd-1234567890ab", req.StatusId)
			}
			return http.StatusCreated, clients.CreateDataElementResponse{
				Id:           "b1234567-abcd-1234-abcd-1234567890ab",
				ResourceType: "Asset",
				Name:         req.Name,
				DisplayName:  req.DisplayName,
				CreatedOn:    1700000000000,
				CreatedBy:    "user-uuid",
			}
		},
	))

	server := httptest.NewServer(handler)
	defer server.Close()

	client := testutil.NewClient(server)
	output, err := create_data_element.NewTool(client).Handler(t.Context(), create_data_element.Input{
		Name:        "test_element",
		DomainId:    "d1234567-abcd-1234-abcd-1234567890ab",
		DisplayName: "My Display Name",
		StatusId:    "s1234567-abcd-1234-abcd-1234567890ab",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if output.Id != "b1234567-abcd-1234-abcd-1234567890ab" {
		t.Errorf("got id %q, want %q", output.Id, "b1234567-abcd-1234-abcd-1234567890ab")
	}
	if output.ResourceType != "Asset" {
		t.Errorf("got resource_type %q, want %q", output.ResourceType, "Asset")
	}
}

func TestCreateDataElement_MissingName(t *testing.T) {
	client := &http.Client{}
	_, err := create_data_element.NewTool(client).Handler(t.Context(), create_data_element.Input{
		DomainId: "d1234567-abcd-1234-abcd-1234567890ab",
	})
	if err == nil {
		t.Fatal("expected error for missing name, got nil")
	}
	if err.Error() != "name is required" {
		t.Errorf("got error %q, want %q", err.Error(), "name is required")
	}
}

func TestCreateDataElement_MissingDomainId(t *testing.T) {
	client := &http.Client{}
	_, err := create_data_element.NewTool(client).Handler(t.Context(), create_data_element.Input{
		Name: "test_element",
	})
	if err == nil {
		t.Fatal("expected error for missing domain_id, got nil")
	}
	if err.Error() != "domain_id is required" {
		t.Errorf("got error %q, want %q", err.Error(), "domain_id is required")
	}
}

func TestCreateDataElement_DuplicateNameConflict(t *testing.T) {
	handler := http.NewServeMux()
	handler.Handle("/rest/2.0/assets", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"errorCode":"DUPLICATE","message":"Asset with name 'test_element' already exists in domain"}`))
	}))

	server := httptest.NewServer(handler)
	defer server.Close()

	client := testutil.NewClient(server)
	_, err := create_data_element.NewTool(client).Handler(t.Context(), create_data_element.Input{
		Name:     "test_element",
		DomainId: "d1234567-abcd-1234-abcd-1234567890ab",
	})
	if err == nil {
		t.Fatal("expected error for duplicate name, got nil")
	}
	expected := `unexpected status 400: {"errorCode":"DUPLICATE","message":"Asset with name 'test_element' already exists in domain"}`
	if err.Error() != expected {
		t.Errorf("got error %q, want %q", err.Error(), expected)
	}
}

func TestCreateDataElement_ServerError(t *testing.T) {
	handler := http.NewServeMux()
	handler.Handle("/rest/2.0/assets", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`internal server error`))
	}))

	server := httptest.NewServer(handler)
	defer server.Close()

	client := testutil.NewClient(server)
	_, err := create_data_element.NewTool(client).Handler(t.Context(), create_data_element.Input{
		Name:     "test_element",
		DomainId: "d1234567-abcd-1234-abcd-1234567890ab",
	})
	if err == nil {
		t.Fatal("expected error for server error, got nil")
	}
}

func TestCreateDataElement_TypeIdIsSet(t *testing.T) {
	var capturedTypeId string
	handler := http.NewServeMux()
	handler.Handle("/rest/2.0/assets", testutil.JsonHandlerInOut(
		func(_ *http.Request, req clients.CreateDataElementRequest) (int, clients.CreateDataElementResponse) {
			capturedTypeId = req.TypeId
			return http.StatusCreated, clients.CreateDataElementResponse{
				Id:           "c1234567-abcd-1234-abcd-1234567890ab",
				ResourceType: "Asset",
				Name:         req.Name,
			}
		},
	))

	server := httptest.NewServer(handler)
	defer server.Close()

	client := testutil.NewClient(server)
	_, err := create_data_element.NewTool(client).Handler(t.Context(), create_data_element.Input{
		Name:     "test_element",
		DomainId: "d1234567-abcd-1234-abcd-1234567890ab",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if capturedTypeId != create_data_element.DataElementAssetTypeId {
		t.Errorf("got typeId %q, want %q", capturedTypeId, create_data_element.DataElementAssetTypeId)
	}
}
