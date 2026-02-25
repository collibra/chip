package tools_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/collibra/chip/pkg/clients"
	"github.com/collibra/chip/pkg/tools"
)

func TestAskGlossary(t *testing.T) {
	handler := http.NewServeMux()
	handler.Handle("/rest/aiCopilot/v1/tools/askGlossary", JsonHandlerInOut(func(_ *http.Request, request clients.ToolRequest) (int, clients.ToolResponse) {
		return http.StatusOK, clients.ToolResponse{
			Content: []clients.ToolContent{
				{Text: fmt.Sprintf("Q: %s, A: %s", request.Message.Content.Text, "Annual Recurring Revenue")},
			},
		}
	}))

	server := httptest.NewServer(handler)
	defer server.Close()

	client := newClient(server)
	output, err := tools.NewAskGlossaryTool(client).Handler(t.Context(), tools.AskGlossaryInput{
		Question: "What is the definition of ARR?",
	})
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	expectedAnswer := "Q: What is the definition of ARR?, A: Annual Recurring Revenue"
	if output.Answer != expectedAnswer {
		t.Fatalf("Expected answer '%s', got: '%s'", expectedAnswer, output.Answer)
	}
}
