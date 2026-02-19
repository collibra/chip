package tools

import (
	"context"
	"net/http"

	"github.com/collibra/chip/pkg/chip"
	"github.com/collibra/chip/pkg/clients"
)

type BusinessTermDataGetInput struct {
	BusinessTermID string `json:"businessTermId" jsonschema:"Required. The UUID of the Business Term asset to trace back to physical data assets."`
}

type BusinessTermDataGetOutput struct {
	BusinessTermID        string                        `json:"businessTermId" jsonschema:"The Business Term asset ID."`
	ConnectedPhysicalData []BusinessTermDataAttribute `json:"connectedPhysicalData" jsonschema:"The data attributes with their connected columns and tables."`
	Error                 string                        `json:"error,omitempty" jsonschema:"Error message if the operation failed."`
}

type BusinessTermDataAttribute struct {
	ID               string           `json:"id"`
	Name             string           `json:"name"`
	AssetType        string           `json:"assetType"`
	Description      string           `json:"description"`
	ConnectedColumns []ColumnWithTable `json:"connectedColumns"`
}

func NewBusinessTermDataGetTool(collibraClient *http.Client) *chip.Tool[BusinessTermDataGetInput, BusinessTermDataGetOutput] {
	return &chip.Tool[BusinessTermDataGetInput, BusinessTermDataGetOutput]{
		Name:        "business_term_data_get",
		Description: "Retrieve the physical data assets (Columns and Tables) associated with a Business Term via the path Business Term → Data Attribute → Column → Table.",
		Handler:     handleBusinessTermDataGet(collibraClient),
		Permissions: []string{},
	}
}

func handleBusinessTermDataGet(collibraClient *http.Client) chip.ToolHandlerFunc[BusinessTermDataGetInput, BusinessTermDataGetOutput] {
	return func(ctx context.Context, input BusinessTermDataGetInput) (BusinessTermDataGetOutput, error) {
		if input.BusinessTermID == "" {
			return BusinessTermDataGetOutput{Error: "businessTermId is required"}, nil
		}

		dataAttributes, err := clients.FindConnectedAssets(ctx, collibraClient, input.BusinessTermID, clients.GenericConnectedAssetRelID)
		if err != nil {
			return BusinessTermDataGetOutput{}, err
		}

		physicalData := make([]BusinessTermDataAttribute, 0, len(dataAttributes))
		for _, da := range dataAttributes {
			daDescription := clients.FetchDescription(ctx, collibraClient, da.ID)

			columns, err := clients.FindColumnsForDataAttribute(ctx, collibraClient, da.ID)
			if err != nil {
				return BusinessTermDataGetOutput{}, err
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
					return BusinessTermDataGetOutput{}, err
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

			physicalData = append(physicalData, BusinessTermDataAttribute{
				ID:               da.ID,
				Name:             da.Name,
				AssetType:        da.AssetType,
				Description:      daDescription,
				ConnectedColumns: columnsWithDetails,
			})
		}

		return BusinessTermDataGetOutput{
			BusinessTermID:        input.BusinessTermID,
			ConnectedPhysicalData: physicalData,
		}, nil
	}
}
