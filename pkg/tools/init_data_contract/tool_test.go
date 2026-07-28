package init_data_contract_test

import (
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	tools "github.com/collibra/chip/pkg/tools/init_data_contract"
	"github.com/collibra/chip/pkg/tools/testutil"
)

const (
	governedAssetID = "00000000-0000-0000-0000-0000000000aa"
	domainID        = "00000000-0000-0000-0000-0000000000bb"
)

func TestInitDataContractWithManifest(t *testing.T) {
	manifestContent := `id: test-manifest-123
kind: DataContract
apiVersion: 1.0.3
title: Sample Data Contract`

	expectedResponse := `{
		"id": "00000000-0000-0000-0000-000000000001",
		"name": "Sample Data Contract",
		"manifestId": "test-manifest-123",
		"domainName": "Sales Data Products",
		"domainId": "00000000-0000-0000-0000-000000000002",
		"activeVersion": "0.0.1",
		"manifestVersion": {
			"version": "0.0.1",
			"active": true,
			"format": "ODCS",
			"createdBy": "00000000-0000-0000-0000-0000000000cc",
			"createdOn": 1476703764163,
			"lastModifiedBy": "00000000-0000-0000-0000-0000000000cc",
			"lastModifiedOn": 1476703764163
		}
	}`

	handler := http.NewServeMux()
	handler.Handle("/rest/dataProduct/v1/dataContracts", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		if !strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/form-data") {
			http.Error(w, "Expected multipart/form-data", http.StatusBadRequest)
			return
		}

		if err := r.ParseMultipartForm(10 << 20); err != nil {
			http.Error(w, "Failed to parse multipart form: "+err.Error(), http.StatusBadRequest)
			return
		}

		if r.FormValue("governedAssetId") != governedAssetID {
			http.Error(w, "Expected governedAssetId "+governedAssetID, http.StatusBadRequest)
			return
		}

		file, _, err := r.FormFile("manifest")
		if err != nil {
			http.Error(w, "Missing manifest file", http.StatusBadRequest)
			return
		}
		defer func(file multipart.File) {
			_ = file.Close()
		}(file)

		content, err := io.ReadAll(file)
		if err != nil {
			http.Error(w, "Failed to read manifest file", http.StatusBadRequest)
			return
		}

		if string(content) != manifestContent {
			http.Error(w, "Manifest content mismatch", http.StatusBadRequest)
			return
		}

		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(expectedResponse))
	}))

	server := httptest.NewServer(handler)
	defer server.Close()

	client := testutil.NewClient(server)
	output, err := tools.NewTool(client).Handler(t.Context(), tools.Input{
		GovernedAssetID: governedAssetID,
		Manifest:        manifestContent,
	})
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if !output.Success {
		t.Fatalf("Expected success to be true, error: %s", output.Error)
	}

	if output.ID != "00000000-0000-0000-0000-000000000001" {
		t.Fatalf("Expected ID '00000000-0000-0000-0000-000000000001', got: '%s'", output.ID)
	}

	if output.ManifestID != "test-manifest-123" {
		t.Fatalf("Expected ManifestID 'test-manifest-123', got: '%s'", output.ManifestID)
	}

	if output.ActiveVersion != "0.0.1" {
		t.Fatalf("Expected ActiveVersion '0.0.1', got: '%s'", output.ActiveVersion)
	}

	if output.Format != "ODCS" {
		t.Fatalf("Expected Format 'ODCS', got: '%s'", output.Format)
	}

	if output.DomainName != "Sales Data Products" {
		t.Fatalf("Expected DomainName 'Sales Data Products', got: '%s'", output.DomainName)
	}
}

func TestInitDataContractWithoutManifest(t *testing.T) {
	expectedResponse := `{
		"id": "00000000-0000-0000-0000-000000000003",
		"name": "My Port",
		"manifestId": "00000000-0000-0000-0000-000000000003",
		"domainName": "Sales Data Products",
		"domainId": "00000000-0000-0000-0000-0000000000bb",
		"activeVersion": "0.0.1",
		"manifestVersion": {
			"version": "0.0.1",
			"active": true,
			"format": "ODCS",
			"createdBy": "00000000-0000-0000-0000-0000000000cc",
			"createdOn": 1476703764163,
			"lastModifiedBy": "00000000-0000-0000-0000-0000000000cc",
			"lastModifiedOn": 1476703764163
		}
	}`

	handler := http.NewServeMux()
	handler.Handle("/rest/dataProduct/v1/dataContracts", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseMultipartForm(10 << 20); err != nil {
			http.Error(w, "Failed to parse multipart form", http.StatusBadRequest)
			return
		}

		if r.FormValue("governedAssetId") != governedAssetID {
			http.Error(w, "Expected governedAssetId "+governedAssetID, http.StatusBadRequest)
			return
		}

		if r.FormValue("domainId") != domainID {
			http.Error(w, "Expected domainId "+domainID, http.StatusBadRequest)
			return
		}

		if r.FormValue("name") != "My Port" {
			http.Error(w, "Expected name 'My Port'", http.StatusBadRequest)
			return
		}

		// No manifest file should be sent when Manifest is empty.
		if _, _, err := r.FormFile("manifest"); err == nil {
			http.Error(w, "Expected no manifest file", http.StatusBadRequest)
			return
		}

		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(expectedResponse))
	}))

	server := httptest.NewServer(handler)
	defer server.Close()

	client := testutil.NewClient(server)
	output, err := tools.NewTool(client).Handler(t.Context(), tools.Input{
		GovernedAssetID: governedAssetID,
		DomainID:        domainID,
		Name:            "My Port",
	})
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if !output.Success {
		t.Fatalf("Expected success to be true, error: %s", output.Error)
	}

	if output.ID != "00000000-0000-0000-0000-000000000003" {
		t.Fatalf("Expected ID '00000000-0000-0000-0000-000000000003', got: '%s'", output.ID)
	}
}

func TestInitDataContractInvalidGovernedAssetID(t *testing.T) {
	server := httptest.NewServer(http.NotFoundHandler())
	defer server.Close()

	client := testutil.NewClient(server)
	_, err := tools.NewTool(client).Handler(t.Context(), tools.Input{
		GovernedAssetID: "not-a-uuid",
	})
	if err == nil {
		t.Fatal("Expected validation error for invalid governedAssetId")
	}
	if !strings.Contains(err.Error(), "governedAssetId") {
		t.Fatalf("Expected error to mention governedAssetId, got: %v", err)
	}
}

func TestInitDataContractInvalidDomainID(t *testing.T) {
	server := httptest.NewServer(http.NotFoundHandler())
	defer server.Close()

	client := testutil.NewClient(server)
	_, err := tools.NewTool(client).Handler(t.Context(), tools.Input{
		GovernedAssetID: governedAssetID,
		DomainID:        "not-a-uuid",
	})
	if err == nil {
		t.Fatal("Expected validation error for invalid domainId")
	}
	if !strings.Contains(err.Error(), "domainId") {
		t.Fatalf("Expected error to mention domainId, got: %v", err)
	}
}

func TestInitDataContractServerError(t *testing.T) {
	handler := http.NewServeMux()
	handler.Handle("/rest/dataProduct/v1/dataContracts", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}))

	server := httptest.NewServer(handler)
	defer server.Close()

	client := testutil.NewClient(server)
	output, err := tools.NewTool(client).Handler(t.Context(), tools.Input{
		GovernedAssetID: governedAssetID,
	})
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if output.Success {
		t.Fatal("Expected success to be false")
	}

	if output.Error == "" {
		t.Fatal("Expected error message for server error")
	}
}
