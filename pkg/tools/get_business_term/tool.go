package get_business_term

import (
	"context"
	"fmt"
	"net/http"

	"github.com/collibra/chip/pkg/chip"
	"github.com/collibra/chip/pkg/clients"
)

// Input defines the parameters for the get_business_term tool.
type Input struct {
	AssetID string `json:"asset_id" jsonschema:"The ID (UUID) of the Business Term asset to retrieve"`
}

// ColumnLineage represents a Column asset in the physical data lineage.
type ColumnLineage struct {
	ID           string `json:"id" jsonschema:"The unique ID of the Column asset"`
	Name         string `json:"name" jsonschema:"The name of the Column asset"`
	RelationType string `json:"relation_type" jsonschema:"The type of relation connecting to this Column"`
}

// TableLineage represents a Table asset in the physical data lineage.
type TableLineage struct {
	ID           string          `json:"id" jsonschema:"The unique ID of the Table asset"`
	Name         string          `json:"name" jsonschema:"The name of the Table asset"`
	RelationType string          `json:"relation_type" jsonschema:"The type of relation connecting to this Table"`
	Columns      []ColumnLineage `json:"columns" jsonschema:"Columns connected to this Table"`
}

// DataAttributeLineage represents a Data Attribute asset in the physical data lineage.
type DataAttributeLineage struct {
	ID           string         `json:"id" jsonschema:"The unique ID of the Data Attribute asset"`
	Name         string         `json:"name" jsonschema:"The name of the Data Attribute asset"`
	RelationType string         `json:"relation_type" jsonschema:"The type of relation connecting to this Data Attribute"`
	Tables       []TableLineage `json:"tables" jsonschema:"Tables connected to this Data Attribute"`
}

// Output defines the response for the get_business_term tool.
type Output struct {
	ID             string                 `json:"id" jsonschema:"The unique ID of the Business Term"`
	Name           string                 `json:"name" jsonschema:"The name of the Business Term"`
	DisplayName    string                 `json:"display_name,omitempty" jsonschema:"Optional. The display name of the Business Term"`
	AssetType      string                 `json:"asset_type" jsonschema:"The asset type name"`
	Status         string                 `json:"status" jsonschema:"The status of the Business Term"`
	Domain         string                 `json:"domain" jsonschema:"The domain the Business Term belongs to"`
	DataAttributes []DataAttributeLineage `json:"data_attributes" jsonschema:"Connected Data Attributes with their Tables and Columns lineage"`
}

// NewTool creates a new get_business_term tool that retrieves a Business Term by ID
// with its full lineage to physical data: Business Term → Data Attribute → Table → Column.
func NewTool(collibraClient *http.Client) *chip.Tool[Input, Output] {
	return &chip.Tool[Input, Output]{
		Name:        "get_business_term",
		Description: "Retrieve a Business Term by ID with its attributes, asset type, and the full lineage to physical data: Business Term → Data Attribute → Table → Column.",
		Handler:     handler(collibraClient),
		Permissions: []string{"dgc.ai-copilot"},
	}
}

func handler(collibraClient *http.Client) chip.ToolHandlerFunc[Input, Output] {
	return func(ctx context.Context, input Input) (Output, error) {
		if input.AssetID == "" {
			return Output{}, fmt.Errorf("asset_id is required")
		}

		// Step 1: Fetch the Business Term asset details.
		asset, err := clients.GetBusinessTermAsset(ctx, collibraClient, input.AssetID)
		if err != nil {
			return Output{}, err
		}

		output := Output{
			ID:             asset.ID,
			Name:           asset.Name,
			DisplayName:    asset.DisplayName,
			AssetType:      asset.Type.Name,
			Status:         asset.Status.Name,
			Domain:         asset.Domain.Name,
			DataAttributes: []DataAttributeLineage{},
		}

		// Step 2: Fetch relations from the Business Term.
		btRelations, err := clients.GetBusinessTermRelations(ctx, collibraClient, input.AssetID)
		if err != nil {
			return Output{}, err
		}

		// Step 3: Filter for Data Attribute targets and follow lineage.
		for _, rel := range btRelations.Results {
			if rel.Target.Type.Name != "Data Attribute" {
				continue
			}

			da := DataAttributeLineage{
				ID:           rel.Target.ID,
				Name:         rel.Target.Name,
				RelationType: rel.Type.Name,
				Tables:       []TableLineage{},
			}

			// Step 4: For each Data Attribute, find connected Tables.
			daRelations, err := clients.GetBusinessTermRelations(ctx, collibraClient, rel.Target.ID)
			if err != nil {
				return Output{}, err
			}

			for _, daRel := range daRelations.Results {
				if daRel.Target.Type.Name != "Table" {
					continue
				}

				table := TableLineage{
					ID:           daRel.Target.ID,
					Name:         daRel.Target.Name,
					RelationType: daRel.Type.Name,
					Columns:      []ColumnLineage{},
				}

				// Step 5: For each Table, find connected Columns.
				tableRelations, err := clients.GetBusinessTermRelations(ctx, collibraClient, daRel.Target.ID)
				if err != nil {
					return Output{}, err
				}

				for _, tableRel := range tableRelations.Results {
					if tableRel.Target.Type.Name != "Column" {
						continue
					}

					table.Columns = append(table.Columns, ColumnLineage{
						ID:           tableRel.Target.ID,
						Name:         tableRel.Target.Name,
						RelationType: tableRel.Type.Name,
					})
				}

				da.Tables = append(da.Tables, table)
			}

			output.DataAttributes = append(output.DataAttributes, da)
		}

		return output, nil
	}
}
