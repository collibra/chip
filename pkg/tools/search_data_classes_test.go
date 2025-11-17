package tools_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/collibra/chip/pkg/clients"
	"github.com/collibra/chip/pkg/tools"
)

func TestFindDataClasses(t *testing.T) {
	server := httptest.NewServer(&testServer{
		"/rest/classification/v1/dataClasses": JsonHandlerOut(func(httpRequest *http.Request) clients.DataClassesResponse {
			return clients.DataClassesResponse{
				Results: []clients.DataClass{{Description: httpRequest.URL.Query().Encode()}},
			}
		}),
	})
	defer server.Close()

	client := newClient(server)
	output, err := tools.NewSearchDataClassesTool().ToolHandler(t.Context(), client, tools.SearchDataClassesInput{
		Name: "Question",
	})
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if len(output.DataClasses) != 1 {
		t.Fatalf("Expected 1 data class, got: %d", len(output.DataClasses))
	}
	dataClass := output.DataClasses[0]
	expectedAnswer := "name=Question"
	if dataClass.Description != expectedAnswer {
		t.Fatalf("Expected answer '%s', got: '%s'", expectedAnswer, dataClass.Description)
	}
}
