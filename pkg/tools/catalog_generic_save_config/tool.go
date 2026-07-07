package catalog_generic_save_config

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
	IngestibleId  string `json:"ingestibleId" jsonschema:"the UUID of the GENERIC integration instance"`
	Configuration string `json:"configuration" jsonschema:"the configuration JSON string to save"`
}

type Output struct {
	Config  *clients.GenericConfiguration `json:"config,omitempty" jsonschema:"the saved configuration"`
	Success bool                          `json:"success" jsonschema:"whether the configuration was saved successfully"`
	Error   string                        `json:"error,omitempty" jsonschema:"error message if saving failed"`
}

func NewTool(collibraClient *http.Client) *chip.Tool[Input, Output] {
	return &chip.Tool[Input, Output]{
		Name:        "catalog_generic_save_config",
		Description: "Creates or updates the configuration for a GENERIC integration instance. The configuration is a JSON string whose schema depends on the integration type. Use catalog_generic_get_schema to discover valid fields.",
		Handler:     handler(collibraClient),
		Permissions: []string{},
		Annotations: &mcp.ToolAnnotations{DestructiveHint: chip.Ptr(true)},
	}
}

func handler(collibraClient *http.Client) chip.ToolHandlerFunc[Input, Output] {
	return func(ctx context.Context, input Input) (Output, error) {
		if err := validation.UUID("ingestibleId", input.IngestibleId); err != nil {
			return Output{}, err
		}
		config, err := clients.SaveGenericConfig(ctx, collibraClient, input.IngestibleId, clients.SaveGenericConfigRequest{
			Configuration: input.Configuration,
		})
		if err != nil {
			return Output{Success: false, Error: fmt.Sprintf("failed to save configuration: %s", err.Error())}, nil
		}
		return Output{Config: config, Success: true}, nil
	}
}
