package create_asset

import (
	"context"
	"fmt"
	"net/http"

	"github.com/collibra/chip/pkg/chip"
	"github.com/collibra/chip/pkg/clients"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Input defines the parameters for the create_asset tool.
type Input struct {
	Name        string            `json:"name" jsonschema:"The name of the asset to create"`
	AssetTypeID string            `json:"assetTypeId" jsonschema:"The UUID of the asset type (from prepare_create_asset resolved.assetTypeId)"`
	DomainID    string            `json:"domainId" jsonschema:"The UUID of the domain to create the asset in (from prepare_create_asset resolved.domainId)"`
	DisplayName string            `json:"displayName,omitempty" jsonschema:"Optional. The display name of the asset"`
	Attributes  map[string]string `json:"attributes,omitempty" jsonschema:"Optional. Map of attribute type UUID to attribute value"`
}

// Output defines the result of the create_asset tool.
type Output struct {
	AssetID string `json:"assetId" jsonschema:"The UUID of the newly created asset"`
}

// NewTool creates a new create_asset tool instance.
func NewTool(collibraClient *http.Client) *chip.Tool[Input, Output] {
	return &chip.Tool[Input, Output]{
		Name:        "create_asset",
		Description: "Create a new data asset with optional attributes in Collibra.",
		Handler:     handler(collibraClient),
		Permissions:  []string{},
		Annotations: &mcp.ToolAnnotations{DestructiveHint: boolPtr(false)},
	}
}

func handler(collibraClient *http.Client) chip.ToolHandlerFunc[Input, Output] {
	return func(ctx context.Context, input Input) (Output, error) {
		// Create the asset
		assetReq := clients.CreateAssetRequest{
			Name:        input.Name,
			TypeID:      input.AssetTypeID,
			DomainID:    input.DomainID,
			DisplayName: input.DisplayName,
		}

		assetResp, err := clients.CreateAsset(ctx, collibraClient, assetReq)
		if err != nil {
			return Output{}, err
		}

		// If attributes are provided, create each one
		for attrTypeID, attrValue := range input.Attributes {
			attrReq := clients.CreateAttributeRequest{
				AssetID: assetResp.ID,
				TypeID:  attrTypeID,
				Value:   attrValue,
			}

			_, err := clients.CreateAttribute(ctx, collibraClient, attrReq)
			if err != nil {
				return Output{}, fmt.Errorf("asset created (id=%s) but failed to add attribute (typeId=%s): %w", assetResp.ID, attrTypeID, err)
			}
		}

		return Output{AssetID: assetResp.ID}, nil
	}
}

func boolPtr(b bool) *bool { return &b }
