package discover_data_assets_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/collibra/chip/pkg/clients"
	tools "github.com/collibra/chip/pkg/tools/discover_data_assets"
	"github.com/collibra/chip/pkg/tools/testutil"
)

func TestAskDad(t *testing.T) {
	handler := http.NewServeMux()
	handler.Handle("/rest/aiCopilot/v1/tools/askDad", testutil.JsonHandlerInOut(func(_ *http.Request, request clients.ToolRequest) (int, clients.ToolResponse) {
		return http.StatusOK, clients.ToolResponse{
			Content: []clients.ToolContent{
				{Text: fmt.Sprintf("Q: %s, A: %s", request.Message.Content.Text, "Name, Email, Phone Number")},
			},
		}
	}))

	server := httptest.NewServer(handler)
	defer server.Close()

	client := testutil.NewClient(server)
	output, err := tools.NewTool(client).Handler(t.Context(), tools.Input{
		Question: "Column names with PII in table users?",
	})
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	expectedAnswer := "Q: Column names with PII in table users?, A: Name, Email, Phone Number"
	if output.Answer != expectedAnswer {
		t.Fatalf("Expected answer '%s', got: '%s'", expectedAnswer, output.Answer)
	}
}
