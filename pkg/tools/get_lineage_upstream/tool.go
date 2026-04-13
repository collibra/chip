package get_lineage_upstream

import (
	"context"
	"net/http"

	"github.com/collibra/chip/pkg/chip"
	"github.com/collibra/chip/pkg/clients"
)

type Input struct {
	EntityId   string `json:"entityId" jsonschema:"Required. ID of the entity to trace upstream from. Can be numeric string or DGC UUID."`
	EntityType string `json:"entityType,omitempty" jsonschema:"Optional. Filter to only include entities of this type (e.g. 'table', 'column'). Useful when you only care about specific upstream asset types."`
	Limit      int    `json:"limit,omitempty" jsonschema:"Optional. Max relations per page. Default: 20, Min: 1, Max: 100."`
	Cursor     string `json:"cursor,omitempty" jsonschema:"Optional. Pagination cursor from a previous response. Do not construct manually."`
}

func NewTool(collibraClient *http.Client) *chip.Tool[Input, clients.GetLineageDirectionalOutput] {
	return &chip.Tool[Input, clients.GetLineageDirectionalOutput]{
		Name:        "get_lineage_upstream",
		Description: "Get the upstream technical lineage graph for a data entity -- all direct and indirect source entities that feed data into it, along with the transformations connecting them. This traces through all data objects across external systems (including unregistered assets, temporary tables, and source code), not just assets in the Collibra Data Catalog. Use this to answer \"Where does this data come from?\" or \"What are the sources feeding this table?\" Each relation in the result connects a source entity to a target entity through one or more transformations. Results are paginated.",
		Handler:     handler(collibraClient),
		Permissions: []string{},
	}
}

func handler(collibraClient *http.Client) chip.ToolHandlerFunc[Input, clients.GetLineageDirectionalOutput] {
	return func(ctx context.Context, input Input) (clients.GetLineageDirectionalOutput, error) {
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
