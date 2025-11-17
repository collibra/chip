package tools_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/collibra/chip/pkg/tools"
)

func TestPushDataContractManifest(t *testing.T) {
	manifestContent := `id: test-manifest-123
kind: DataContract
apiVersion: 1.0.3
title: Sample Data Contract
description: This is a sample data contract manifest`

	expectedResponse := `{
		"id": "00000000-0000-0000-0000-000000000001",
		"domainId": "00000000-0000-0000-0000-000000000002",
		"manifestId": "test-manifest-123"
	}`

	server := httptest.NewServer(&testServer{
		"/rest/dataProduct/v1/dataContracts/addFromManifest": http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != "POST" {
				http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
				return
			}

			if !strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/form-data") {
				http.Error(w, "Expected multipart/form-data", http.StatusBadRequest)
				return
			}

			err := r.ParseMultipartForm(10 << 20) // 10 MB max
			if err != nil {
				http.Error(w, "Failed to parse multipart form: "+err.Error(), http.StatusBadRequest)
				return
			}

			file, _, err := r.FormFile("manifest")
			if err != nil {
				http.Error(w, "Missing manifest file", http.StatusBadRequest)
				return
			}
			defer file.Close()

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
			w.Write([]byte(expectedResponse))
		}),
	})
	defer server.Close()

	client := newClient(server)
	output, err := tools.NewPushDataContractManifestTool().ToolHandler(context.Background(), client, tools.PushDataContractManifestInput{
		Manifest: manifestContent,
	})
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if !output.Success {
		t.Fatal("Expected success to be true")
	}

	if output.Error != "" {
		t.Fatalf("Expected no error, got: %s", output.Error)
	}

	if output.ID != "00000000-0000-0000-0000-000000000001" {
		t.Fatalf("Expected ID '00000000-0000-0000-0000-000000000001', got: '%s'", output.ID)
	}

	if output.ManifestID != "test-manifest-123" {
		t.Fatalf("Expected ManifestID 'test-manifest-123', got: '%s'", output.ManifestID)
	}
}

func TestPushDataContractManifestWithOptionalParams(t *testing.T) {
	manifestContent := `id: test-manifest-456
kind: DataContract
apiVersion: 1.0.3
title: Another Data Contract`

	expectedResponse := `{
		"id": "00000000-0000-0000-0000-000000000003",
		"domainId": "00000000-0000-0000-0000-000000000004",
		"manifestId": "test-manifest-456"
	}`

	server := httptest.NewServer(&testServer{
		"/rest/dataProduct/v1/dataContracts/addFromManifest": http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			err := r.ParseMultipartForm(10 << 20)
			if err != nil {
				http.Error(w, "Failed to parse multipart form", http.StatusBadRequest)
				return
			}

			if r.FormValue("manifestId") != "test-manifest-456" {
				http.Error(w, "Expected manifestId 'test-manifest-456'", http.StatusBadRequest)
				return
			}

			if r.FormValue("version") != "1.0.0" {
				http.Error(w, "Expected version '1.0.0'", http.StatusBadRequest)
				return
			}

			if r.FormValue("force") != "true" {
				http.Error(w, "Expected force 'true'", http.StatusBadRequest)
				return
			}

			// active field should not be present when Active is false (implementation only sends it when true)
			if r.FormValue("active") != "" {
				http.Error(w, "Expected active field to be absent when false", http.StatusBadRequest)
				return
			}

			w.WriteHeader(http.StatusCreated)
			w.Write([]byte(expectedResponse))
		}),
	})
	defer server.Close()

	client := newClient(server)
	output, err := tools.NewPushDataContractManifestTool().ToolHandler(context.Background(), client, tools.PushDataContractManifestInput{
		Manifest:   manifestContent,
		ManifestID: "test-manifest-456",
		Version:    "1.0.0",
		Force:      true,
		Active:     false,
	})
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if !output.Success {
		t.Fatal("Expected success to be true")
	}

	if output.Error != "" {
		t.Fatalf("Expected no error, got: %s", output.Error)
	}
}

func TestPushDataContractManifestEmptyManifest(t *testing.T) {
	server := httptest.NewServer(&testServer{})
	defer server.Close()

	client := newClient(server)
	output, err := tools.NewPushDataContractManifestTool().ToolHandler(context.Background(), client, tools.PushDataContractManifestInput{
		Manifest: "",
	})
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if output.Success {
		t.Fatal("Expected success to be false")
	}

	if output.Error == "" {
		t.Fatal("Expected error message for empty manifest")
	}
}

func TestPushDataContractManifestServerError(t *testing.T) {
	manifestContent := `id: test-manifest-789
kind: DataContract`

	server := httptest.NewServer(&testServer{
		"/rest/dataProduct/v1/dataContracts/addFromManifest": http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "Internal server error", http.StatusInternalServerError)
		}),
	})
	defer server.Close()

	client := newClient(server)
	output, err := tools.NewPushDataContractManifestTool().ToolHandler(context.Background(), client, tools.PushDataContractManifestInput{
		Manifest: manifestContent,
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
