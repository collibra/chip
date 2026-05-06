// Package list_relation_types exposes the cached DGC relation-type catalog
// to MCP clients. The full catalog is fetched once per chip-binary
// lifetime (see clients/catalog_cache.go) and sliced server-side per
// request, so paginated browsing doesn't re-hit the target environment.
package list_relation_types

import (
	"context"
	"net/http"

	"github.com/collibra/chip/pkg/chip"
	"github.com/collibra/chip/pkg/clients"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type Input struct {
	Limit  int `json:"limit,omitempty" jsonschema:"Optional. Maximum number of results to return. The maximum allowed limit is 1000. Default: 100."`
	Offset int `json:"offset,omitempty" jsonschema:"Optional. Index of first result (pagination offset). Default: 0."`
}

type Output struct {
	Total         int64          `json:"total" jsonschema:"The total number of relation types in the catalog"`
	Offset        int64          `json:"offset" jsonschema:"The offset for the results"`
	Limit         int64          `json:"limit" jsonschema:"The maximum number of results returned"`
	RelationTypes []RelationType `json:"relationTypes" jsonschema:"The list of relation types"`
}

type RelationType struct {
	ID          string         `json:"id" jsonschema:"The unique identifier of the relation type"`
	PublicID    string         `json:"publicId,omitempty" jsonschema:"The public id of the relation type"`
	Description string         `json:"description,omitempty" jsonschema:"The description of the relation type"`
	Role        string         `json:"role,omitempty" jsonschema:"The name of the role that the source plays in the relation"`
	CoRole      string         `json:"coRole,omitempty" jsonschema:"The name of the role that the target plays in the relation"`
	System      bool           `json:"system" jsonschema:"Whether this is a system relation type"`
	SourceType  *TypeReference `json:"sourceType,omitempty" jsonschema:"Source asset type reference"`
	TargetType  *TypeReference `json:"targetType,omitempty" jsonschema:"Target asset type reference"`
}

type TypeReference struct {
	ID       string `json:"id" jsonschema:"Asset type UUID"`
	Name     string `json:"name,omitempty" jsonschema:"Asset type display name"`
	PublicID string `json:"publicId,omitempty" jsonschema:"Asset type publicId"`
}

func NewTool(collibraClient *http.Client) *chip.Tool[Input, Output] {
	return &chip.Tool[Input, Output]{
		Name:        "list_relation_types",
		Description: "List relation types available in Collibra. The full catalog is cached server-side for one hour, so repeated calls do not re-hit DGC.",
		Handler:     handler(collibraClient),
		Permissions: []string{},
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}
}

func handler(collibraClient *http.Client) chip.ToolHandlerFunc[Input, Output] {
	return func(ctx context.Context, input Input) (Output, error) {
		if input.Limit <= 0 {
			input.Limit = 100
		}
		all, err := clients.ListAllRelationTypes(ctx, collibraClient)
		if err != nil {
			return Output{}, err
		}
		page := paginate(all, input.Offset, input.Limit)
		out := Output{
			Total:         int64(len(all)),
			Offset:        int64(input.Offset),
			Limit:         int64(input.Limit),
			RelationTypes: make([]RelationType, len(page)),
		}
		for i, rt := range page {
			out.RelationTypes[i] = projectRelationType(rt)
		}
		return out, nil
	}
}

func paginate(all []clients.RelationTypeDetails, offset, limit int) []clients.RelationTypeDetails {
	if offset >= len(all) {
		return nil
	}
	end := offset + limit
	if end > len(all) {
		end = len(all)
	}
	return all[offset:end]
}

func projectRelationType(rt clients.RelationTypeDetails) RelationType {
	out := RelationType{
		ID:          rt.ID,
		PublicID:    rt.PublicID,
		Description: rt.Description,
		Role:        rt.Role,
		CoRole:      rt.CoRole,
		System:      rt.System,
	}
	if rt.SourceType != nil {
		out.SourceType = &TypeReference{ID: rt.SourceType.ID, Name: rt.SourceType.Name, PublicID: rt.SourceType.PublicID}
	}
	if rt.TargetType != nil {
		out.TargetType = &TypeReference{ID: rt.TargetType.ID, Name: rt.TargetType.Name, PublicID: rt.TargetType.PublicID}
	}
	return out
}
