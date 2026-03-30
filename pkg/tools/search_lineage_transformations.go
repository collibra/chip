package tools

import (
	"context"
	"net/http"

	"github.com/collibra/chip/pkg/chip"
	"github.com/collibra/chip/pkg/clients"
)

type SearchLineageTransformationsInput struct {
	NameContains string `json:"nameContains,omitempty" jsonschema:"Optional. Partial match on transformation name (case insensitive). Min: 1, Max: 256 chars. Example: 'etl'"`
	Limit        int    `json:"limit,omitempty" jsonschema:"Optional. Max results per page. Default: 20, Min: 1, Max: 100."`
	Cursor       string `json:"cursor,omitempty" jsonschema:"Optional. Pagination cursor from a previous response. Do not construct manually."`
}

func NewSearchLineageTransformationsTool(collibraClient *http.Client) *chip.Tool[SearchLineageTransformationsInput, clients.SearchLineageTransformationsOutput] {
	return &chip.Tool[SearchLineageTransformationsInput, clients.SearchLineageTransformationsOutput]{
		Name: "search_lineage_transformations",
		Description: `WORKFLOW: This is a SPECIALIZED tool — only use it when the user explicitly asks about a transformation by name (e.g. "find the ETL job called X"). This is NOT a general entry point for lineage questions.
					  For most lineage questions ("where does this data come from?", "what depends on this?"), start with search_lineage_entities instead — that is the correct entry point. Transformation IDs are normally obtained from get_lineage_upstream or get_lineage_downstream results, not from this search.
					  Use when the user asks: "find the transformation named X", "search for ETL jobs matching Y", "list transformations with 'sales' in the name". Returns paginated transformation summaries (ID and name). Use get_lineage_transformation with a returned ID to see the full SQL/logic.`,
		Handler:     handleSearchLineageTransformations(collibraClient),
		Permissions: []string{},
	}
}

func handleSearchLineageTransformations(collibraClient *http.Client) chip.ToolHandlerFunc[SearchLineageTransformationsInput, clients.SearchLineageTransformationsOutput] {
	return func(ctx context.Context, input SearchLineageTransformationsInput) (clients.SearchLineageTransformationsOutput, error) {
		result, err := clients.SearchLineageTransformations(ctx, collibraClient, input.NameContains, input.Limit, input.Cursor)
		if err != nil {
			return clients.SearchLineageTransformationsOutput{}, err
		}

		return *result, nil
	}
}
