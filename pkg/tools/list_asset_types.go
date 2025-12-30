package tools

import (
	"context"
	"net/http"

	"github.com/collibra/chip/pkg/chip"
	"github.com/collibra/chip/pkg/clients"
)

type ListAssetTypesInput struct {
	Limit  int `json:"limit,omitempty" jsonschema:"Optional. Maximum number of results to return. The maximum allowed limit is 1000. Default: 100."`
	Offset int `json:"offset,omitempty" jsonschema:"Optional. Index of first result (pagination offset). Default: 0."`
}

type ListAssetTypesOutput struct {
	Total      int64       `json:"total" jsonschema:"The total number of asset types available matching the search criteria"`
	Offset     int64       `json:"offset" jsonschema:"The offset for the results"`
	Limit      int64       `json:"limit" jsonschema:"The maximum number of results returned"`
	AssetTypes []AssetType `json:"assetTypes" jsonschema:"The list of asset types"`
}

type AssetType struct {
	ID                 string `json:"id" jsonschema:"The unique identifier of the asset type"`
	Name               string `json:"name" jsonschema:"The name of the asset type"`
	Description        string `json:"description,omitempty" jsonschema:"The description of the asset type"`
	PublicId           string `json:"publicId,omitempty" jsonschema:"The public id of the asset type"`
	DisplayNameEnabled bool   `json:"displayNameEnabled" jsonschema:"Whether display name is enabled for assets of this type"`
	RatingEnabled      bool   `json:"ratingEnabled" jsonschema:"Whether rating is enabled for assets of this type"`
	FinalType          bool   `json:"finalType" jsonschema:"Whether the ability to create child asset types is locked"`
	System             bool   `json:"system" jsonschema:"Whether this is a system asset type"`
	Product            string `json:"product,omitempty" jsonschema:"The product to which this asset type is linked"`
}

func NewListAssetTypesTool(collibraClient *http.Client) *chip.Tool[ListAssetTypesInput, ListAssetTypesOutput] {
	return &chip.Tool[ListAssetTypesInput, ListAssetTypesOutput]{
		Name:        "asset_types_list",
		Description: "List asset types available in Collibra with their properties and metadata.",
		Handler:     handleListAssetTypes(collibraClient),
	}
}

func handleListAssetTypes(collibraClient *http.Client) chip.ToolHandlerFunc[ListAssetTypesInput, ListAssetTypesOutput] {
	return func(ctx context.Context, input ListAssetTypesInput) (ListAssetTypesOutput, error) {
		if input.Limit == 0 {
			input.Limit = 100
		}

		response, err := clients.ListAssetTypes(ctx, collibraClient, input.Limit, input.Offset)
		if err != nil {
			return ListAssetTypesOutput{}, err
		}

		assetTypes := make([]AssetType, len(response.Results))
		for i, at := range response.Results {
			assetTypes[i] = AssetType{
				ID:                 at.ID,
				Name:               at.Name,
				Description:        at.Description,
				PublicId:           at.PublicId,
				DisplayNameEnabled: at.DisplayNameEnabled,
				RatingEnabled:      at.RatingEnabled,
				FinalType:          at.FinalType,
				System:             at.System,
				Product:            at.Product,
			}
		}

		return ListAssetTypesOutput{
			Total:      response.Total,
			Offset:     response.Offset,
			Limit:      response.Limit,
			AssetTypes: assetTypes,
		}, nil
	}
}
