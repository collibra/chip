package tools_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/collibra/chip/pkg/clients"
	"github.com/collibra/chip/pkg/tools"
)

func TestAskDad(t *testing.T) {
	handler := http.NewServeMux()
	handler.Handle("/rest/aiCopilot/v1/tools/askDad", JsonHandlerInOut(func(_ *http.Request, request clients.ToolRequest) (int, clients.ToolResponse) {
		return http.StatusOK, clients.ToolResponse{
			Content: []clients.ToolContent{
				{Text: fmt.Sprintf("Q: %s, A: %s", request.Message.Content.Text, "Name, Email, Phone Number")},
			},
		}
	}))

	server := httptest.NewServer(handler)
	defer server.Close()

	client := newClient(server)
	output, err := tools.NewAskDadTool(client).Handler(t.Context(), tools.AskDadInput{
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
