package tools

import (
	"context"
	"log/slog"
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

// LineageEntitySummary is a lightweight entity reference included in directional lineage results.
type LineageEntitySummary struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	ParentId string `json:"parentId,omitempty"`
}

// LineageDirectionalOutput wraps the client response and adds resolved entity names.
type LineageDirectionalOutput struct {
	EntityId   string                          `json:"entityId"`
	Direction  string                          `json:"direction"`
	Relations  []clients.LineageRelation        `json:"relations"`
	Entities   map[string]LineageEntitySummary  `json:"entities" jsonschema:"Map of entity ID to name/type for all entities referenced in the relations. Allows identifying entities without additional get_lineage_entity calls."`
	Pagination *clients.LineagePagination       `json:"pagination,omitempty"`
	Warnings   []clients.LineageResponseWarning `json:"warnings,omitempty"`
	Error      string                          `json:"error,omitempty"`
}

func NewGetLineageUpstreamTool(collibraClient *http.Client) *chip.Tool[GetLineageUpstreamInput, LineageDirectionalOutput] {
	return &chip.Tool[GetLineageUpstreamInput, LineageDirectionalOutput]{
		Name: "get_lineage_upstream",
		Description: `WORKFLOW: Call this AFTER search_lineage_entities has given you an entity ID. This is the tool for tracing data sources.
					  Use when the user asks: "where does this data come from?", "what are the sources for this table?", "how is this column calculated?", "what feeds into this report?".
					  Typical workflow: (1) search_lineage_entities to find the entity ID → (2) get_lineage_upstream with that ID → (3) optionally get_lineage_entity for the most relevant source entities only.
					  Returns: a paginated list of relations with an entities map that resolves all entity IDs to names and types. You can identify columns by name without additional calls.
					  Do not call get_lineage_transformation unless the user explicitly asks about the SQL or transformation logic.`,
		Handler:     handleGetLineageUpstream(collibraClient),
		Permissions: []string{},
	}
}

func handleGetLineageUpstream(collibraClient *http.Client) chip.ToolHandlerFunc[GetLineageUpstreamInput, LineageDirectionalOutput] {
	return func(ctx context.Context, input GetLineageUpstreamInput) (LineageDirectionalOutput, error) {
		return handleLineageDirectional(ctx, collibraClient, input.EntityId, input.EntityType, input.Limit, input.Cursor, clients.GetLineageUpstream)
	}
}

// handleLineageDirectional fetches directional lineage and resolves entity names for all referenced IDs.
func handleLineageDirectional(
	ctx context.Context,
	collibraClient *http.Client,
	entityId, entityType string,
	limit int,
	cursor string,
	fetch func(context.Context, *http.Client, string, string, int, string) (*clients.GetLineageDirectionalOutput, error),
) (LineageDirectionalOutput, error) {
	if entityId == "" {
		return LineageDirectionalOutput{Error: "entityId is required"}, nil
	}
	result, err := fetch(ctx, collibraClient, entityId, entityType, limit, cursor)
	if err != nil {
		return LineageDirectionalOutput{}, err
	}

	// Collect all unique entity IDs from relations.
	idSet := make(map[string]struct{})
	for _, rel := range result.Relations {
		idSet[rel.SourceEntityId] = struct{}{}
		idSet[rel.TargetEntityId] = struct{}{}
	}

	// Resolve each entity ID to its name and type.
	entities := make(map[string]LineageEntitySummary, len(idSet))
	for id := range idSet {
		entity, err := clients.GetLineageEntity(ctx, collibraClient, id)
		if err != nil {
			slog.WarnContext(ctx, "failed to resolve lineage entity", "id", id, "error", err)
			continue
		}
		if entity.Entity != nil {
			entities[id] = LineageEntitySummary{
				Name:     entity.Entity.Name,
				Type:     entity.Entity.Type,
				ParentId: entity.Entity.ParentId,
			}
		}
	}

	return LineageDirectionalOutput{
		EntityId:   result.EntityId,
		Direction:  string(result.Direction),
		Relations:  result.Relations,
		Entities:   entities,
		Pagination: result.Pagination,
		Warnings:   result.Warnings,
		Error:      result.Error,
	}, nil
}
