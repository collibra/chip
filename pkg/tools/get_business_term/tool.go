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
	AssetID string `json:"asset_id" jsonschema:"The UUID of the Business Term asset to retrieve"`
}

// Output defines the structured result of the get_business_term tool.
type Output struct {
	ID         string                 `json:"id" jsonschema:"The UUID of the Business Term"`
	Name       string                 `json:"name" jsonschema:"The name of the Business Term"`
	AssetType  string                 `json:"asset_type" jsonschema:"The asset type name"`
	DomainID   string                 `json:"domain_id" jsonschema:"The UUID of the domain"`
	DomainName string                 `json:"domain_name" jsonschema:"The name of the domain"`
	StatusName string                 `json:"status_name" jsonschema:"The status of the Business Term"`
	Attributes []OutputAttribute      `json:"attributes" jsonschema:"Key-value attribute pairs of the Business Term"`
	Lineage    []DataAttributeLineage `json:"lineage" jsonschema:"Physical data lineage chain: Data Attribute, Table, and Column assets connected to this Business Term"`
}

// OutputAttribute represents a single attribute key-value pair.
type OutputAttribute struct {
	Name  string `json:"name" jsonschema:"The attribute type name"`
	Value string `json:"value" jsonschema:"The attribute value"`
}

// DataAttributeLineage represents a Data Attribute in the lineage chain.
type DataAttributeLineage struct {
	ID           string         `json:"id" jsonschema:"The UUID of the Data Attribute"`
	Name         string         `json:"name" jsonschema:"The name of the Data Attribute"`
	RelationType string         `json:"relation_type" jsonschema:"The relation type connecting the Business Term to this Data Attribute"`
	Tables       []TableLineage `json:"tables" jsonschema:"Tables connected to this Data Attribute"`
}

// TableLineage represents a Table in the lineage chain.
type TableLineage struct {
	ID           string          `json:"id" jsonschema:"The UUID of the Table"`
	Name         string          `json:"name" jsonschema:"The name of the Table"`
	RelationType string          `json:"relation_type" jsonschema:"The relation type connecting the Data Attribute to this Table"`
	Columns      []ColumnLineage `json:"columns" jsonschema:"Columns connected to this Table"`
}

// ColumnLineage represents a Column in the lineage chain.
type ColumnLineage struct {
	ID           string `json:"id" jsonschema:"The UUID of the Column"`
	Name         string `json:"name" jsonschema:"The name of the Column"`
	RelationType string `json:"relation_type" jsonschema:"The relation type connecting the Table to this Column"`
}

// NewTool creates a new get_business_term tool that retrieves a Business Term
// by ID with its attributes and physical data lineage chain.
func NewTool(collibraClient *http.Client) *chip.Tool[Input, Output] {
	return &chip.Tool[Input, Output]{
		Name:        "get_business_term",
		Description: "Retrieve a Business Term by ID with its attributes, asset type, and full physical data lineage chain (Business Term → Data Attribute → Table → Column).",
		Handler:     handler(collibraClient),
		Permissions: []string{"dgc.ai-copilot"},
	}
}

func handler(collibraClient *http.Client) chip.ToolHandlerFunc[Input, Output] {
	return func(ctx context.Context, input Input) (Output, error) {
		if input.AssetID == "" {
			return Output{}, fmt.Errorf("asset_id is required")
		}

		// Step 1: Get the Business Term asset details.
		asset, err := clients.GetBusinessTermAsset(ctx, collibraClient, input.AssetID)
		if err != nil {
			return Output{}, fmt.Errorf("getting asset: %w", err)
		}

		// Step 2: Get the asset's attributes.
		attrs, err := clients.GetBusinessTermAttributes(ctx, collibraClient, input.AssetID)
		if err != nil {
			return Output{}, fmt.Errorf("getting attributes: %w", err)
		}

		outputAttrs := make([]OutputAttribute, 0, len(attrs))
		for _, a := range attrs {
			outputAttrs = append(outputAttrs, OutputAttribute{
				Name:  a.Type.Name,
				Value: a.Value,
			})
		}

		// Step 3: Get relations from the Business Term to find Data Attributes.
		relations, err := clients.GetBusinessTermRelations(ctx, collibraClient, clients.GetBusinessTermRelationsParams{
			SourceID: input.AssetID,
		})
		if err != nil {
			return Output{}, fmt.Errorf("getting relations: %w", err)
		}

		lineage := make([]DataAttributeLineage, 0)
		for _, rel := range relations.Results {
			if rel.Target.Type.Name != "Data Attribute" {
				continue
			}

			da := DataAttributeLineage{
				ID:           rel.Target.ID,
				Name:         rel.Target.Name,
				RelationType: rel.Type.Name,
				Tables:       make([]TableLineage, 0),
			}

			// Step 4: For each Data Attribute, find connected Tables.
			daRelations, err := clients.GetBusinessTermRelations(ctx, collibraClient, clients.GetBusinessTermRelationsParams{
				SourceID: rel.Target.ID,
			})
			if err != nil {
				return Output{}, fmt.Errorf("getting data attribute relations for %s: %w", rel.Target.ID, err)
			}

			for _, daRel := range daRelations.Results {
				if daRel.Target.Type.Name != "Table" {
					continue
				}

				table := TableLineage{
					ID:           daRel.Target.ID,
					Name:         daRel.Target.Name,
					RelationType: daRel.Type.Name,
					Columns:      make([]ColumnLineage, 0),
				}

				// Step 5: For each Table, find connected Columns.
				tableRelations, err := clients.GetBusinessTermRelations(ctx, collibraClient, clients.GetBusinessTermRelationsParams{
					SourceID: daRel.Target.ID,
				})
				if err != nil {
					return Output{}, fmt.Errorf("getting table relations for %s: %w", daRel.Target.ID, err)
				}

				for _, tRel := range tableRelations.Results {
					if tRel.Target.Type.Name != "Column" {
						continue
					}
					table.Columns = append(table.Columns, ColumnLineage{
						ID:           tRel.Target.ID,
						Name:         tRel.Target.Name,
						RelationType: tRel.Type.Name,
					})
				}

				da.Tables = append(da.Tables, table)
			}

			lineage = append(lineage, da)
		}

		return Output{
			ID:         asset.ID,
			Name:       asset.Name,
			AssetType:  asset.Type.Name,
			DomainID:   asset.Domain.ID,
			DomainName: asset.Domain.Name,
			StatusName: asset.Status.Name,
			Attributes: outputAttrs,
			Lineage:    lineage,
		}, nil
	}
}
