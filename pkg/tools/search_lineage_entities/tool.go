package search_lineage_entities

import (
	"context"
	"net/http"

	"github.com/collibra/chip/pkg/chip"
	"github.com/collibra/chip/pkg/clients"
)

type Input struct {
	NameContains string `json:"nameContains,omitempty" jsonschema:"Optional. Partial match on entity name (case insensitive). Min: 1, Max: 256 chars. Example: 'sales'"`
	Type         string `json:"type,omitempty" jsonschema:"Optional. Exact match on entity type. Common types: table, column, file, report, apiEndpoint, topic. Example: 'table'"`
	DgcId        string `json:"dgcId,omitempty" jsonschema:"Optional. Filter by Data Governance Catalog UUID. Use to find the lineage entity linked to a specific Collibra catalog asset."`
	Limit        int    `json:"limit,omitempty" jsonschema:"Optional. Max results per page. Default: 20, Min: 1, Max: 100."`
	Cursor       string `json:"cursor,omitempty" jsonschema:"Optional. Pagination cursor from a previous response. Do not construct manually."`
}

func NewTool(collibraClient *http.Client) *chip.Tool[Input, clients.SearchLineageEntitiesOutput] {
	return &chip.Tool[Input, clients.SearchLineageEntitiesOutput]{
		Name:        "search_lineage_entities",
		Description: "Search for data entities in the technical lineage graph by name, type, or DGC identifier. Technical lineage covers all data objects across external systems -- including source code, transformations, and temporary tables -- regardless of whether they are registered in Collibra (unlike business lineage, which only covers assets ingested into the Data Catalog). Returns a paginated list of matching entities. This is typically the starting tool when you don't have a specific entity ID -- for example, to find all tables with \"sales\" in the name, or to find the lineage entity linked to a specific Collibra catalog asset via its DGC UUID. Supports partial name matching (case insensitive).",
		Handler:     handler(collibraClient),
		Permissions: []string{},
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
