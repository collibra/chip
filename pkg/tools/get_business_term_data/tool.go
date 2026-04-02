package get_business_term_data

import (
	"context"
	"net/http"

	"github.com/collibra/chip/pkg/chip"
	"github.com/collibra/chip/pkg/clients"
)

type ColumnWithTable struct {
	ID             string                `json:"id"`
	Name           string                `json:"name"`
	AssetType      string                `json:"assetType"`
	Description    string                `json:"description"`
	ConnectedTable *AssetWithDescription `json:"connectedTable"`
}

type AssetWithDescription struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	AssetType   string `json:"assetType"`
	Description string `json:"description"`
}

type Input struct {
	BusinessTermID string `json:"businessTermId" jsonschema:"Required. The UUID of the Business Term asset to trace back to physical data assets."`
}

type Output struct {
	BusinessTermID        string                      `json:"businessTermId" jsonschema:"The Business Term asset ID."`
	ConnectedPhysicalData []Attribute `json:"connectedPhysicalData" jsonschema:"The data attributes with their connected columns and tables."`
	Error                 string                      `json:"error,omitempty" jsonschema:"Error message if the operation failed."`
}

type Attribute struct {
	ID               string            `json:"id"`
	Name             string            `json:"name"`
	AssetType        string            `json:"assetType"`
	Description      string            `json:"description"`
	ConnectedColumns []ColumnWithTable `json:"connectedColumns"`
}

func NewTool(collibraClient *http.Client) *chip.Tool[Input, Output] {
	return &chip.Tool[Input, Output]{
		Name:        "get_business_term_data",
		Description: "Retrieve the physical data assets (Columns and Tables) associated with a Business Term via the path Business Term → Data Attribute → Column → Table.",
		Handler:     handler(collibraClient),
		Permissions: []string{},
	}
}

func handler(collibraClient *http.Client) chip.ToolHandlerFunc[Input, Output] {
	return func(ctx context.Context, input Input) (Output, error) {
		if input.BusinessTermID == "" {
			return Output{Error: "businessTermId is required"}, nil
		}

		dataAttributes, err := clients.FindConnectedAssets(ctx, collibraClient, input.BusinessTermID, clients.GenericConnectedAssetRelID)
		if err != nil {
			return Output{}, err
		}

		physicalData := make([]Attribute, 0, len(dataAttributes))
		for _, da := range dataAttributes {
			daDescription := clients.FetchDescription(ctx, collibraClient, da.ID)

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

			physicalData = append(physicalData, Attribute{
				ID:               da.ID,
				Name:             da.Name,
				AssetType:        da.AssetType,
				Description:      daDescription,
				ConnectedColumns: columnsWithDetails,
			})
		}

		return Output{
			BusinessTermID:        input.BusinessTermID,
			ConnectedPhysicalData: physicalData,
		}, nil
	}
}
