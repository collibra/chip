package get_column_semantics

import (
	"context"
	"net/http"

	"github.com/collibra/chip/pkg/chip"
	"github.com/collibra/chip/pkg/clients"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// AssetWithDescription represents an enriched asset used in traversal tool outputs.
type AssetWithDescription struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	AssetType   string `json:"assetType"`
	Description string `json:"description"`
}

type Input struct {
	ColumnID string `json:"columnId" jsonschema:"Required. The UUID of the column asset to retrieve semantics for."`
}

type Output struct {
	Semantics []DataAttributeSemantics `json:"semantics" jsonschema:"The list of data attributes with their connected measures and business assets."`
	Error     string                   `json:"error,omitempty" jsonschema:"Error message if the operation failed."`
}

type DataAttributeSemantics struct {
	ID                      string                 `json:"id"`
	Name                    string                 `json:"name"`
	AssetType               string                 `json:"assetType"`
	Description             string                 `json:"description"`
	ConnectedMeasures       []AssetWithDescription `json:"connectedMeasures"`
	ConnectedBusinessAssets []AssetWithDescription `json:"connectedBusinessAssets"`
}

func NewTool(collibraClient *http.Client) *chip.Tool[Input, Output] {
	return &chip.Tool[Input, Output]{
		Name:        "get_column_semantics",
		Description: "Retrieve all connected Data Attribute assets for a Column, including descriptions and related Measures and generic business assets with their descriptions.",
		Handler:     handler(collibraClient),
		Permissions: []string{},
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}
}

func handler(collibraClient *http.Client) chip.ToolHandlerFunc[Input, Output] {
	return func(ctx context.Context, input Input) (Output, error) {
		if input.ColumnID == "" {
			return Output{Error: "columnId is required"}, nil
		}

		dataAttributes, err := clients.FindColumnsForDataAttribute(ctx, collibraClient, input.ColumnID)
		if err != nil {
			return Output{}, err
		}

		semantics := make([]DataAttributeSemantics, 0, len(dataAttributes))
		for _, da := range dataAttributes {
			description := clients.FetchDescription(ctx, collibraClient, da.ID)

			rawMeasures, err := clients.FindConnectedAssets(ctx, collibraClient, da.ID, clients.MeasureIsCalculatedUsingDataElementRelID)
			if err != nil {
				return Output{}, err
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

			rawGenericAssets, err := clients.FindConnectedAssets(ctx, collibraClient, da.ID, clients.BusinessAssetRepresentsDataAssetRelID)
			if err != nil {
				return Output{}, err
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

		return Output{Semantics: semantics}, nil
	}
}
