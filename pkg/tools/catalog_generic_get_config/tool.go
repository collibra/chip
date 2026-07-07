package catalog_generic_get_config

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
	IngestibleId string `json:"ingestibleId" jsonschema:"the UUID of the GENERIC integration instance"`
}

type Output struct {
	Config *clients.GenericConfiguration `json:"config,omitempty" jsonschema:"the integration configuration"`
	Found  bool                          `json:"found" jsonschema:"whether a configuration was found for this integration"`
	Error  string                        `json:"error,omitempty" jsonschema:"error message if retrieval failed"`
}

func NewTool(collibraClient *http.Client) *chip.Tool[Input, Output] {
	return &chip.Tool[Input, Output]{
		Name:        "catalog_generic_get_config",
		Description: "Retrieves the configuration for a GENERIC integration instance (e.g. Databricks Unity Catalog, Dataplex). The configuration value is a JSON string specific to the integration type.",
		Handler:     handler(collibraClient),
		Permissions: []string{},
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}
}

func handler(collibraClient *http.Client) chip.ToolHandlerFunc[Input, Output] {
	return func(ctx context.Context, input Input) (Output, error) {
		if err := validation.UUID("ingestibleId", input.IngestibleId); err != nil {
			return Output{}, err
		}
		config, err := clients.GetGenericConfig(ctx, collibraClient, input.IngestibleId)
		if err != nil {
			return Output{Found: false, Error: fmt.Sprintf("failed to get configuration: %s", err.Error())}, nil
		}
		return Output{Config: config, Found: true}, nil
	}
}
