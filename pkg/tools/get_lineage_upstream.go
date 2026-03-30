package tools

import (
	"context"
	"net/http"

	"github.com/collibra/chip/pkg/chip"
	"github.com/collibra/chip/pkg/clients"
)

type GetLineageUpstreamInput struct {
	EntityId   string `json:"entityId" jsonschema:"Required. The lineage entity ID to trace upstream from (obtained from search_lineage_entities). This is NOT a DGC asset UUID — to go from a catalog asset to a lineage entity ID, first call search_lineage_entities with the asset's UUID as dgcId."`
	EntityType string `json:"entityType,omitempty" jsonschema:"Optional. Filter to only include entities of this type (e.g. 'table', 'column'). Useful when you only care about specific upstream asset types."`
	Limit      int    `json:"limit,omitempty" jsonschema:"Optional. Max relations per page. Default: 20, Min: 1, Max: 100."`
	Cursor     string `json:"cursor,omitempty" jsonschema:"Optional. Pagination cursor from a previous response. Do not construct manually."`
}

func NewGetLineageUpstreamTool(collibraClient *http.Client) *chip.Tool[GetLineageUpstreamInput, clients.GetLineageDirectionalOutput] {
	return &chip.Tool[GetLineageUpstreamInput, clients.GetLineageDirectionalOutput]{
		Name: "get_lineage_upstream",
		Description: `WORKFLOW: Call this AFTER search_lineage_entities has given you an entity ID. This is the tool for tracing data sources.
					  Use when the user asks: "where does this data come from?", "what are the sources for this table?", "how is this column calculated?", "what feeds into this report?".
					  Typical workflow: (1) search_lineage_entities to find the entity ID → (2) get_lineage_upstream with that ID → (3) optionally get_lineage_entity for the most relevant source entities only.
					  Returns: a paginated list of relations, each connecting a source entity ID to the target through transformation IDs. Results contain IDs only — summarize what you can from the graph structure and only call get_lineage_entity for entities the user specifically needs details on.
					  Do not call get_lineage_transformation unless the user explicitly asks about the SQL or transformation logic. The upstream graph already shows which transformations connect entities.`,
		Handler:     handleGetLineageUpstream(collibraClient),
		Permissions: []string{},
	}
}

func handleGetLineageUpstream(collibraClient *http.Client) chip.ToolHandlerFunc[GetLineageUpstreamInput, clients.GetLineageDirectionalOutput] {
	return func(ctx context.Context, input GetLineageUpstreamInput) (clients.GetLineageDirectionalOutput, error) {
		return handleLineageDirectional(ctx, collibraClient, input.EntityId, input.EntityType, input.Limit, input.Cursor, clients.GetLineageUpstream)
	}
}

// handleLineageDirectional is a shared helper for the upstream and downstream tool handlers.
func handleLineageDirectional(
	ctx context.Context,
	collibraClient *http.Client,
	entityId, entityType string,
	limit int,
	cursor string,
	fetch func(context.Context, *http.Client, string, string, int, string) (*clients.GetLineageDirectionalOutput, error),
) (clients.GetLineageDirectionalOutput, error) {
	if entityId == "" {
		return clients.GetLineageDirectionalOutput{Error: "entityId is required"}, nil
	}
	result, err := fetch(ctx, collibraClient, entityId, entityType, limit, cursor)
	if err != nil {
		return clients.GetLineageDirectionalOutput{}, err
	}
	return *result, nil
}
