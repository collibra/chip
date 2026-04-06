package prepare_add_business_term

import (
	"context"
	"net/http"
	"strings"

	"github.com/collibra/chip/pkg/chip"
	"github.com/collibra/chip/pkg/clients"
)

const businessTermPublicID = "BusinessTerm"

// Input represents the input parameters for the prepare_add_business_term tool.
type Input struct {
	Name        string `json:"name" jsonschema:"The name of the business term to add"`
	DomainName  string `json:"domain_name,omitempty" jsonschema:"Optional. The domain name to resolve for the business term"`
	DomainID    string `json:"domain_id,omitempty" jsonschema:"Optional. The domain ID if already known"`
	Description string `json:"description,omitempty" jsonschema:"Optional. A description for the business term"`
}

// Output represents the structured result of the preparation check.
type Output struct {
	Status           string                 `json:"status" jsonschema:"Status of the preparation: ready, incomplete, needs_clarification, or duplicate_found"`
	Message          string                 `json:"message" jsonschema:"Human-readable explanation of the status"`
	ResolvedDomain   *DomainInfo            `json:"resolved_domain,omitempty" jsonschema:"Optional. The resolved domain information"`
	Duplicates       []DuplicateAssetInfo   `json:"duplicates,omitempty" jsonschema:"Optional. List of existing assets that may be duplicates"`
	AttributeSchema  []AttributeSchemaEntry `json:"attribute_schema,omitempty" jsonschema:"Optional. Full attribute schema for the business term type"`
	AvailableDomains []DomainInfo           `json:"available_domains,omitempty" jsonschema:"Optional. Available domains for selection when domain is missing or ambiguous"`
}

// DomainInfo represents a resolved domain.
type DomainInfo struct {
	ID   string `json:"id" jsonschema:"Domain ID"`
	Name string `json:"name" jsonschema:"Domain name"`
}

// DuplicateAssetInfo represents an existing asset that may be a duplicate.
type DuplicateAssetInfo struct {
	ID     string     `json:"id" jsonschema:"Asset ID"`
	Name   string     `json:"name" jsonschema:"Asset name"`
	Domain DomainInfo `json:"domain" jsonschema:"Domain of the asset"`
}

// AttributeSchemaEntry represents the full schema for a single attribute type.
type AttributeSchemaEntry struct {
	ID            string             `json:"id" jsonschema:"Attribute type ID"`
	Name          string             `json:"name" jsonschema:"Attribute type name"`
	Kind          string             `json:"kind" jsonschema:"Attribute data type"`
	Required      bool               `json:"required" jsonschema:"Whether this attribute is mandatory"`
	Constraints   *AttributeConstraints `json:"constraints,omitempty" jsonschema:"Optional. Validation rules and limits"`
	AllowedValues []string           `json:"allowed_values,omitempty" jsonschema:"Optional. Permitted values if constrained"`
	RelationType  *RelationTypeInfo  `json:"relation_type,omitempty" jsonschema:"Optional. Relation type with direction and target"`
}

// AttributeConstraints represents validation constraints for an attribute.
type AttributeConstraints struct {
	MinLength *int `json:"min_length,omitempty" jsonschema:"Optional. Minimum string length"`
	MaxLength *int `json:"max_length,omitempty" jsonschema:"Optional. Maximum string length"`
}

// RelationTypeInfo represents a relation type with direction and target.
type RelationTypeInfo struct {
	ID         string  `json:"id" jsonschema:"Relation type ID"`
	Role       string  `json:"role" jsonschema:"Role name"`
	CoRole     string  `json:"co_role" jsonschema:"Co-role name"`
	Direction  string  `json:"direction" jsonschema:"Direction of the relation"`
	TargetType TypeRef `json:"target_type" jsonschema:"Target type reference"`
}

// TypeRef is a simple reference to a type by ID and name.
type TypeRef struct {
	ID   string `json:"id" jsonschema:"Type ID"`
	Name string `json:"name" jsonschema:"Type name"`
}

// NewTool creates a new prepare_add_business_term tool.
func NewTool(collibraClient *http.Client) *chip.Tool[Input, Output] {
	return &chip.Tool[Input, Output]{
		Name:        "prepare_add_business_term",
		Description: "Validate business term data, resolve domains, check for duplicates, and hydrate attribute schemas. Returns structured status with pre-fetched options for missing fields.",
		Handler:     handler(collibraClient),
		Permissions: []string{"dgc.ai-copilot"},
	}
}

func handler(collibraClient *http.Client) chip.ToolHandlerFunc[Input, Output] {
	return func(ctx context.Context, input Input) (Output, error) {
		// Step 1: List all domains for resolution and pre-fetching options.
		domains, err := clients.PrepareAddBusinessTermListDomains(ctx, collibraClient)
		if err != nil {
			return Output{}, err
		}

		availableDomains := make([]DomainInfo, len(domains))
		for i, d := range domains {
			availableDomains[i] = DomainInfo{ID: d.ID, Name: d.Name}
		}

		// Step 2: Resolve domain.
		var resolvedDomain *DomainInfo

		if input.DomainID != "" {
			// Validate that the provided domain ID exists.
			domain, err := clients.PrepareAddBusinessTermGetDomain(ctx, collibraClient, input.DomainID)
			if err != nil {
				return Output{}, err
			}
			resolvedDomain = &DomainInfo{ID: domain.ID, Name: domain.Name}
		} else if input.DomainName != "" {
			// Resolve domain by name — check for exact (case-insensitive) matches.
			var matches []DomainInfo
			for _, d := range domains {
				if strings.EqualFold(d.Name, input.DomainName) {
					matches = append(matches, DomainInfo{ID: d.ID, Name: d.Name})
				}
			}

			switch len(matches) {
			case 1:
				resolvedDomain = &matches[0]
			case 0:
				// No match — domain remains unresolved, will result in incomplete status.
			default:
				// Multiple matches — needs clarification.
				return Output{
					Status:           "needs_clarification",
					Message:          "Multiple domains match the provided name. Please select one.",
					AvailableDomains: matches,
				}, nil
			}
		}

		// Step 3: Get business term asset type configuration.
		assetType, err := clients.PrepareAddBusinessTermGetAssetType(ctx, collibraClient, businessTermPublicID)
		if err != nil {
			return Output{}, err
		}

		// Step 4: Retrieve attribute assignments for the business term type.
		assignments, err := clients.PrepareAddBusinessTermGetAssignments(ctx, collibraClient, assetType.ID)
		if err != nil {
			return Output{}, err
		}

		// Step 5: Hydrate full attribute schemas.
		attributeSchema := make([]AttributeSchemaEntry, 0, len(assignments))
		for _, assignment := range assignments {
			attrType, err := clients.PrepareAddBusinessTermGetAttributeType(ctx, collibraClient, assignment.AttributeType.ID)
			if err != nil {
				return Output{}, err
			}

			entry := AttributeSchemaEntry{
				ID:            attrType.ID,
				Name:          attrType.Name,
				Kind:          attrType.Kind,
				Required:      attrType.Required || assignment.Min > 0,
				AllowedValues: attrType.AllowedValues,
			}

			if attrType.Constraints != nil {
				entry.Constraints = &AttributeConstraints{
					MinLength: attrType.Constraints.MinLength,
					MaxLength: attrType.Constraints.MaxLength,
				}
			}

			if attrType.RelationType != nil {
				entry.RelationType = &RelationTypeInfo{
					ID:        attrType.RelationType.ID,
					Role:      attrType.RelationType.Role,
					CoRole:    attrType.RelationType.CoRole,
					Direction: attrType.RelationType.Direction,
					TargetType: TypeRef{
						ID:   attrType.RelationType.TargetType.ID,
						Name: attrType.RelationType.TargetType.Name,
					},
				}
			}

			attributeSchema = append(attributeSchema, entry)
		}

		// Step 6: Search for duplicate assets.
		var duplicates []DuplicateAssetInfo
		if input.Name != "" {
			assets, err := clients.PrepareAddBusinessTermSearchAssets(ctx, collibraClient, input.Name, assetType.ID)
			if err != nil {
				return Output{}, err
			}

			for _, a := range assets {
				duplicates = append(duplicates, DuplicateAssetInfo{
					ID:   a.ID,
					Name: a.Name,
					Domain: DomainInfo{
						ID:   a.Domain.ID,
						Name: a.Domain.Name,
					},
				})
			}
		}

		// Step 7: Determine status.
		if len(duplicates) > 0 {
			return Output{
				Status:           "duplicate_found",
				Message:          "Existing business terms match the provided name.",
				ResolvedDomain:   resolvedDomain,
				Duplicates:       duplicates,
				AttributeSchema:  attributeSchema,
				AvailableDomains: availableDomains,
			}, nil
		}

		if input.Name == "" || resolvedDomain == nil {
			return Output{
				Status:           "incomplete",
				Message:          "Missing required fields. Please provide name and domain.",
				ResolvedDomain:   resolvedDomain,
				AttributeSchema:  attributeSchema,
				AvailableDomains: availableDomains,
			}, nil
		}

		return Output{
			Status:          "ready",
			Message:         "All required data is present and validated. Ready to add business term.",
			ResolvedDomain:  resolvedDomain,
			AttributeSchema: attributeSchema,
		}, nil
	}
}
