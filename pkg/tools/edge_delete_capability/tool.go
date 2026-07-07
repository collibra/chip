package edge_delete_capability

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
	Id string `json:"id" jsonschema:"the UUID of the capability to delete"`
}

type Output struct {
	Success bool   `json:"success" jsonschema:"whether the capability was deleted"`
	Error   string `json:"error,omitempty" jsonschema:"error message if deletion failed"`
}

func NewTool(collibraClient *http.Client) *chip.Tool[Input, Output] {
	return &chip.Tool[Input, Output]{
		Name:        "edge_delete_capability",
		Description: "Deletes an Edge capability by its UUID.",
		Handler:     handler(collibraClient),
		Permissions: []string{},
		Annotations: &mcp.ToolAnnotations{DestructiveHint: chip.Ptr(true), IdempotentHint: true},
	}
}

func handler(collibraClient *http.Client) chip.ToolHandlerFunc[Input, Output] {
	return func(ctx context.Context, input Input) (Output, error) {
		if err := validation.UUID("id", input.Id); err != nil {
			return Output{}, err
		}
		if err := clients.DeleteCapability(ctx, collibraClient, input.Id); err != nil {
			return Output{Success: false, Error: fmt.Sprintf("failed to delete capability: %s", err.Error())}, nil
		}
		return Output{Success: true}, nil
	}
}
