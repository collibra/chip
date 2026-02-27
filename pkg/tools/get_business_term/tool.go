package get_business_term

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/collibra/chip/pkg/chip"
	"github.com/collibra/chip/pkg/clients"
)

// Input is the input for the get_business_term tool.
type Input struct {
	AssetID string `json:"asset_id" jsonschema:"The UUID of the Business Term asset to retrieve"`
}

// Attribute represents a single asset attribute (e.g. definition, note, example).
type Attribute struct {
	Name  string `json:"name" jsonschema:"The attribute type name (e.g. Definition, Note, Example)"`
	Value string `json:"value" jsonschema:"The attribute value"`
}

// ColumnAsset represents a Column in the physical data lineage.
type ColumnAsset struct {
	ID           string `json:"id" jsonschema:"The UUID of the Column asset"`
	Name         string `json:"name" jsonschema:"The name of the Column asset"`
	RelationType string `json:"relation_type" jsonschema:"The type of relation connecting the Table to this Column"`
}

// TableAsset represents a Table in the physical data lineage with its connected Columns.
type TableAsset struct {
	ID           string        `json:"id" jsonschema:"The UUID of the Table asset"`
	Name         string        `json:"name" jsonschema:"The name of the Table asset"`
	RelationType string        `json:"relation_type" jsonschema:"The type of relation connecting the Data Attribute to this Table"`
	Columns      []ColumnAsset `json:"columns" jsonschema:"Column assets connected to this Table"`
}

// DataAttributeAsset represents a Data Attribute in the physical data lineage with its connected Tables.
type DataAttributeAsset struct {
	ID           string       `json:"id" jsonschema:"The UUID of the Data Attribute asset"`
	Name         string       `json:"name" jsonschema:"The name of the Data Attribute asset"`
	RelationType string       `json:"relation_type" jsonschema:"The type of relation connecting the Business Term to this Data Attribute"`
	Tables       []TableAsset `json:"tables" jsonschema:"Table assets connected to this Data Attribute"`
}

// Output is the output of the get_business_term tool.
type Output struct {
	Name        string               `json:"name" jsonschema:"The name of the Business Term"`
	Description string               `json:"description" jsonschema:"The description of the Business Term, extracted from attributes"`
	Domain      string               `json:"domain" jsonschema:"The domain the Business Term belongs to"`
	Status      string               `json:"status" jsonschema:"The current status of the Business Term"`
	AssetType   string               `json:"asset_type" jsonschema:"The asset type name"`
	Attributes  []Attribute          `json:"attributes" jsonschema:"All attributes of the Business Term (e.g. definition, note, example)"`
	Lineage     []DataAttributeAsset `json:"lineage" jsonschema:"Physical data lineage: Data Attributes, their Tables, and Columns"`
}

// NewTool creates a new get_business_term tool.
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

		// 1. Get asset details
		asset, err := clients.GetBusinessTermAsset(ctx, collibraClient, input.AssetID)
		if err != nil {
			return Output{}, fmt.Errorf("getting asset: %w", err)
		}

		// 2. Get attributes
		attrs, err := clients.GetBusinessTermAttributes(ctx, collibraClient, input.AssetID)
		if err != nil {
			return Output{}, fmt.Errorf("getting attributes: %w", err)
		}

		// Process attributes and extract description
		attributes := make([]Attribute, 0, len(attrs))
		var description string
		for _, attr := range attrs {
			value := attributeValueToString(attr.Value)
			attributes = append(attributes, Attribute{
				Name:  attr.Type.Name,
				Value: value,
			})
			if attr.Type.Name == "Description" {
				description = value
			}
		}

		// 3. Build physical data lineage: BT → Data Attribute → Table → Column
		lineage, err := buildLineage(ctx, collibraClient, input.AssetID)
		if err != nil {
			return Output{}, fmt.Errorf("building lineage: %w", err)
		}

		return Output{
			Name:        asset.Name,
			Description: description,
			Domain:      asset.Domain.Name,
			Status:      asset.Status.Name,
			AssetType:   asset.Type.Name,
			Attributes:  attributes,
			Lineage:     lineage,
		}, nil
	}
}

// buildLineage finds Data Attributes connected to the Business Term and follows
// the chain to Tables and Columns.
func buildLineage(ctx context.Context, client *http.Client, assetID string) ([]DataAttributeAsset, error) {
	rels, err := clients.GetBusinessTermRelations(ctx, client, assetID)
	if err != nil {
		return nil, fmt.Errorf("getting relations for asset %s: %w", assetID, err)
	}

	dataAttributes := make([]DataAttributeAsset, 0)
	for _, rel := range rels.Results {
		if rel.Target.Type.Name != "Data Attribute" {
			continue
		}

		tables, err := buildTables(ctx, client, rel.Target.ID)
		if err != nil {
			return nil, fmt.Errorf("building tables for data attribute %s: %w", rel.Target.ID, err)
		}

		dataAttributes = append(dataAttributes, DataAttributeAsset{
			ID:           rel.Target.ID,
			Name:         rel.Target.Name,
			RelationType: rel.Type.Name,
			Tables:       tables,
		})
	}

	return dataAttributes, nil
}

// buildTables finds Tables connected to a Data Attribute and follows to Columns.
func buildTables(ctx context.Context, client *http.Client, dataAttributeID string) ([]TableAsset, error) {
	rels, err := clients.GetBusinessTermRelations(ctx, client, dataAttributeID)
	if err != nil {
		return nil, fmt.Errorf("getting relations for data attribute %s: %w", dataAttributeID, err)
	}

	tables := make([]TableAsset, 0)
	for _, rel := range rels.Results {
		if rel.Target.Type.Name != "Table" {
			continue
		}

		columns, err := buildColumns(ctx, client, rel.Target.ID)
		if err != nil {
			return nil, fmt.Errorf("building columns for table %s: %w", rel.Target.ID, err)
		}

		tables = append(tables, TableAsset{
			ID:           rel.Target.ID,
			Name:         rel.Target.Name,
			RelationType: rel.Type.Name,
			Columns:      columns,
		})
	}

	return tables, nil
}

// buildColumns finds Columns connected to a Table.
func buildColumns(ctx context.Context, client *http.Client, tableID string) ([]ColumnAsset, error) {
	rels, err := clients.GetBusinessTermRelations(ctx, client, tableID)
	if err != nil {
		return nil, fmt.Errorf("getting relations for table %s: %w", tableID, err)
	}

	columns := make([]ColumnAsset, 0)
	for _, rel := range rels.Results {
		if rel.Target.Type.Name != "Column" {
			continue
		}

		columns = append(columns, ColumnAsset{
			ID:           rel.Target.ID,
			Name:         rel.Target.Name,
			RelationType: rel.Type.Name,
		})
	}

	return columns, nil
}

// attributeValueToString converts an attribute value (which may be string, number,
// object, or array) to a string representation.
func attributeValueToString(v interface{}) string {
	switch val := v.(type) {
	case string:
		return val
	case nil:
		return ""
	default:
		b, err := json.Marshal(val)
		if err != nil {
			return fmt.Sprintf("%v", val)
		}
		return string(b)
	}
}
