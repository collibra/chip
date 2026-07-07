package edge_delete_connection

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
	ConnectionId string `json:"connectionId" jsonschema:"the UUID of the connection to delete"`
}

type Output struct {
	Success bool   `json:"success" jsonschema:"whether the connection was deleted"`
	Error   string `json:"error,omitempty" jsonschema:"error message if deletion failed"`
}

func NewTool(collibraClient *http.Client) *chip.Tool[Input, Output] {
	return &chip.Tool[Input, Output]{
		Name:        "edge_delete_connection",
		Description: "Deletes an Edge connection by its UUID.",
		Handler:     handler(collibraClient),
		Permissions: []string{},
		Annotations: &mcp.ToolAnnotations{DestructiveHint: chip.Ptr(true), IdempotentHint: true},
	}
}

func handler(collibraClient *http.Client) chip.ToolHandlerFunc[Input, Output] {
	return func(ctx context.Context, input Input) (Output, error) {
		if err := validation.UUID("connectionId", input.ConnectionId); err != nil {
			return Output{}, err
		}
		if err := clients.DeleteConnection(ctx, collibraClient, input.ConnectionId); err != nil {
			return Output{Success: false, Error: fmt.Sprintf("failed to delete connection: %s", err.Error())}, nil
		}
		return Output{Success: true}, nil
	}
}
