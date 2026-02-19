package tools

import (
	"context"
	"net/http"

	"github.com/collibra/chip/pkg/chip"
	"github.com/collibra/chip/pkg/clients"
)

// AssetWithDescription represents an enriched asset used in traversal tool outputs.
type AssetWithDescription struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	AssetType   string `json:"assetType"`
	Description string `json:"description"`
}

type ColumnSemanticsGetInput struct {
	ColumnID string `json:"columnId" jsonschema:"Required. The UUID of the column asset to retrieve semantics for."`
}

type ColumnSemanticsGetOutput struct {
	Semantics []DataAttributeSemantics `json:"semantics" jsonschema:"The list of data attributes with their connected measures and business assets."`
	Error     string                   `json:"error,omitempty" jsonschema:"Error message if the operation failed."`
}

type DataAttributeSemantics struct {
	ID                      string                `json:"id"`
	Name                    string                `json:"name"`
	AssetType               string                `json:"assetType"`
	Description             string                `json:"description"`
	ConnectedMeasures       []AssetWithDescription `json:"connectedMeasures"`
	ConnectedBusinessAssets []AssetWithDescription `json:"connectedBusinessAssets"`
}

func NewColumnSemanticsGetTool(collibraClient *http.Client) *chip.Tool[ColumnSemanticsGetInput, ColumnSemanticsGetOutput] {
	return &chip.Tool[ColumnSemanticsGetInput, ColumnSemanticsGetOutput]{
		Name:        "column_semantics_get",
		Description: "Retrieve all connected Data Attribute assets for a Column, including descriptions and related Measures and generic business assets with their descriptions.",
		Handler:     handleColumnSemanticsGet(collibraClient),
		Permissions: []string{},
	}
}

func handleColumnSemanticsGet(collibraClient *http.Client) chip.ToolHandlerFunc[ColumnSemanticsGetInput, ColumnSemanticsGetOutput] {
	return func(ctx context.Context, input ColumnSemanticsGetInput) (ColumnSemanticsGetOutput, error) {
		if input.ColumnID == "" {
			return ColumnSemanticsGetOutput{Error: "columnId is required"}, nil
		}

		dataAttributes, err := clients.FindColumnsForDataAttribute(ctx, collibraClient, input.ColumnID)
		if err != nil {
			return ColumnSemanticsGetOutput{}, err
		}

		semantics := make([]DataAttributeSemantics, 0, len(dataAttributes))
		for _, da := range dataAttributes {
			description := clients.FetchDescription(ctx, collibraClient, da.ID)

			rawMeasures, err := clients.FindConnectedAssets(ctx, collibraClient, da.ID, clients.DataAttributeRepresentsMeasureRelID)
			if err != nil {
				return ColumnSemanticsGetOutput{}, err
			}

			measures := make([]AssetWithDescription, 0, len(rawMeasures))
			for _, m := range rawMeasures {
				measures = append(measures, AssetWithDescription{
					ID:          m.ID,
					Name:        m.Name,
					AssetType:   m.AssetType,
					Description: clients.FetchDescription(ctx, collibraClient, m.ID),
				})
			}

			rawGenericAssets, err := clients.FindConnectedAssets(ctx, collibraClient, da.ID, clients.GenericConnectedAssetRelID)
			if err != nil {
				return ColumnSemanticsGetOutput{}, err
			}

			genericAssets := make([]AssetWithDescription, 0, len(rawGenericAssets))
			for _, g := range rawGenericAssets {
				genericAssets = append(genericAssets, AssetWithDescription{
					ID:          g.ID,
					Name:        g.Name,
					AssetType:   g.AssetType,
					Description: clients.FetchDescription(ctx, collibraClient, g.ID),
				})
			}

			semantics = append(semantics, DataAttributeSemantics{
				ID:                      da.ID,
				Name:                    da.Name,
				AssetType:               da.AssetType,
				Description:             description,
				ConnectedMeasures:       measures,
				ConnectedBusinessAssets: genericAssets,
			})
		}

		return ColumnSemanticsGetOutput{Semantics: semantics}, nil
	}
}
