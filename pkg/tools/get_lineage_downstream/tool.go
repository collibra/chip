package get_lineage_downstream

import (
	"context"
	"net/http"

	"github.com/collibra/chip/pkg/chip"
	"github.com/collibra/chip/pkg/clients"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type Input struct {
	EntityId   string `json:"entityId" jsonschema:"Required. ID of the entity to trace downstream from. Can be numeric string or DGC UUID."`
	EntityType string `json:"entityType,omitempty" jsonschema:"Optional. Filter to only include entities of this type (e.g. 'table', 'report'). Useful when you only care about specific downstream asset types."`
	Limit      int    `json:"limit,omitempty" jsonschema:"Optional. Max relations per page. Default: 20, Min: 1, Max: 100."`
	Cursor     string `json:"cursor,omitempty" jsonschema:"Optional. Pagination cursor from a previous response. Do not construct manually."`
}

func NewTool(collibraClient *http.Client) *chip.Tool[Input, clients.GetLineageDirectionalOutput] {
	return &chip.Tool[Input, clients.GetLineageDirectionalOutput]{
		Name:        "get_lineage_downstream",
		Description: "Get the downstream technical lineage graph for a data entity -- all direct and indirect consumer entities that are impacted by it, along with the transformations connecting them. This traces through all data objects across external systems (including unregistered assets, temporary tables, and source code), not just assets in the Collibra Data Catalog. Use this to answer \"What depends on this data?\" or \"If this table changes, what else is affected?\" Essential for impact analysis before modifying or deprecating a data asset. Results are paginated.",
		Handler:     handler(collibraClient),
		Permissions: []string{},
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}
}

func handler(collibraClient *http.Client) chip.ToolHandlerFunc[Input, clients.GetLineageDirectionalOutput] {
	return func(ctx context.Context, input Input) (clients.GetLineageDirectionalOutput, error) {
		return handleLineageDirectional(ctx, collibraClient, input.EntityId, input.EntityType, input.Limit, input.Cursor, clients.GetLineageDownstream)
	}
}

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
