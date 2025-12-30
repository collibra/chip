package chip

import (
	"context"
	"log"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type toolInput struct {
	Input string `json:"input" jsonschema:"the input to tool"`
}

type toolOutput struct {
	Output string `json:"output" jsonschema:"the output from tool"`
}

func newTool() *Tool[toolInput, toolOutput] {
	return &Tool[toolInput, toolOutput]{
		Name:        "the_tool",
		Description: "The tool.",
		Handler:     handleTool(),
	}
}

func handleTool() ToolHandlerFunc[toolInput, toolOutput] {
	return func(ctx context.Context, input toolInput) (toolOutput, error) {
		return toolOutput{Output: input.Input}, nil
	}
}

func TestTool_IgnoreUnknownFields(t *testing.T) {
	chipServer := NewServer()
	RegisterTool[toolInput, toolOutput](chipServer, newTool())
	chipSession := newChipSession(t.Context(), chipServer)
	defer closeSilently(chipSession)

	_, err := chipSession.CallTool(t.Context(), &mcp.CallToolParams{
		Name: "the_tool",
		Arguments: map[string]any{
			"input":          "Echo this message",
			"tool_reasoning": "This should be ignored",
		},
	})
	if err != nil {
		t.Fatalf("should not return error when extra fields in arguments: %v", err)
	}
}

func newChipSession(ctx context.Context, chipServer *Server) *mcp.ClientSession {
	t1, t2 := mcp.NewInMemoryTransports()
	if _, err := chipServer.Connect(ctx, t1, nil); err != nil {
		log.Fatal(err)
	}
	chipClient := mcp.NewClient(&mcp.Implementation{Name: "client", Version: "v0.0.1"}, nil)
	chipSession, err := chipClient.Connect(ctx, t2, nil)
	if err != nil {
		log.Fatal(err)
	}
	return chipSession
}

func closeSilently(session *mcp.ClientSession) {
	_ = session.Close()
}
