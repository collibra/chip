package get_lineage_entity

import (
	"context"
	"net/http"

	"github.com/collibra/chip/pkg/chip"
	"github.com/collibra/chip/pkg/clients"
)

type Input struct {
	EntityId string `json:"entityId" jsonschema:"Required. Unique identifier of the data entity. Can be a numeric string (e.g. '12345') or a DGC UUID (e.g. '550e8400-e29b-41d4-a716-446655440000')."`
}

func NewTool(collibraClient *http.Client) *chip.Tool[Input, clients.GetLineageEntityOutput] {
	return &chip.Tool[Input, clients.GetLineageEntityOutput]{
		Name:        "get_lineage_entity",
		Description: "Get detailed metadata about a specific data entity in the technical lineage graph. Technical lineage covers all data objects across external systems -- including source code, transformations, and temporary tables -- regardless of whether they are registered in Collibra (unlike business lineage, which only covers assets ingested into the Data Catalog). An entity represents any tracked data asset such as a table, column, file, report, API endpoint, or topic. Returns the entity's name, type, source systems, parent entity, and linked Data Governance Catalog (DGC) identifier. Use this when you have an entity ID from a lineage traversal, search result, or user input and need its full details.",
		Handler:     handler(collibraClient),
		Permissions: []string{},
	}
}

func handler(collibraClient *http.Client) chip.ToolHandlerFunc[Input, clients.GetLineageEntityOutput] {
	return func(ctx context.Context, input Input) (clients.GetLineageEntityOutput, error) {
		if input.EntityId == "" {
			return clients.GetLineageEntityOutput{Found: false, Error: "entityId is required"}, nil
		}

		result, err := clients.GetLineageEntity(ctx, collibraClient, input.EntityId)
		if err != nil {
			return clients.GetLineageEntityOutput{}, err
		}

		return *result, nil
	}
}
