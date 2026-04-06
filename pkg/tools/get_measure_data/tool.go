package get_measure_data

import (
	"context"
	"net/http"

	"github.com/collibra/chip/pkg/chip"
	"github.com/collibra/chip/pkg/clients"
)

type AssetWithDescription struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	AssetType   string `json:"assetType"`
	Description string `json:"description"`
}

// ColumnWithTable represents a column and its parent table in traversal tool outputs.
type ColumnWithTable struct {
	ID             string                `json:"id"`
	Name           string                `json:"name"`
	AssetType      string                `json:"assetType"`
	Description    string                `json:"description"`
	ConnectedTable *AssetWithDescription `json:"connectedTable"`
}

type Input struct {
	MeasureID string `json:"measureId" jsonschema:"Required. The UUID of the measure asset to trace back to its underlying physical columns."`
}

type Output struct {
	DataHierarchy []Attribute `json:"dataHierarchy" jsonschema:"The list of data attributes with their connected columns and tables."`
	Error         string                 `json:"error,omitempty" jsonschema:"Error message if the operation failed."`
}

type Attribute struct {
	ID               string            `json:"id"`
	Name             string            `json:"name"`
	AssetType        string            `json:"assetType"`
	ConnectedColumns []ColumnWithTable `json:"connectedColumns"`
}

func NewTool(collibraClient *http.Client) *chip.Tool[Input, Output] {
	return &chip.Tool[Input, Output]{
		Name:        "get_measure_data",
		Description: "Retrieve all underlying Column assets connected to a Measure via the path Measure → Data Attribute → Column, including each Column's description and parent Table.",
		Handler:     handler(collibraClient),
		Permissions: []string{},
	}
}

func handler(collibraClient *http.Client) chip.ToolHandlerFunc[Input, Output] {
	return func(ctx context.Context, input Input) (Output, error) {
		if input.MeasureID == "" {
			return Output{Error: "measureId is required"}, nil
		}

		dataAttributes, err := clients.FindConnectedAssets(ctx, collibraClient, input.MeasureID, clients.DataAttributeRepresentsMeasureRelID)
		if err != nil {
			return Output{}, err
		}

		hierarchy := make([]Attribute, 0, len(dataAttributes))
		for _, da := range dataAttributes {
			columns, err := clients.FindColumnsForDataAttribute(ctx, collibraClient, da.ID)
			if err != nil {
				return Output{}, err
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
					return Output{}, err
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

			hierarchy = append(hierarchy, Attribute{
				ID:               da.ID,
				Name:             da.Name,
				AssetType:        da.AssetType,
				ConnectedColumns: columnsWithDetails,
			})
		}

		return Output{DataHierarchy: hierarchy}, nil
	}
}
