package list_context_specifications

import (
	"context"
	"net/http"

	"github.com/collibra/chip/pkg/chip"
	"github.com/collibra/chip/pkg/clients"
	"github.com/collibra/chip/pkg/tools/validation"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type Input struct {
	AssetId           string `json:"assetId,omitempty" jsonschema:"Optional. UUID of the asset to discover applicable Context Specifications for. Only specs whose source asset type matches the asset type of this asset will be returned."`
	AssetTypePublicId string `json:"assetTypePublicId,omitempty" jsonschema:"Optional. Filter by asset type public ID (e.g. 'Table'). Only Context Specifications with a matching asset type will be returned."`
	Offset            int    `json:"offset,omitempty" jsonschema:"Optional. Index of the first result to retrieve. Default: 0."`
	Limit             int    `json:"limit,omitempty" jsonschema:"Optional. Maximum number of results to retrieve. Default: 50. Maximum: 1000."`
}

type AssetType struct {
	PublicId string `json:"publicId" jsonschema:"The public ID of the asset type (e.g. 'Table')"`
	Name     string `json:"name,omitempty" jsonschema:"The display name of the asset type"`
}

type ContextSpecSummary struct {
	ID          string    `json:"id" jsonschema:"The UUID of the Context Specification"`
	Name        string    `json:"name" jsonschema:"The display name of the Context Specification"`
	Description string    `json:"description,omitempty" jsonschema:"Optional description of the Context Specification"`
	AssetType   AssetType `json:"assetType" jsonschema:"The asset type this Context Specification applies to"`
}

type Output struct {
	Total   int                  `json:"total" jsonschema:"Total number of Context Specifications matching the query"`
	Results []ContextSpecSummary `json:"results" jsonschema:"The list of matching Context Specifications"`
}

func NewTool(collibraClient *http.Client) *chip.Tool[Input, Output] {
	return &chip.Tool[Input, Output]{
		Name:        "list_context_specifications",
		Title:       "List Context Specifications",
		Description: "Primary discovery tool for Context Specifications. Returns all Context Specifications applicable to a given asset or asset type. Use assetId to find specs that match the type of a specific asset, or assetTypePublicId to filter by a known asset type. Call this first to discover which Context Specifications are available before calling get_context.",
		Handler:     handler(collibraClient),
		Permissions: []string{},
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}
}

func handler(collibraClient *http.Client) chip.ToolHandlerFunc[Input, Output] {
	return func(ctx context.Context, input Input) (Output, error) {
		if err := validation.UUIDOptional("assetId", input.AssetId); err != nil {
			return Output{}, err
		}

		if input.Limit == 0 {
			input.Limit = 50
		}

		response, err := clients.ListContextSpecifications(ctx, collibraClient, input.AssetId, input.AssetTypePublicId, input.Offset, input.Limit)
		if err != nil {
			return Output{}, err
		}

		results := make([]ContextSpecSummary, len(response.Results))
		for i, spec := range response.Results {
			results[i] = ContextSpecSummary{
				ID:          spec.ID,
				Name:        spec.Name,
				Description: spec.Description,
				AssetType: AssetType{
					PublicId: spec.AssetType.PublicId,
					Name:     spec.AssetType.Name,
				},
			}
		}

		return Output{
			Total:   response.Total,
			Results: results,
		}, nil
	}
}
