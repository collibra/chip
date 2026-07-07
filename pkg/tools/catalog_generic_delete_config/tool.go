package catalog_generic_delete_config

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
	IngestibleId string `json:"ingestibleId" jsonschema:"the UUID of the GENERIC integration instance whose configuration to delete"`
}

type Output struct {
	Success bool   `json:"success" jsonschema:"whether the configuration was deleted"`
	Error   string `json:"error,omitempty" jsonschema:"error message if deletion failed"`
}

func NewTool(collibraClient *http.Client) *chip.Tool[Input, Output] {
	return &chip.Tool[Input, Output]{
		Name:        "catalog_generic_delete_config",
		Description: "Deletes the configuration for a GENERIC integration instance.",
		Handler:     handler(collibraClient),
		Permissions: []string{},
		Annotations: &mcp.ToolAnnotations{DestructiveHint: chip.Ptr(true), IdempotentHint: true},
	}
}

func handler(collibraClient *http.Client) chip.ToolHandlerFunc[Input, Output] {
	return func(ctx context.Context, input Input) (Output, error) {
		if err := validation.UUID("ingestibleId", input.IngestibleId); err != nil {
			return Output{}, err
		}
		if err := clients.DeleteGenericConfig(ctx, collibraClient, input.IngestibleId); err != nil {
			return Output{Success: false, Error: fmt.Sprintf("failed to delete configuration: %s", err.Error())}, nil
		}
		return Output{Success: true}, nil
	}
}
