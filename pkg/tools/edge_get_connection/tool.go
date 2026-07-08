package edge_get_connection

import (
	"context"
	"fmt"
	"net/http"

	"github.com/collibra/chip/pkg/chip"
	"github.com/collibra/chip/pkg/clients"
	"github.com/collibra/chip/pkg/tools/validation"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type Input struct {
	ConnectionId string `json:"connectionId" jsonschema:"the UUID of the connection to retrieve"`
}

type Output struct {
	Connection *clients.EdgeConnection `json:"connection,omitempty" jsonschema:"the connection details"`
	Found      bool                    `json:"found" jsonschema:"whether the connection was found"`
	Error      string                  `json:"error,omitempty" jsonschema:"error message if retrieval failed"`
}

func NewTool(collibraClient *http.Client) *chip.Tool[Input, Output] {
	return &chip.Tool[Input, Output]{
		Name:        "edge_get_connection",
		Description: "Gets an Edge connection by its UUID. Returns the connection's configuration, type, edge site, and parameters.",
		Handler:     handler(collibraClient),
		Permissions: []string{},
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}
}

func handler(collibraClient *http.Client) chip.ToolHandlerFunc[Input, Output] {
	return func(ctx context.Context, input Input) (Output, error) {
		if err := validation.UUID("connectionId", input.ConnectionId); err != nil {
			return Output{}, err
		}
		conn, err := clients.GetConnection(ctx, collibraClient, input.ConnectionId)
		if err != nil {
			return Output{Found: false, Error: fmt.Sprintf("failed to get connection: %s", err.Error())}, nil
		}
		return Output{Connection: conn, Found: true}, nil
	}
}
