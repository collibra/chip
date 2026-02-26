package get_business_term

import (
	"context"
	"net/http"

	"github.com/collibra/chip/pkg/chip"
	"github.com/collibra/chip/pkg/clients"
)

// Input defines the parameters for the get_business_term tool.
type Input struct {
	AssetID string `json:"asset_id" jsonschema:"The UUID of the Business Term asset to retrieve"`
}

// ColumnInfo represents a Column asset in the physical data lineage.
type ColumnInfo struct {
	ID           string `json:"id" jsonschema:"The UUID of the Column asset"`
	Name         string `json:"name" jsonschema:"The name of the Column asset"`
	RelationType string `json:"relation_type" jsonschema:"The type of relation connecting to this Column"`
}

// TableInfo represents a Table asset with its connected Columns.
type TableInfo struct {
	ID           string       `json:"id" jsonschema:"The UUID of the Table asset"`
	Name         string       `json:"name" jsonschema:"The name of the Table asset"`
	RelationType string       `json:"relation_type" jsonschema:"The type of relation connecting to this Table"`
	Columns      []ColumnInfo `json:"columns" jsonschema:"Columns connected to this Table"`
}

// DataAttributeInfo represents a Data Attribute with its connected Tables and Columns.
type DataAttributeInfo struct {
	ID           string      `json:"id" jsonschema:"The UUID of the Data Attribute asset"`
	Name         string      `json:"name" jsonschema:"The name of the Data Attribute asset"`
	RelationType string      `json:"relation_type" jsonschema:"The type of relation connecting to this Data Attribute"`
	Tables       []TableInfo `json:"tables" jsonschema:"Tables connected to this Data Attribute"`
}

// Output defines the response from the get_business_term tool.
type Output struct {
	ID          string              `json:"id" jsonschema:"The UUID of the Business Term"`
	Name        string              `json:"name" jsonschema:"The name of the Business Term"`
	DisplayName string              `json:"display_name" jsonschema:"The display name of the Business Term"`
	AssetType   string              `json:"asset_type" jsonschema:"The asset type name"`
	Status      string              `json:"status" jsonschema:"The status of the Business Term"`
	DomainName  string              `json:"domain_name" jsonschema:"The name of the domain the Business Term belongs to"`
	DomainID    string              `json:"domain_id" jsonschema:"The UUID of the domain"`
	Lineage     []DataAttributeInfo `json:"lineage" jsonschema:"Physical data lineage chain: Data Attributes with their connected Tables and Columns"`
}

// NewTool creates a new get_business_term tool instance.
func NewTool(collibraClient *http.Client) *chip.Tool[Input, Output] {
	return &chip.Tool[Input, Output]{
		Name:        "get_business_term",
		Description: "Retrieve a Business Term by ID with its asset type, status, domain, and full physical data lineage chain (Business Term → Data Attribute → Table → Column).",
		Handler:     handler(collibraClient),
		Permissions: []string{"dgc.ai-copilot"},
	}
}

func handler(collibraClient *http.Client) chip.ToolHandlerFunc[Input, Output] {
	return func(ctx context.Context, input Input) (Output, error) {
		// Step 1: Get the Business Term asset details.
		asset, err := clients.GetBusinessTermAsset(ctx, collibraClient, input.AssetID)
		if err != nil {
			return Output{}, err
		}

		output := Output{
			ID:          asset.ID,
			Name:        asset.Name,
			DisplayName: asset.DisplayName,
			AssetType:   asset.Type.Name,
			Status:      asset.Status.Name,
			DomainName:  asset.Domain.Name,
			DomainID:    asset.Domain.ID,
			Lineage:     []DataAttributeInfo{},
		}

		// Step 2: Find relations from the Business Term to discover Data Attributes.
		btRelations, err := clients.GetBusinessTermRelations(ctx, collibraClient, input.AssetID)
		if err != nil {
			return Output{}, err
		}

		for _, rel := range btRelations.Results {
			if rel.Target.Type.Name != "Data Attribute" {
				continue
			}

			da := DataAttributeInfo{
				ID:           rel.Target.ID,
				Name:         rel.Target.Name,
				RelationType: rel.Type.Name,
				Tables:       []TableInfo{},
			}

			// Step 3: Find relations from the Data Attribute to discover Tables.
			daRelations, err := clients.GetBusinessTermRelations(ctx, collibraClient, rel.Target.ID)
			if err != nil {
				return Output{}, err
			}

			for _, daRel := range daRelations.Results {
				if daRel.Target.Type.Name != "Table" {
					continue
				}

				table := TableInfo{
					ID:           daRel.Target.ID,
					Name:         daRel.Target.Name,
					RelationType: daRel.Type.Name,
					Columns:      []ColumnInfo{},
				}

				// Step 4: Find relations from the Table to discover Columns.
				tableRelations, err := clients.GetBusinessTermRelations(ctx, collibraClient, daRel.Target.ID)
				if err != nil {
					return Output{}, err
				}

				for _, tRel := range tableRelations.Results {
					if tRel.Target.Type.Name != "Column" {
						continue
					}

					col := ColumnInfo{
						ID:           tRel.Target.ID,
						Name:         tRel.Target.Name,
						RelationType: tRel.Type.Name,
					}
					table.Columns = append(table.Columns, col)
				}

				da.Tables = append(da.Tables, table)
			}

			output.Lineage = append(output.Lineage, da)
		}

		return output, nil
	}
}
