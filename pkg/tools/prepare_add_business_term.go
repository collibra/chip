package tools

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/collibra/chip/pkg/chip"
	"github.com/collibra/chip/pkg/clients"
)

// PrepareAddBusinessTermInput defines the input for the prepare_add_business_term tool.
type PrepareAddBusinessTermInput struct {
	Name        string `json:"name" jsonschema:"The name of the business term to create"`
	DomainID    string `json:"domain_id,omitempty" jsonschema:"Optional. The ID of the domain to create the business term in"`
	DomainName  string `json:"domain_name,omitempty" jsonschema:"Optional. The name of the domain to search for"`
	Description string `json:"description,omitempty" jsonschema:"Optional. Description of the business term"`
}

// PrepareAddBusinessTermDomainInfo represents resolved domain information in the output.
type PrepareAddBusinessTermDomainInfo struct {
	ID          string `json:"id" jsonschema:"Domain identifier"`
	Name        string `json:"name" jsonschema:"Domain name"`
	Description string `json:"description" jsonschema:"Domain description"`
}

// PrepareAddBusinessTermDuplicateInfo represents a duplicate business term found.
type PrepareAddBusinessTermDuplicateInfo struct {
	ID          string `json:"id" jsonschema:"Asset identifier"`
	Name        string `json:"name" jsonschema:"Asset name"`
	DomainID    string `json:"domain_id" jsonschema:"Domain identifier of the duplicate"`
	Description string `json:"description" jsonschema:"Asset description"`
}

// PrepareAddBusinessTermConstraintInfo represents constraints on an attribute.
type PrepareAddBusinessTermConstraintInfo struct {
	MinLength     int      `json:"min_length,omitempty" jsonschema:"Optional. Minimum length constraint"`
	MaxLength     int      `json:"max_length,omitempty" jsonschema:"Optional. Maximum length constraint"`
	Pattern       string   `json:"pattern,omitempty" jsonschema:"Optional. Regex pattern constraint"`
	AllowedValues []string `json:"allowed_values,omitempty" jsonschema:"Optional. List of allowed values"`
}

// PrepareAddBusinessTermRelationTypeInfo represents a relation type in the attribute schema.
type PrepareAddBusinessTermRelationTypeInfo struct {
	ID                string `json:"id" jsonschema:"Relation type identifier"`
	Name              string `json:"name" jsonschema:"Relation type name"`
	Direction         string `json:"direction" jsonschema:"Relation direction (e.g. outgoing, incoming)"`
	TargetAssetTypeID string `json:"target_asset_type_id" jsonschema:"Target asset type identifier"`
}

// PrepareAddBusinessTermAttributeInfo represents a hydrated attribute in the schema.
type PrepareAddBusinessTermAttributeInfo struct {
	ID            string                                       `json:"id" jsonschema:"Attribute type identifier"`
	Name          string                                       `json:"name" jsonschema:"Attribute type name"`
	Kind          string                                       `json:"kind" jsonschema:"Attribute kind (e.g. String, Boolean, Numeric)"`
	Required      bool                                         `json:"required" jsonschema:"Whether this attribute is required for the business term"`
	Description   string                                       `json:"description" jsonschema:"Attribute description"`
	Constraints   *PrepareAddBusinessTermConstraintInfo        `json:"constraints,omitempty" jsonschema:"Optional. Constraints for this attribute"`
	RelationTypes []PrepareAddBusinessTermRelationTypeInfo     `json:"relation_types,omitempty" jsonschema:"Optional. Relation types with direction and target"`
}

// PrepareAddBusinessTermOutput defines the output for the prepare_add_business_term tool.
type PrepareAddBusinessTermOutput struct {
	Status           string                                    `json:"status" jsonschema:"Preparation status: ready, incomplete, needs_clarification, or duplicate_found"`
	Message          string                                    `json:"message" jsonschema:"Human-readable explanation of the current status"`
	Domain           *PrepareAddBusinessTermDomainInfo         `json:"domain,omitempty" jsonschema:"Optional. Resolved domain information"`
	AvailableDomains []PrepareAddBusinessTermDomainInfo        `json:"available_domains,omitempty" jsonschema:"Optional. Available domains for selection when domain is missing or ambiguous"`
	Duplicates       []PrepareAddBusinessTermDuplicateInfo     `json:"duplicates,omitempty" jsonschema:"Optional. Duplicate business terms found in the target domain"`
	AttributeSchema  []PrepareAddBusinessTermAttributeInfo     `json:"attribute_schema,omitempty" jsonschema:"Optional. Hydrated attribute schema for business term creation"`
}

// NewPrepareAddBusinessTermTool creates a new prepare_add_business_term tool instance.
func NewPrepareAddBusinessTermTool(collibraClient *http.Client) *chip.Tool[PrepareAddBusinessTermInput, PrepareAddBusinessTermOutput] {
	return &chip.Tool[PrepareAddBusinessTermInput, PrepareAddBusinessTermOutput]{
		Name:        "prepare_add_business_term",
		Description: "Validate and prepare business term creation by resolving domains, checking for duplicates, and hydrating attribute schemas. Returns a structured status indicating readiness.",
		Handler:     handlePrepareAddBusinessTerm(collibraClient),
		Permissions: []string{"dgc.ai-copilot"},
	}
}

func handlePrepareAddBusinessTerm(collibraClient *http.Client) chip.ToolHandlerFunc[PrepareAddBusinessTermInput, PrepareAddBusinessTermOutput] {
	return func(ctx context.Context, input PrepareAddBusinessTermInput) (PrepareAddBusinessTermOutput, error) {
		// Step 1: Check if name is provided
		if input.Name == "" {
			domains, _ := clients.PrepareAddBusinessTermListDomains(ctx, collibraClient)
			return PrepareAddBusinessTermOutput{
				Status:           "incomplete",
				Message:          "Business term name is required.",
				AvailableDomains: convertDomainResponses(domains),
			}, nil
		}

		// Step 2: Resolve domain
		var resolvedDomain *PrepareAddBusinessTermDomainInfo

		if input.DomainID != "" {
			d, err := clients.PrepareAddBusinessTermGetDomain(ctx, collibraClient, input.DomainID)
			if err != nil {
				domains, _ := clients.PrepareAddBusinessTermListDomains(ctx, collibraClient)
				return PrepareAddBusinessTermOutput{
					Status:           "needs_clarification",
					Message:          fmt.Sprintf("Domain with ID '%s' was not found. Please select a valid domain.", input.DomainID),
					AvailableDomains: convertDomainResponses(domains),
				}, nil
			}
			resolvedDomain = &PrepareAddBusinessTermDomainInfo{
				ID:          d.ID,
				Name:        d.Name,
				Description: d.Description,
			}
		} else if input.DomainName != "" {
			domains, err := clients.PrepareAddBusinessTermListDomains(ctx, collibraClient)
			if err != nil {
				return PrepareAddBusinessTermOutput{}, fmt.Errorf("listing domains: %w", err)
			}
			var matches []PrepareAddBusinessTermDomainInfo
			for _, d := range domains {
				if strings.EqualFold(d.Name, input.DomainName) {
					matches = append(matches, PrepareAddBusinessTermDomainInfo{
						ID:          d.ID,
						Name:        d.Name,
						Description: d.Description,
					})
				}
			}
			if len(matches) == 0 {
				return PrepareAddBusinessTermOutput{
					Status:           "needs_clarification",
					Message:          fmt.Sprintf("No domain found matching name '%s'. Please select from available domains.", input.DomainName),
					AvailableDomains: convertDomainResponses(domains),
				}, nil
			} else if len(matches) > 1 {
				return PrepareAddBusinessTermOutput{
					Status:           "needs_clarification",
					Message:          fmt.Sprintf("Multiple domains match name '%s'. Please select the correct domain.", input.DomainName),
					AvailableDomains: matches,
				}, nil
			}
			resolvedDomain = &matches[0]
		} else {
			domains, _ := clients.PrepareAddBusinessTermListDomains(ctx, collibraClient)
			return PrepareAddBusinessTermOutput{
				Status:           "incomplete",
				Message:          "Domain is required. Please provide a domain_id or domain_name.",
				AvailableDomains: convertDomainResponses(domains),
			}, nil
		}

		// Step 3: Get business term asset type
		assetType, err := clients.PrepareAddBusinessTermGetAssetType(ctx, collibraClient, clients.BusinessTermAssetTypePublicID)
		if err != nil {
			return PrepareAddBusinessTermOutput{}, fmt.Errorf("getting business term asset type: %w", err)
		}

		// Step 4: Check for duplicates
		searchResult, err := clients.PrepareAddBusinessTermSearchAssets(ctx, collibraClient, input.Name, assetType.ID, resolvedDomain.ID)
		if err != nil {
			return PrepareAddBusinessTermOutput{}, fmt.Errorf("searching for duplicate assets: %w", err)
		}
		if len(searchResult.Results) > 0 {
			var duplicates []PrepareAddBusinessTermDuplicateInfo
			for _, a := range searchResult.Results {
				duplicates = append(duplicates, PrepareAddBusinessTermDuplicateInfo{
					ID:          a.ID,
					Name:        a.Name,
					DomainID:    a.Domain.ID,
					Description: a.Description,
				})
			}
			return PrepareAddBusinessTermOutput{
				Status:     "duplicate_found",
				Message:    fmt.Sprintf("Found %d existing business term(s) with name '%s' in the specified domain.", len(duplicates), input.Name),
				Domain:     resolvedDomain,
				Duplicates: duplicates,
			}, nil
		}

		// Step 5: Get attribute schema
		assignments, err := clients.PrepareAddBusinessTermGetAssignments(ctx, collibraClient, assetType.ID)
		if err != nil {
			return PrepareAddBusinessTermOutput{}, fmt.Errorf("getting attribute assignments: %w", err)
		}

		var schema []PrepareAddBusinessTermAttributeInfo
		for _, assignment := range assignments {
			attrType, err := clients.PrepareAddBusinessTermGetAttributeType(ctx, collibraClient, assignment.AttributeTypeID)
			if err != nil {
				return PrepareAddBusinessTermOutput{}, fmt.Errorf("getting attribute type %s: %w", assignment.AttributeTypeID, err)
			}

			attr := PrepareAddBusinessTermAttributeInfo{
				ID:          attrType.ID,
				Name:        attrType.Name,
				Kind:        attrType.Kind,
				Required:    assignment.Required,
				Description: attrType.Description,
			}

			if attrType.Constraints != nil {
				attr.Constraints = &PrepareAddBusinessTermConstraintInfo{
					MinLength:     attrType.Constraints.MinLength,
					MaxLength:     attrType.Constraints.MaxLength,
					Pattern:       attrType.Constraints.Pattern,
					AllowedValues: attrType.Constraints.AllowedValues,
				}
			}

			for _, rt := range attrType.RelationTypes {
				attr.RelationTypes = append(attr.RelationTypes, PrepareAddBusinessTermRelationTypeInfo{
					ID:                rt.ID,
					Name:              rt.Name,
					Direction:         rt.Direction,
					TargetAssetTypeID: rt.TargetAssetTypeID,
				})
			}

			schema = append(schema, attr)
		}

		// Step 6: Return ready
		return PrepareAddBusinessTermOutput{
			Status:          "ready",
			Message:         fmt.Sprintf("Business term '%s' is ready to be created in domain '%s'.", input.Name, resolvedDomain.Name),
			Domain:          resolvedDomain,
			AttributeSchema: schema,
		}, nil
	}
}

// convertDomainResponses converts client domain responses to output domain info.
func convertDomainResponses(domains []clients.PrepareAddBusinessTermDomainResponse) []PrepareAddBusinessTermDomainInfo {
	if domains == nil {
		return nil
	}
	result := make([]PrepareAddBusinessTermDomainInfo, len(domains))
	for i, d := range domains {
		result[i] = PrepareAddBusinessTermDomainInfo{
			ID:          d.ID,
			Name:        d.Name,
			Description: d.Description,
		}
	}
	return result
}
