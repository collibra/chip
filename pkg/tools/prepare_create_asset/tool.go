package prepare_create_asset

import (
	"context"
	"net/http"

	"github.com/collibra/chip/pkg/chip"
	"github.com/collibra/chip/pkg/clients"
)

const maxOptions = 20

// Input defines the input parameters for the prepare_create_asset tool.
type Input struct {
	AssetName        string   `json:"assetName" jsonschema:"The name of the asset to create"`
	AssetTypeID      string   `json:"assetTypeId,omitempty" jsonschema:"Optional. The publicId of the asset type"`
	DomainID         string   `json:"domainId,omitempty" jsonschema:"Optional. The ID of the target domain"`
	AttributeTypeIDs []string `json:"attributeTypeIds,omitempty" jsonschema:"Optional. List of attribute type IDs to hydrate schema for"`
}

// AssetTypeOption represents an asset type option returned when the asset type is missing.
type AssetTypeOption struct {
	ID       string `json:"id" jsonschema:"The internal ID of the asset type"`
	PublicID string `json:"publicId" jsonschema:"The public ID of the asset type"`
	Name     string `json:"name" jsonschema:"The name of the asset type"`
}

// ResolvedInfo contains the resolved UUIDs needed by create_asset.
type ResolvedInfo struct {
	AssetTypeID   string `json:"assetTypeId" jsonschema:"The resolved UUID of the asset type — pass this to create_asset"`
	AssetTypeName string `json:"assetTypeName" jsonschema:"The resolved name of the asset type"`
	DomainID      string `json:"domainId" jsonschema:"The resolved UUID of the domain — pass this to create_asset"`
	DomainName    string `json:"domainName" jsonschema:"The resolved name of the domain"`
}

// DomainOption represents a domain option returned when the domain is missing.
type DomainOption struct {
	ID   string `json:"id" jsonschema:"The ID of the domain"`
	Name string `json:"name" jsonschema:"The name of the domain"`
}

// AttributeSchema represents the full schema for an attribute type.
type AttributeSchema struct {
	ID              string                         `json:"id" jsonschema:"The ID of the attribute type"`
	Name            string                         `json:"name" jsonschema:"The name of the attribute type"`
	Kind            string                         `json:"kind" jsonschema:"The data type of the attribute"`
	Required        bool                           `json:"required" jsonschema:"Whether the attribute is mandatory"`
	Constraints     *clients.PrepareCreateConstraints `json:"constraints,omitempty" jsonschema:"Optional. Validation constraints for the attribute"`
	AllowedValues   []string                       `json:"allowedValues,omitempty" jsonschema:"Optional. List of permitted values if restricted"`
	Direction       string                         `json:"direction,omitempty" jsonschema:"Optional. Direction for relation attributes"`
	TargetAssetType *AssetTypeOption               `json:"targetAssetType,omitempty" jsonschema:"Optional. Target asset type for relation attributes"`
}

// DuplicateAsset represents an existing asset found during duplicate checking.
type DuplicateAsset struct {
	ID   string `json:"id" jsonschema:"The ID of the duplicate asset"`
	Name string `json:"name" jsonschema:"The name of the duplicate asset"`
}

// Output defines the output of the prepare_create_asset tool.
type Output struct {
	Status           string            `json:"status" jsonschema:"The preparation status: ready, incomplete, needs_clarification, or duplicate_found"`
	Message          string            `json:"message" jsonschema:"A human-readable message explaining the status"`
	Resolved         *ResolvedInfo     `json:"resolved,omitempty" jsonschema:"Optional. Resolved UUIDs for asset type and domain — present when status is ready. Pass these to create_asset."`
	AssetTypeOptions []AssetTypeOption `json:"assetTypeOptions,omitempty" jsonschema:"Optional. Available asset types when asset type is missing"`
	DomainOptions    []DomainOption    `json:"domainOptions,omitempty" jsonschema:"Optional. Available domains when domain is missing"`
	OptionsTruncated bool             `json:"optionsTruncated" jsonschema:"Whether options were truncated to the maximum limit of 20"`
	AttributeSchema  []AttributeSchema `json:"attributeSchema,omitempty" jsonschema:"Optional. Full attribute schemas for the asset type"`
	Duplicates       []DuplicateAsset  `json:"duplicates,omitempty" jsonschema:"Optional. Existing assets that may be duplicates"`
}

// NewTool creates the prepare_create_asset tool.
func NewTool(collibraClient *http.Client) *chip.Tool[Input, Output] {
	return &chip.Tool[Input, Output]{
		Name:        "prepare_create_asset",
		Description: "Resolve asset type, domain, hydrate full attribute schema, check duplicates — return structured status for asset creation readiness.",
		Handler:     handler(collibraClient),
		Permissions: []string{"dgc.ai-copilot"},
	}
}

func handler(collibraClient *http.Client) chip.ToolHandlerFunc[Input, Output] {
	return func(ctx context.Context, input Input) (Output, error) {
		truncated := false

		// If asset type is missing, return incomplete with options
		if input.AssetTypeID == "" {
			assetTypes, total, err := clients.ListAssetTypesForPrepare(ctx, collibraClient, maxOptions+1)
			if err != nil {
				return Output{}, err
			}
			if total > maxOptions {
				truncated = true
			}
			if len(assetTypes) > maxOptions {
				assetTypes = assetTypes[:maxOptions]
			}
			options := make([]AssetTypeOption, len(assetTypes))
			for i, at := range assetTypes {
				options[i] = AssetTypeOption{ID: at.ID, PublicID: at.PublicID, Name: at.Name}
			}
			return Output{
				Status:           string(clients.StatusIncomplete),
				Message:          "Asset type is required. Please select from the available options.",
				AssetTypeOptions: options,
				OptionsTruncated: truncated,
			}, nil
		}

		// If domain is missing, return incomplete with options
		if input.DomainID == "" {
			domains, total, err := clients.ListDomainsForPrepare(ctx, collibraClient, maxOptions+1)
			if err != nil {
				return Output{}, err
			}
			if total > maxOptions {
				truncated = true
			}
			if len(domains) > maxOptions {
				domains = domains[:maxOptions]
			}
			options := make([]DomainOption, len(domains))
			for i, d := range domains {
				options[i] = DomainOption{ID: d.ID, Name: d.Name}
			}
			return Output{
				Status:           string(clients.StatusIncomplete),
				Message:          "Domain is required. Please select from the available options.",
				DomainOptions:    options,
				OptionsTruncated: truncated,
			}, nil
		}

		// Resolve asset type by publicId
		assetType, err := clients.GetAssetTypeByPublicID(ctx, collibraClient, input.AssetTypeID)
		if err != nil {
			return Output{
				Status:  string(clients.StatusNeedsClarification),
				Message: "Could not resolve asset type: " + err.Error(),
			}, nil
		}

		// Validate domain exists
		domain, err := clients.GetDomainByID(ctx, collibraClient, input.DomainID)
		if err != nil {
			return Output{
				Status:  string(clients.StatusNeedsClarification),
				Message: "Could not resolve domain: " + err.Error(),
			}, nil
		}

		// Validate asset type is allowed in the target domain
		allowedTypes, err := clients.GetAvailableAssetTypesForDomain(ctx, collibraClient, domain.ID)
		if err != nil {
			return Output{}, err
		}

		domainAllowed := false
		for _, at := range allowedTypes {
			if at.ID == assetType.ID {
				domainAllowed = true
				break
			}
		}
		if !domainAllowed {
			return Output{
				Status:  string(clients.StatusNeedsClarification),
				Message: "Asset type \"" + assetType.Name + "\" is not allowed in domain \"" + domain.Name + "\". Please select a valid combination.",
			}, nil
		}

		// Check for duplicates
		duplicates, err := clients.SearchAssetsForDuplicate(ctx, collibraClient, input.AssetName, assetType.ID, domain.ID)
		if err != nil {
			return Output{}, err
		}
		if len(duplicates) > 0 {
			dups := make([]DuplicateAsset, len(duplicates))
			for i, d := range duplicates {
				dups[i] = DuplicateAsset{ID: d.ID, Name: d.Name}
			}
			return Output{
				Status:     string(clients.StatusDuplicateFound),
				Message:    "An asset with the same name already exists in this domain.",
				Duplicates: dups,
			}, nil
		}

		// Hydrate attribute schemas
		var schemas []AttributeSchema
		for _, attrID := range input.AttributeTypeIDs {
			attrType, err := clients.GetAttributeTypeByID(ctx, collibraClient, attrID)
			if err != nil {
				return Output{}, err
			}
			schema := AttributeSchema{
				ID:       attrType.ID,
				Name:     attrType.Name,
				Kind:     attrType.Kind,
				Required: attrType.Required,
			}
			if attrType.Constraints != nil {
				schema.Constraints = attrType.Constraints
			}
			if len(attrType.AllowedValues) > 0 {
				schema.AllowedValues = attrType.AllowedValues
			}
			if attrType.Direction != "" {
				schema.Direction = attrType.Direction
			}
			if attrType.TargetAssetType != nil {
				schema.TargetAssetType = &AssetTypeOption{
					ID:       attrType.TargetAssetType.ID,
					PublicID: attrType.TargetAssetType.PublicID,
					Name:     attrType.TargetAssetType.Name,
				}
			}
			schemas = append(schemas, schema)
		}

		return Output{
			Status:  string(clients.StatusReady),
			Message: "All validations passed. Ready to create asset \"" + input.AssetName + "\" of type \"" + assetType.Name + "\" in domain \"" + domain.Name + "\".",
			Resolved: &ResolvedInfo{
				AssetTypeID:   assetType.ID,
				AssetTypeName: assetType.Name,
				DomainID:      domain.ID,
				DomainName:    domain.Name,
			},
			AttributeSchema: schemas,
		}, nil
	}
}
