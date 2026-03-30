package get_lineage_transformation

import (
	"context"
	"net/http"

	"github.com/collibra/chip/pkg/chip"
	"github.com/collibra/chip/pkg/clients"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type Input struct {
	TransformationId string `json:"transformationId" jsonschema:"Required. ID of the transformation to be fetched (e.g. '67890')."`
}

func NewTool(collibraClient *http.Client) *chip.Tool[Input, clients.GetLineageTransformationOutput] {
	return &chip.Tool[Input, clients.GetLineageTransformationOutput]{
		Name: "get_lineage_transformation",
		Description: `WORKFLOW: This is a TERMINAL tool — only call it when the user explicitly wants to see the actual SQL, script, or transformation logic. Requires a transformation ID from a prior get_lineage_upstream or get_lineage_downstream result.
					  Use when the user asks: "show me the SQL", "what logic transforms this data?", "how is this ETL job defined?".
					  Do NOT call this just to understand the lineage graph — get_lineage_upstream and get_lineage_downstream already show which transformations connect entities, which is sufficient for most lineage questions. Only call this when the user wants the actual code or logic.
					  Do NOT call search_lineage_transformations to find a transformation ID if you already have it from upstream/downstream results.`,
		Handler:     handler(collibraClient),
		Permissions: []string{},
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}
}

func handler(collibraClient *http.Client) chip.ToolHandlerFunc[Input, clients.GetLineageTransformationOutput] {
	return func(ctx context.Context, input Input) (clients.GetLineageTransformationOutput, error) {
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
