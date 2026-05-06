// Package list_attribute_types exposes the cached DGC attribute-type
// catalog to MCP clients. The full catalog is fetched once per
// chip-binary lifetime (see clients/catalog_cache.go) and sliced
// server-side per request.
package list_attribute_types

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
	Total          int64           `json:"total" jsonschema:"The total number of attribute types in the catalog"`
	Offset         int64           `json:"offset" jsonschema:"The offset for the results"`
	Limit          int64           `json:"limit" jsonschema:"The maximum number of results returned"`
	AttributeTypes []AttributeType `json:"attributeTypes" jsonschema:"The list of attribute types"`
}

type AttributeType struct {
	ID            string   `json:"id" jsonschema:"The unique identifier of the attribute type"`
	Name          string   `json:"name" jsonschema:"The name of the attribute type"`
	Description   string   `json:"description,omitempty" jsonschema:"The description of the attribute type"`
	PublicID      string   `json:"publicId,omitempty" jsonschema:"The public id of the attribute type"`
	Kind          string   `json:"kind,omitempty" jsonschema:"The attributeTypeDiscriminator: e.g. StringAttributeType, BooleanAttributeType, SingleValueListAttributeType"`
	AllowedValues []string `json:"allowedValues,omitempty" jsonschema:"Populated for SingleValueList / MultiValueList kinds; empty otherwise"`
	System        bool     `json:"system" jsonschema:"Whether this is a system attribute type"`
}

func NewTool(collibraClient *http.Client) *chip.Tool[Input, Output] {
	return &chip.Tool[Input, Output]{
		Name:        "list_attribute_types",
		Description: "List attribute types available in Collibra. The full catalog is cached server-side for one hour, so repeated calls do not re-hit DGC.",
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
		all, err := clients.ListAllAttributeTypes(ctx, collibraClient)
		if err != nil {
			return Output{}, err
		}
		page := paginate(all, input.Offset, input.Limit)
		out := Output{
			Total:          int64(len(all)),
			Offset:         int64(input.Offset),
			Limit:          int64(input.Limit),
			AttributeTypes: make([]AttributeType, len(page)),
		}
		for i, at := range page {
			out.AttributeTypes[i] = AttributeType{
				ID:            at.ID,
				Name:          at.Name,
				Description:   at.Description,
				PublicID:      at.PublicID,
				Kind:          at.AttributeTypeDiscriminator,
				AllowedValues: at.AllowedValues,
				System:        at.System,
			}
		}
		return out, nil
	}
}

func paginate(all []clients.AttributeTypeDetails, offset, limit int) []clients.AttributeTypeDetails {
	if offset >= len(all) {
		return nil
	}
	end := offset + limit
	if end > len(all) {
		end = len(all)
	}
	return all[offset:end]
}