package tools_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/collibra/chip/pkg/tools"
	"github.com/google/uuid"
)

func TestPullDataContractManifest(t *testing.T) {
	contractId, _ := uuid.NewUUID()
	manifestContent := `id: test-manifest-123
kind: DataContract
apiVersion: 1.0.3
title: Sample Data Contract
description: This is a sample data contract manifest`

	server := httptest.NewServer(&testServer{
		"/rest/dataProduct/v1/dataContracts/" + contractId.String() + "/activeVersion/manifest": StringHandlerOut(func(r *http.Request) string {
			return manifestContent
		}),
	})
	defer server.Close()

	client := newClient(server)
	output, err := tools.NewPullDataContractManifestTool(client).ToolHandler(context.Background(), tools.PullDataContractManifestInput{
		DataContractID: contractId.String(),
	})
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if !output.Found {
		t.Fatal("Expected manifest to be found")
	}

	if output.Error != "" {
		t.Fatalf("Expected no error, got: %s", output.Error)
	}

	if output.Manifest != manifestContent {
		t.Fatalf("Expected manifest content '%s', got: '%s'", manifestContent, output.Manifest)
	}
}

func TestPullDataContractManifestInvalidUUID(t *testing.T) {
	server := httptest.NewServer(&testServer{})
	defer server.Close()

	client := newClient(server)
	output, err := tools.NewPullDataContractManifestTool(client).ToolHandler(context.Background(), tools.PullDataContractManifestInput{
		DataContractID: "invalid-uuid",
	})
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if output.Found {
		t.Fatal("Expected manifest not to be found")
	}

	if output.Error == "" {
		t.Fatal("Expected error message for invalid UUID")
	}
}

func TestPullDataContractManifestNotFound(t *testing.T) {
	contractId, _ := uuid.NewUUID()

	server := httptest.NewServer(&testServer{
		"/rest/dataProduct/v1/dataContracts/" + contractId.String() + "/activeVersion/manifest": http.NotFoundHandler(),
	})
	defer server.Close()

	client := newClient(server)
	output, err := tools.NewPullDataContractManifestTool(client).ToolHandler(context.Background(), tools.PullDataContractManifestInput{
		DataContractID: contractId.String(),
	})
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if output.Found {
		t.Fatal("Expected manifest not to be found")
	}

	if output.Error == "" {
		t.Fatal("Expected error message for not found")
	}
}
