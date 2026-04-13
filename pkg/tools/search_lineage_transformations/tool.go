package search_lineage_transformations

import (
	"context"
	"net/http"

	"github.com/collibra/chip/pkg/chip"
	"github.com/collibra/chip/pkg/clients"
)

type Input struct {
	NameContains string `json:"nameContains,omitempty" jsonschema:"Optional. Partial match on transformation name (case insensitive). Min: 1, Max: 256 chars. Example: 'etl'"`
	Limit        int    `json:"limit,omitempty" jsonschema:"Optional. Max results per page. Default: 20, Min: 1, Max: 100."`
	Cursor       string `json:"cursor,omitempty" jsonschema:"Optional. Pagination cursor from a previous response. Do not construct manually."`
}

func NewTool(collibraClient *http.Client) *chip.Tool[Input, clients.SearchLineageTransformationsOutput] {
	return &chip.Tool[Input, clients.SearchLineageTransformationsOutput]{
		Name:        "search_lineage_transformations",
		Description: "Search for transformations in the technical lineage graph by name. Returns a paginated list of matching transformation summaries. Use this to discover ETL jobs, SQL queries, or other processing activities without knowing their IDs. For example, find all transformations with \"etl\" or \"sales\" in the name. To see the full transformation logic (SQL/script), use get_lineage_transformation with the returned ID.",
		Handler:     handler(collibraClient),
		Permissions: []string{},
	}
}

func handler(collibraClient *http.Client) chip.ToolHandlerFunc[Input, clients.SearchLineageTransformationsOutput] {
	return func(ctx context.Context, input Input) (clients.SearchLineageTransformationsOutput, error) {
		result, err := clients.SearchLineageTransformations(ctx, collibraClient, input.NameContains, input.Limit, input.Cursor)
		if err != nil {
			return clients.SearchLineageTransformationsOutput{}, err
		}

		return *result, nil
	}
}
