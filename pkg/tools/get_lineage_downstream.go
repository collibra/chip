package tools

import (
	"context"
	"net/http"

	"github.com/collibra/chip/pkg/chip"
	"github.com/collibra/chip/pkg/clients"
)

type GetLineageDownstreamInput struct {
	EntityId   string `json:"entityId" jsonschema:"Required. The lineage entity ID to trace downstream from (obtained from search_lineage_entities). This is NOT a DGC asset UUID — to go from a catalog asset to a lineage entity ID, first call search_lineage_entities with the asset's UUID as dgcId."`
	EntityType string `json:"entityType,omitempty" jsonschema:"Optional. Filter to only include entities of this type (e.g. 'table', 'report'). Useful when you only care about specific downstream asset types."`
	Limit      int    `json:"limit,omitempty" jsonschema:"Optional. Max relations per page. Default: 20, Min: 1, Max: 100."`
	Cursor     string `json:"cursor,omitempty" jsonschema:"Optional. Pagination cursor from a previous response. Do not construct manually."`
}

func NewGetLineageDownstreamTool(collibraClient *http.Client) *chip.Tool[GetLineageDownstreamInput, LineageDirectionalOutput] {
	return &chip.Tool[GetLineageDownstreamInput, LineageDirectionalOutput]{
		Name: "get_lineage_downstream",
		Description: `WORKFLOW: Call this AFTER search_lineage_entities has given you an entity ID. This is the tool for impact analysis and tracing data consumers.
					  Use when the user asks: "what depends on this data?", "what uses this table?", "what breaks if this column changes?", "what reports use this data?", "what is the impact of changing this?".
					  Typical workflow: (1) search_lineage_entities to find the entity ID → (2) get_lineage_downstream with that ID → (3) optionally get_lineage_entity for the most relevant consumer entities only.
					  Returns: a paginated list of relations with an entities map that resolves all entity IDs to names and types. You can identify columns by name without additional calls.
					  Do not call get_lineage_transformation unless the user explicitly asks about the SQL or transformation logic.`,
		Handler:     handleGetLineageDownstream(collibraClient),
		Permissions: []string{},
	}
}

func handleGetLineageDownstream(collibraClient *http.Client) chip.ToolHandlerFunc[GetLineageDownstreamInput, LineageDirectionalOutput] {
	return func(ctx context.Context, input GetLineageDownstreamInput) (LineageDirectionalOutput, error) {
		return handleLineageDirectional(ctx, collibraClient, input.EntityId, input.EntityType, input.Limit, input.Cursor, clients.GetLineageDownstream)
	}
}
