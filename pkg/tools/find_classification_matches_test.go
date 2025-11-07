package tools_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/collibra/chip/pkg/clients"
	"github.com/collibra/chip/pkg/tools"
)

func TestFindClassificationMatches(t *testing.T) {
	server := httptest.NewServer(&testServer{
		"/rest/catalog/1.0/dataClassification/classificationMatches/bulk": JsonHandlerOut(func(httpRequest *http.Request) clients.PagedResponseClassificationMatch {
			return clients.PagedResponseClassificationMatch{
				Total:  1,
				Offset: 0,
				Limit:  50,
				Results: []clients.ClassificationMatch{
					{
						ID:     "test-match-id",
						Status: "ACCEPTED",
						Asset: clients.NamedResourceReference{
							ID:   "asset-id",
							Name: "Test Asset",
						},
						Classification: clients.Classification{
							ID:   "classification-id",
							Name: "Test Classification",
						},
					},
				},
			}
		}),
	})
	defer server.Close()

	client := newClient(server)
	output, err := tools.NewFindClassificationMatchesTool().ToolHandler(t.Context(), client, tools.FindClassificationMatchesInput{
		Statuses: []string{"ACCEPTED"},
		Limit:    50,
	})
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if len(output.ClassificationMatches) != 1 {
		t.Fatalf("Expected 1 classification match, got: %d", len(output.ClassificationMatches))
	}

	match := output.ClassificationMatches[0]
	if match.Status != "ACCEPTED" {
		t.Fatalf("Expected status 'ACCEPTED', got: '%s'", match.Status)
	}

	if match.Asset.Name != "Test Asset" {
		t.Fatalf("Expected asset name 'Test Asset', got: '%s'", match.Asset.Name)
	}

	if match.Classification.Name != "Test Classification" {
		t.Fatalf("Expected classification name 'Test Classification', got: '%s'", match.Classification.Name)
	}
}
