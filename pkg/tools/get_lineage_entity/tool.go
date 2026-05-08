package get_lineage_entity

import (
	"context"
	"net/http"

	"github.com/collibra/chip/pkg/chip"
	"github.com/collibra/chip/pkg/clients"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type Input struct {
	EntityId string `json:"entityId" jsonschema:"Required. The lineage entity ID (obtained from search_lineage_entities, get_lineage_upstream, or get_lineage_downstream). This is NOT a DGC asset UUID."`
}

func NewTool(collibraClient *http.Client) *chip.Tool[Input, clients.GetLineageEntityOutput] {
	return &chip.Tool[Input, clients.GetLineageEntityOutput]{
		Name: "get_lineage_entity",
		Description: `WORKFLOW: This is a FOLLOW-UP tool for resolving entity IDs you already have. Do not call this as a first step — start with search_lineage_entities instead.
					  Use when you have an entity ID from get_lineage_upstream or get_lineage_downstream results and need to know the entity's name, type, or other metadata. Returns: name, type, source systems, parent entity, and linked DGC identifier.
					  IMPORTANT: Upstream/downstream results return entity IDs only. You do NOT need to resolve every ID — summarize based on entity IDs and only call this tool for the most relevant entities the user asked about. Resolving all IDs wastes tool calls.
					  Do not call this if search_lineage_entities already returned the information you need.`,
		Handler:     handler(collibraClient),
		Permissions: []string{},
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}
}

func handler(collibraClient *http.Client) chip.ToolHandlerFunc[Input, clients.GetLineageEntityOutput] {
	return func(ctx context.Context, input Input) (clients.GetLineageEntityOutput, error) {
		if input.EntityId == "" {
			return clients.GetLineageEntityOutput{Found: false, Error: "entityId is required"}, nil
		}

		result, err := clients.GetLineageEntity(ctx, collibraClient, input.EntityId)
		if err != nil {
			return clients.GetLineageEntityOutput{}, err
		}

		return *result, nil
	}
}
