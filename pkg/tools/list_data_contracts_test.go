package tools_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/collibra/chip/pkg/clients"
	"github.com/collibra/chip/pkg/tools"
	"github.com/google/uuid"
)

func TestListDataContracts(t *testing.T) {
	contractId, _ := uuid.NewUUID()
	domainId, _ := uuid.NewUUID()
	manifestId := "test-manifest-123"

	server := httptest.NewServer(&testServer{
		"/rest/dataProduct/v1/dataContracts": JsonHandlerOut(func(httpRequest *http.Request) clients.DataContractListPaginated {
			return clients.DataContractListPaginated{
				Items: []clients.DataContract{
					{
						ID:         contractId.String(),
						DomainID:   domainId.String(),
						ManifestID: manifestId,
					},
				},
				Limit:      100,
				NextCursor: "",
			}
		}),
	})
	defer server.Close()

	client := newClient(server)
	output, err := tools.NewListDataContractsTool(client).ToolHandler(t.Context(), tools.ListDataContractsInput{
		Limit: 100,
	})
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if output.Limit != 100 {
		t.Fatalf("Expected limit 100, got: %d", output.Limit)
	}

	if len(output.Contracts) != 1 {
		t.Fatalf("Expected 1 data contract, got: %d", len(output.Contracts))
	}

	contract := output.Contracts[0]
	if contract.ID != contractId.String() {
		t.Fatalf("Expected ID '%s', got: '%s'", contractId.String(), contract.ID)
	}

	if contract.DomainID != domainId.String() {
		t.Fatalf("Expected domainId '%s', got: '%s'", domainId.String(), contract.DomainID)
	}

	if contract.ManifestID != manifestId {
		t.Fatalf("Expected manifestId '%s', got: '%s'", manifestId, contract.ManifestID)
	}
}

func TestListDataContractsWithTotal(t *testing.T) {
	contractId, _ := uuid.NewUUID()
	domainId, _ := uuid.NewUUID()
	manifestId := "test-manifest-456"
	total := 42

	server := httptest.NewServer(&testServer{
		"/rest/dataProduct/v1/dataContracts": JsonHandlerOut(func(httpRequest *http.Request) clients.DataContractListPaginated {
			return clients.DataContractListPaginated{
				Items: []clients.DataContract{
					{
						ID:         contractId.String(),
						DomainID:   domainId.String(),
						ManifestID: manifestId,
					},
				},
				Limit:      100,
				NextCursor: "nextPageCursor",
				Total:      total,
			}
		}),
	})
	defer server.Close()

	client := newClient(server)
	output, err := tools.NewListDataContractsTool(client).ToolHandler(t.Context(), tools.ListDataContractsInput{
		Limit: 100,
	})
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if output.Total == nil {
		t.Fatal("Expected total to be present")
	}

	if *output.Total != 42 {
		t.Fatalf("Expected total 42, got: %d", *output.Total)
	}

	if output.NextCursor != "nextPageCursor" {
		t.Fatalf("Expected nextCursor 'nextPageCursor', got: '%s'", output.NextCursor)
	}
}
