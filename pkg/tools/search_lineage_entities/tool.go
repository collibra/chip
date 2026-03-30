package search_lineage_entities

import (
	"context"
	"net/http"

	"github.com/collibra/chip/pkg/chip"
	"github.com/collibra/chip/pkg/clients"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type Input struct {
	NameContains string `json:"nameContains,omitempty" jsonschema:"Optional. Partial match on entity name (case insensitive). Min: 1, Max: 256 chars. Example: 'sales'"`
	Type         string `json:"type,omitempty" jsonschema:"Optional. Exact match on entity type. Common types: table, column, file, report, apiEndpoint, topic. Example: 'table'"`
	DgcId        string `json:"dgcId,omitempty" jsonschema:"Optional. Filter by Data Governance Catalog UUID. Use to find the lineage entity linked to a specific Collibra catalog asset. Tip: you can pass the 'assetId' from get_asset_details or discover_data_assets here to bridge from the catalog to the lineage graph."`
	Limit        int    `json:"limit,omitempty" jsonschema:"Optional. Max results per page. Default: 20, Min: 1, Max: 100."`
	Cursor       string `json:"cursor,omitempty" jsonschema:"Optional. Pagination cursor from a previous response. Do not construct manually."`
}

func NewTool(collibraClient *http.Client) *chip.Tool[Input, clients.SearchLineageEntitiesOutput] {
	return &chip.Tool[Input, clients.SearchLineageEntitiesOutput]{
		Name: "search_lineage_entities",
		Description: `WORKFLOW: This is the ENTRY POINT for almost all lineage questions. Call this first to find entity IDs before using any other lineage tool.
					  Use when the user asks: "where does this data come from?", "what columns are in this report?", "what feeds into this table?", "what depends on this dataset?". Start here to resolve the entity name to an ID.
					  Searches the technical lineage graph (all data objects across external systems, including unregistered assets, temporary tables, and source code — not just Collibra catalog assets). Supports partial name matching (case insensitive), type filtering (table, column, file, report, apiEndpoint, topic), and DGC UUID lookup. Returns entity IDs and names (paginated).
					  LIMITATIONS — Column-level lineage lookups: Columns cannot be searched by name (nameContains does not work for columns). The dgcId parameter also does not reliably resolve columns because there is no consistent mapping between Collibra catalog column UUIDs and technical lineage entity IDs. To reach a column in the lineage graph, first find its parent table (by name or dgcId), then use get_lineage_upstream or get_lineage_downstream on the table to discover its columns in the lineage graph.
					  NEXT STEPS: Use the returned entity ID with get_lineage_upstream (to trace sources) or get_lineage_downstream (to trace consumers). Do not call get_lineage_entity unless you need metadata not already in the search results.`,
		Handler:     handler(collibraClient),
		Permissions: []string{},
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}
}

func handler(collibraClient *http.Client) chip.ToolHandlerFunc[Input, clients.SearchLineageEntitiesOutput] {
	return func(ctx context.Context, input Input) (clients.SearchLineageEntitiesOutput, error) {
		result, err := clients.SearchLineageEntities(ctx, collibraClient, input.NameContains, input.Type, input.DgcId, input.Limit, input.Cursor)
		if err != nil {
			return clients.SearchLineageEntitiesOutput{}, err
		}

		return *result, nil
	}
}
