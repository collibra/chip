package search_assets

import (
	"context"
	"net/http"

	"github.com/collibra/chip/pkg/chip"
	"github.com/collibra/chip/pkg/clients"
)

type Input struct {
	Name          string   `json:"name,omitempty" jsonschema:"Optional. Asset name to search for."`
	NameMatchMode string   `json:"name_match_mode,omitempty" jsonschema:"Optional. Match mode for name filter: START, END, ANYWHERE, or EXACT."`
	DomainID      string   `json:"domain_id,omitempty" jsonschema:"Optional. Domain ID (UUID) to filter assets by."`
	TypeIDs       []string `json:"type_ids,omitempty" jsonschema:"Optional. List of asset type IDs to filter by."`
	Offset        int32    `json:"offset,omitempty" jsonschema:"Optional. Starting row for pagination (default: 0)."`
	Limit         int32    `json:"limit,omitempty" jsonschema:"Optional. Maximum number of results per page (default: 1000)."`
}

type OutputAsset struct {
	ID          string `json:"id" jsonschema:"The unique identifier of the asset."`
	Name        string `json:"name" jsonschema:"The name of the asset."`
	DisplayName string `json:"display_name" jsonschema:"The display name of the asset."`
	DomainID    string `json:"domain_id" jsonschema:"The domain ID the asset belongs to."`
	TypeID      string `json:"type_id" jsonschema:"The type ID of the asset."`
	Status      string `json:"status" jsonschema:"The status of the asset."`
}

type Output struct {
	Total   int           `json:"total" jsonschema:"Total number of matching assets."`
	Results []OutputAsset `json:"results" jsonschema:"List of matching assets."`
}

func NewTool(collibraClient *http.Client) *chip.Tool[Input, Output] {
	return &chip.Tool[Input, Output]{
		Name:        "search_assets",
		Description: "Search Collibra assets by name, domain, or type with paginated results.",
		Handler:     handler(collibraClient),
		Permissions: []string{"dgc.ai-copilot"},
	}
}

func handler(collibraClient *http.Client) chip.ToolHandlerFunc[Input, Output] {
	return func(ctx context.Context, input Input) (Output, error) {
		resp, err := clients.SearchAssets(ctx, collibraClient, clients.SearchAssetsParams{
			Name:          input.Name,
			NameMatchMode: input.NameMatchMode,
			DomainID:      input.DomainID,
			TypeIDs:       input.TypeIDs,
			Offset:        input.Offset,
			Limit:         input.Limit,
			SortField:     "NAME",
		})
		if err != nil {
			return Output{}, err
		}

		assets := make([]OutputAsset, len(resp.Results))
		for i, r := range resp.Results {
			assets[i] = OutputAsset{
				ID:          r.ID,
				Name:        r.Name,
				DisplayName: r.DisplayName,
				DomainID:    r.DomainID,
				TypeID:      r.TypeID,
				Status:      r.Status.Name,
			}
		}

		return Output{
			Total:   resp.Total,
			Results: assets,
		}, nil
	}
}
