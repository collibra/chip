package ask_glossary_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/collibra/chip/pkg/clients"
	"github.com/collibra/chip/pkg/tools/ask_glossary"
	"github.com/collibra/chip/pkg/tools/testutil"
)

func TestAskGlossary(t *testing.T) {
	handler := http.NewServeMux()
	handler.Handle("/rest/aiCopilot/v1/tools/askGlossary", testutil.JsonHandlerInOut(func(_ *http.Request, request clients.ToolRequest) (int, clients.ToolResponse) {
		return http.StatusOK, clients.ToolResponse{
			Content: []clients.ToolContent{
				{Text: fmt.Sprintf("Q: %s, A: %s", request.Message.Content.Text, "Annual Recurring Revenue")},
			},
		}
	}))

	server := httptest.NewServer(handler)
	defer server.Close()

	client := testutil.NewClient(server)
	output, err := ask_glossary.NewTool(client).Handler(t.Context(), ask_glossary.Input{
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
