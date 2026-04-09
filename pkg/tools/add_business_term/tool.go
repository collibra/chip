package add_business_term

import (
	"context"
	"net/http"

	"github.com/collibra/chip/pkg/chip"
	"github.com/collibra/chip/pkg/clients"
)

const (
	// BusinessTermTypeID is the fixed type public ID for Business Term assets.
	BusinessTermTypeID = "BusinessTerm"
	// DefinitionAttributeTypeID is the type ID for the Definition attribute.
	DefinitionAttributeTypeID = "00000000-0000-0000-0000-000000000202"
)

// InputAttribute represents an additional attribute to add to the business term.
type InputAttribute struct {
	TypeId string `json:"typeId" jsonschema:"UUID of the attribute type"`
	Value  string `json:"value" jsonschema:"Value for the attribute"`
}

// Input is the input for the add_business_term tool.
type Input struct {
	Name       string           `json:"name" jsonschema:"Name of the business term to create"`
	DomainId   string           `json:"domainId" jsonschema:"UUID of the domain to create the business term in"`
	Definition string           `json:"definition,omitempty" jsonschema:"Optional. Definition text for the business term"`
	Attributes []InputAttribute `json:"attributes,omitempty" jsonschema:"Optional. Additional attributes to add to the business term, each with a type_id and value"`
}

// Output is the output of the add_business_term tool.
type Output struct {
	AssetId string `json:"assetId" jsonschema:"UUID of the created business term asset"`
}

// NewTool creates a new add_business_term tool.
func NewTool(collibraClient *http.Client) *chip.Tool[Input, Output] {
	return &chip.Tool[Input, Output]{
		Name:        "add_business_term",
		Description: "Create a business term asset with definition and optional attributes in Collibra.",
		Handler:     handler(collibraClient),
		Permissions: []string{},
	}
}

func handler(collibraClient *http.Client) chip.ToolHandlerFunc[Input, Output] {
	return func(ctx context.Context, input Input) (Output, error) {
		// Step 1: Create the business term asset
		assetResp, err := clients.CreateBusinessTermAsset(ctx, collibraClient, clients.AddBusinessTermAssetRequest{
			Name:         input.Name,
			TypePublicId: BusinessTermTypeID,
			DomainId:     input.DomainId,
		})
		if err != nil {
			return Output{}, err
		}

		assetId := assetResp.Id

		// Step 2: Add definition attribute if provided
		if input.Definition != "" {
			_, err := clients.CreateBusinessTermAttribute(ctx, collibraClient, clients.AddBusinessTermAttributeRequest{
				AssetId: assetId,
				TypeId:  DefinitionAttributeTypeID,
				Value:   input.Definition,
			})
			if err != nil {
				return Output{}, err
			}
		}

		// Step 3: Add additional attributes if provided
		for _, attr := range input.Attributes {
			_, err := clients.CreateBusinessTermAttribute(ctx, collibraClient, clients.AddBusinessTermAttributeRequest{
				AssetId: assetId,
				TypeId:  attr.TypeId,
				Value:   attr.Value,
			})
			if err != nil {
				return Output{}, err
			}
		}

		return Output{AssetId: assetId}, nil
	}
}
