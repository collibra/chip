package edge_list_connections

import (
	"context"
	"net/http"

	"github.com/collibra/chip/pkg/chip"
	"github.com/collibra/chip/pkg/clients"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type Input struct{}

type Output struct {
	Connections []clients.EdgeConnection `json:"connections" jsonschema:"list of Edge connections"`
	Count       int                  `json:"count" jsonschema:"number of connections returned"`
	Error       string               `json:"error,omitempty" jsonschema:"error message if the request failed"`
}

func NewTool(collibraClient *http.Client) *chip.Tool[Input, Output] {
	return &chip.Tool[Input, Output]{
		Name:        "edge_list_connections",
		Description: "Gets all available Edge connections. Returns the full list of connections configured on the Collibra Edge management service.",
		Handler:     handler(collibraClient),
		Permissions: []string{},
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}
}

func handler(collibraClient *http.Client) chip.ToolHandlerFunc[Input, Output] {
	return func(ctx context.Context, input Input) (Output, error) {
		conns, err := clients.ListConnections(ctx, collibraClient)
		if err != nil {
			return Output{Connections: []clients.EdgeConnection{}, Error: err.Error()}, nil
		}
		if conns == nil {
			conns = []clients.EdgeConnection{}
		}
		return Output{Connections: conns, Count: len(conns)}, nil
	}
}
