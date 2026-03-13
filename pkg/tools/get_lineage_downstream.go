package tools

import (
	"context"
	"net/http"

	"github.com/collibra/chip/pkg/chip"
	"github.com/collibra/chip/pkg/clients"
)

type GetLineageDownstreamInput struct {
	EntityId   string `json:"entityId" jsonschema:"Required. ID of the entity to trace downstream from. Can be numeric string or DGC UUID."`
	EntityType string `json:"entityType,omitempty" jsonschema:"Optional. Filter to only include entities of this type (e.g. 'table', 'report'). Useful when you only care about specific downstream asset types."`
	Limit      int    `json:"limit,omitempty" jsonschema:"Optional. Max relations per page. Default: 20, Min: 1, Max: 100."`
	Cursor     string `json:"cursor,omitempty" jsonschema:"Optional. Pagination cursor from a previous response. Do not construct manually."`
}

func NewGetLineageDownstreamTool(collibraClient *http.Client) *chip.Tool[GetLineageDownstreamInput, clients.GetLineageDirectionalOutput] {
	return &chip.Tool[GetLineageDownstreamInput, clients.GetLineageDirectionalOutput]{
		Name:        "get_lineage_downstream",
		Description: "Get the downstream technical lineage graph for a data entity -- all direct and indirect consumer entities that are impacted by it, along with the transformations connecting them. This traces through all data objects across external systems (including unregistered assets, temporary tables, and source code), not just assets in the Collibra Data Catalog. Use this to answer \"What depends on this data?\" or \"If this table changes, what else is affected?\" Essential for impact analysis before modifying or deprecating a data asset. Results are paginated.",
		Handler:     handleGetLineageDownstream(collibraClient),
		Permissions: []string{},
	}
}

func handleGetLineageDownstream(collibraClient *http.Client) chip.ToolHandlerFunc[GetLineageDownstreamInput, clients.GetLineageDirectionalOutput] {
	return func(ctx context.Context, input GetLineageDownstreamInput) (clients.GetLineageDirectionalOutput, error) {
		if input.EntityId == "" {
			return clients.GetLineageDirectionalOutput{Error: "entityId is required"}, nil
		}

		result, err := clients.GetLineageDownstream(ctx, collibraClient, input.EntityId, input.EntityType, input.Limit, input.Cursor)
		if err != nil {
			return clients.GetLineageDirectionalOutput{}, err
		}

		return *result, nil
	}
}
