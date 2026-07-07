package catalog_generic_get_schema

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
	Schema string `json:"schema,omitempty" jsonschema:"the data schema as a JSON string; describes the valid configuration fields for this integration"`
	Found  bool   `json:"found" jsonschema:"whether a schema was found"`
	Error  string `json:"error,omitempty" jsonschema:"error message if retrieval failed"`
}

func NewTool(collibraClient *http.Client) *chip.Tool[Input, Output] {
	return &chip.Tool[Input, Output]{
		Name:        "catalog_generic_get_schema",
		Description: "Retrieves the data schema for a GENERIC integration instance. The schema describes the valid configuration fields that can be used with catalog_generic_save_config.",
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
		schema, err := clients.GetGenericSchema(ctx, collibraClient, input.IngestibleId)
		if err != nil {
			return Output{Found: false, Error: fmt.Sprintf("failed to get schema: %s", err.Error())}, nil
		}
		return Output{Schema: schema, Found: true}, nil
	}
}
