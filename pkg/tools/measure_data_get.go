package tools

import (
	"context"
	"net/http"

	"github.com/collibra/chip/pkg/chip"
	"github.com/collibra/chip/pkg/clients"
)

// ColumnWithTable represents a column and its parent table in traversal tool outputs.
type ColumnWithTable struct {
	ID             string                `json:"id"`
	Name           string                `json:"name"`
	AssetType      string                `json:"assetType"`
	Description    string                `json:"description"`
	ConnectedTable *AssetWithDescription `json:"connectedTable"`
}

type MeasureDataGetInput struct {
	MeasureID string `json:"measureId" jsonschema:"Required. The UUID of the measure asset to trace back to its underlying physical columns."`
}

type MeasureDataGetOutput struct {
	DataHierarchy []MeasureDataAttribute `json:"dataHierarchy" jsonschema:"The list of data attributes with their connected columns and tables."`
	Error         string                 `json:"error,omitempty" jsonschema:"Error message if the operation failed."`
}

type MeasureDataAttribute struct {
	ID               string           `json:"id"`
	Name             string           `json:"name"`
	AssetType        string           `json:"assetType"`
	ConnectedColumns []ColumnWithTable `json:"connectedColumns"`
}

func NewMeasureDataGetTool(collibraClient *http.Client) *chip.Tool[MeasureDataGetInput, MeasureDataGetOutput] {
	return &chip.Tool[MeasureDataGetInput, MeasureDataGetOutput]{
		Name:        "measure_data_get",
		Description: "Retrieve all underlying Column assets connected to a Measure via the path Measure → Data Attribute → Column, including each Column's description and parent Table.",
		Handler:     handleMeasureDataGet(collibraClient),
		Permissions: []string{},
	}
}

func handleMeasureDataGet(collibraClient *http.Client) chip.ToolHandlerFunc[MeasureDataGetInput, MeasureDataGetOutput] {
	return func(ctx context.Context, input MeasureDataGetInput) (MeasureDataGetOutput, error) {
		if input.MeasureID == "" {
			return MeasureDataGetOutput{Error: "measureId is required"}, nil
		}

		dataAttributes, err := clients.FindConnectedAssets(ctx, collibraClient, input.MeasureID, clients.DataAttributeRepresentsMeasureRelID)
		if err != nil {
			return MeasureDataGetOutput{}, err
		}

		hierarchy := make([]MeasureDataAttribute, 0, len(dataAttributes))
		for _, da := range dataAttributes {
			columns, err := clients.FindColumnsForDataAttribute(ctx, collibraClient, da.ID)
			if err != nil {
				return MeasureDataGetOutput{}, err
			}

			columnsWithDetails := make([]ColumnWithTable, 0, len(columns))
			for _, col := range columns {
				colDetail := ColumnWithTable{
					ID:          col.ID,
					Name:        col.Name,
					AssetType:   col.AssetType,
					Description: clients.FetchDescription(ctx, collibraClient, col.ID),
				}

				tables, err := clients.FindConnectedAssets(ctx, collibraClient, col.ID, clients.ColumnToTableRelID)
				if err != nil {
					return MeasureDataGetOutput{}, err
				}
				if len(tables) > 0 {
					t := tables[0]
					colDetail.ConnectedTable = &AssetWithDescription{
						ID:          t.ID,
						Name:        t.Name,
						AssetType:   t.AssetType,
						Description: clients.FetchDescription(ctx, collibraClient, t.ID),
					}
				}

				columnsWithDetails = append(columnsWithDetails, colDetail)
			}

			hierarchy = append(hierarchy, MeasureDataAttribute{
				ID:               da.ID,
				Name:             da.Name,
				AssetType:        da.AssetType,
				ConnectedColumns: columnsWithDetails,
			})
		}

		return MeasureDataGetOutput{DataHierarchy: hierarchy}, nil
	}
}
