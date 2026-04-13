package get_table_semantics

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

type Input struct {
	TableID string `json:"tableId" jsonschema:"Required. The UUID of the Table asset to retrieve semantics for."`
}

type Output struct {
	TableID           string                `json:"tableId" jsonschema:"The Table asset ID."`
	SemanticHierarchy []ColumnWithSemantics `json:"semanticHierarchy" jsonschema:"The semantic hierarchy of columns with their data attributes and measures."`
	Error             string                `json:"error,omitempty" jsonschema:"Error message if the operation failed."`
}

type ColumnWithSemantics struct {
	ID                      string                      `json:"id"`
	Name                    string                      `json:"name"`
	AssetType               string                      `json:"assetType"`
	Description             string                      `json:"description"`
	ConnectedDataAttributes []DataAttributeWithMeasures `json:"connectedDataAttributes"`
}

type DataAttributeWithMeasures struct {
	ID                string                 `json:"id"`
	Name              string                 `json:"name"`
	AssetType         string                 `json:"assetType"`
	Description       string                 `json:"description"`
	ConnectedMeasures []AssetWithDescription `json:"connectedMeasures"`
}

func NewTool(collibraClient *http.Client) *chip.Tool[Input, Output] {
	return &chip.Tool[Input, Output]{
		Name:        "get_table_semantics",
		Description: "Retrieve the semantic layer for a Table asset: Columns, their Data Attributes, and connected Measures. Answers 'What is the semantic context of this table?' or 'Which metrics use data from this table?'.",
		Handler:     handler(collibraClient),
		Permissions: []string{},
	}
}

func handler(collibraClient *http.Client) chip.ToolHandlerFunc[Input, Output] {
	return func(ctx context.Context, input Input) (Output, error) {
		if input.TableID == "" {
			return Output{Error: "tableId is required"}, nil
		}

		rawColumns, err := clients.FindConnectedAssets(ctx, collibraClient, input.TableID, clients.ColumnIsPartOfTableRelID)
		if err != nil {
			return Output{}, err
		}

		columns := make([]ColumnWithSemantics, 0, len(rawColumns))
		for _, col := range rawColumns {
			colDescription := clients.FetchDescription(ctx, collibraClient, col.ID)

			dataAttributes, err := clients.FindColumnsForDataAttribute(ctx, collibraClient, col.ID)
			if err != nil {
				return Output{}, err
			}

			das := make([]DataAttributeWithMeasures, 0, len(dataAttributes))
			for _, da := range dataAttributes {
				daDescription := clients.FetchDescription(ctx, collibraClient, da.ID)

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

				das = append(das, DataAttributeWithMeasures{
					ID:                da.ID,
					Name:              da.Name,
					AssetType:         da.AssetType,
					Description:       daDescription,
					ConnectedMeasures: measures,
				})
			}

			columns = append(columns, ColumnWithSemantics{
				ID:                      col.ID,
				Name:                    col.Name,
				AssetType:               col.AssetType,
				Description:             colDescription,
				ConnectedDataAttributes: das,
			})
		}

		return Output{
			TableID:           input.TableID,
			SemanticHierarchy: columns,
		}, nil
	}
}
