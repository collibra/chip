package tools

import (
	"context"
	"net/http"

	"github.com/collibra/chip/pkg/chip"
	"github.com/collibra/chip/pkg/clients"
)

type GetLineageTransformationInput struct {
	TransformationId string `json:"transformationId" jsonschema:"Required. ID of the transformation to be fetched (e.g. '67890')."`
}

func NewGetLineageTransformationTool(collibraClient *http.Client) *chip.Tool[GetLineageTransformationInput, clients.GetLineageTransformationOutput] {
	return &chip.Tool[GetLineageTransformationInput, clients.GetLineageTransformationOutput]{
		Name:        "get_lineage_transformation",
		Description: "Get detailed information about a specific data transformation, including its SQL or script logic. A transformation represents a data processing activity (ETL job, SQL query, script, etc.) that connects source entities to target entities in the lineage graph. Use this when you found a transformation ID in an upstream/downstream lineage result and want to see what the transformation actually does -- the SQL query, script content, or processing logic.",
		Handler:     handleGetLineageTransformation(collibraClient),
		Permissions: []string{},
	}
}

func handleGetLineageTransformation(collibraClient *http.Client) chip.ToolHandlerFunc[GetLineageTransformationInput, clients.GetLineageTransformationOutput] {
	return func(ctx context.Context, input GetLineageTransformationInput) (clients.GetLineageTransformationOutput, error) {
		if input.TransformationId == "" {
			return clients.GetLineageTransformationOutput{Found: false, Error: "transformationId is required"}, nil
		}

		result, err := clients.GetLineageTransformation(ctx, collibraClient, input.TransformationId)
		if err != nil {
			return clients.GetLineageTransformationOutput{}, err
		}

		return *result, nil
	}
}
