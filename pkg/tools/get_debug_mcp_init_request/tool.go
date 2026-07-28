package get_debug_mcp_init_request

import (
	"context"
	"fmt"
	"net/http"

	"github.com/collibra/chip/pkg/chip"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type Input struct{}

type Output = mcp.InitializeParams

func NewTool(_ *http.Client) *chip.Tool[Input, Output] {
	return &chip.Tool[Input, Output]{
		Name:        "get_debug_mcp_init_request",
		Title:       "Get Debug MCP Init Request",
		Description: "Returns the MCP initialize request the connected client sent during the handshake, including protocolVersion, clientInfo, and declared capabilities (e.g. whether elicitation, sampling, or roots are supported). Useful for debugging MCP client/server compatibility.",
		Handler:     handler(),
		Permissions: []string{},
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}
}

func handler() chip.ToolHandlerFunc[Input, Output] {
	return func(ctx context.Context, _ Input) (Output, error) {
		params, ok := chip.GetInitParams(ctx)
		if !ok {
			return Output{}, fmt.Errorf("no InitializeParams available on context")
		}
		return *params, nil
	}
}
