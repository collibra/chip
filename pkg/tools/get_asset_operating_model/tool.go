// Package get_asset_operating_model implements the get_asset_operating_model
// MCP tool: a read-only lookup of the attributes and relations an asset type
// has in a given domain of the customer's operating model. Its output mirrors
// prepare_create_asset (status, message, resolved, attributeSchema,
// relationTypes, availableStatuses) so that it can replace that tool.
package get_asset_operating_model

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/collibra/chip/pkg/chip"
	"github.com/collibra/chip/pkg/clients"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const complexRelationKind = "ComplexRelationType"

// Status is the lookup outcome.
type Status string

const (
	// StatusReady means the asset type and domain were resolved and the
	// operating model was retrieved.
	StatusReady Status = "ready"
	// StatusNeedsClarification means the asset type or domain could not be
	// resolved, or the asset type is not allowed in the domain.
	StatusNeedsClarification Status = "needs_clarification"
)

type Input struct {
	AssetType string `json:"assetType" jsonschema:"Asset type identifier — accepts a UUID, the type's publicId (e.g. 'BusinessTerm'), or its display name (e.g. 'Business Term'). Resolution tries those in order. Use list_asset_types to find one."`
	Domain    string `json:"domain,omitempty" jsonschema:"Optional. Domain identifier — accepts a UUID or the domain's display name (case-insensitive). The domain decides which attributes and relations apply. Omit it to get the asset type's default operating model, e.g. before querying; pass it before create_asset."`
}

type Output struct {
	Status            Status                 `json:"status" jsonschema:"ready when the operating model was retrieved; needs_clarification when the asset type or domain could not be resolved or the asset type is not allowed in the domain."`
	Message           string                 `json:"message" jsonschema:"Human-readable summary of the outcome."`
	Resolved          *Resolved              `json:"resolved,omitempty" jsonschema:"The resolved asset type and domain. Pass these IDs to create_asset."`
	AttributeSchema   []AttributeSchemaEntry `json:"attributeSchema,omitempty" jsonschema:"Attribute slots for the asset type in the domain: which attributes are required or read-only, what kind of value they take, and whether the value is rich text."`
	RelationTypes     []RelationSchemaEntry  `json:"relationTypes,omitempty" jsonschema:"Relation slots for the asset type in the domain: the relation roles available, each with the relation type's UUID and publicId."`
	AvailableStatuses []StatusOption         `json:"availableStatuses,omitempty" jsonschema:"All statuses defined on this instance, so the agent can pick a non-default initial status when calling create_asset."`
}

// Resolved holds the resolved asset type and domain.
type Resolved struct {
	AssetTypeID       string `json:"assetTypeId" jsonschema:"UUID of the asset type."`
	AssetTypeName     string `json:"assetTypeName" jsonschema:"Display name of the asset type."`
	AssetTypePublicID string `json:"assetTypePublicId,omitempty" jsonschema:"PublicId of the asset type, when known."`
	DomainID          string `json:"domainId,omitempty" jsonschema:"UUID of the domain. Absent when no domain was given."`
	DomainName        string `json:"domainName,omitempty" jsonschema:"Display name of the domain. Absent when no domain was given."`
	DomainTypeID      string `json:"domainTypeId,omitempty" jsonschema:"UUID of the domain's domain type."`
	DomainTypeName    string `json:"domainTypeName,omitempty" jsonschema:"Display name of the domain's domain type."`
}

type AttributeSchemaEntry struct {
	AttributeTypeID string   `json:"attributeTypeId" jsonschema:"UUID of the attribute type — pass this in attributes[].typeId to create_asset."`
	Name            string   `json:"name" jsonschema:"Display name of the attribute (e.g. 'Definition', 'Note'). Also accepted by create_asset."`
	PublicID        string   `json:"publicId,omitempty" jsonschema:"PublicId of the attribute type."`
	Kind            string   `json:"kind" jsonschema:"Attribute-type discriminator (e.g. 'StringAttributeType', 'NumericAttributeType')."`
	Required        bool     `json:"required" jsonschema:"True when the assignment's minimumOccurrences > 0. Collibra doesn't always enforce this at create time — it can be an attestation/workflow signal rather than a hard create-time requirement."`
	Min             int      `json:"min" jsonschema:"Minimum number of occurrences."`
	Max             *int     `json:"max,omitempty" jsonschema:"Maximum number of occurrences. Absent when unbounded."`
	ReadOnly        bool     `json:"readOnly,omitempty" jsonschema:"True when the assignment marks the attribute read-only because Collibra sets its value. create_asset rejects it in attributes[] and does not require it, even when required is true."`
	StringType      string   `json:"stringType,omitempty" jsonschema:"For string-kind attributes: 'RICH_TEXT' means create_asset will run the value through Markdown→HTML conversion."`
	Description     string   `json:"description,omitempty" jsonschema:"Server-defined description of the attribute."`
	AllowedValues   []string `json:"allowedValues,omitempty" jsonschema:"Permitted values for list-type attributes."`
}

type RelationSchemaEntry struct {
	RelationTypeID string     `json:"relationTypeId" jsonschema:"UUID of the relation type. A relation type assigned in both directions appears once per direction, so the same relationTypeId may occur in two entries that differ by direction."`
	PublicID       string     `json:"publicId,omitempty" jsonschema:"PublicId of the relation type."`
	Kind           string     `json:"kind,omitempty" jsonschema:"Discriminator: 'RelationType' for a simple binary relation, 'ComplexRelationType' for a complex one. Complex relation types carry legs[] instead of a single role/coRole."`
	Role           string     `json:"role" jsonschema:"Forward role name of the relation (e.g. 'is part of'). Empty for complex relation types — see legs[] instead."`
	CoRole         string     `json:"coRole,omitempty" jsonschema:"Reverse role name (e.g. 'contains')."`
	Direction      string     `json:"direction,omitempty" jsonschema:"Direction of the relation from the asset's side: 'TO_TARGET' (the asset is the source leg) or 'TO_SOURCE' (the asset is the target leg)."`
	TargetTypeID   string     `json:"targetTypeId,omitempty" jsonschema:"UUID of the asset type on the other leg."`
	TargetTypeName string     `json:"targetTypeName,omitempty" jsonschema:"Display name of the asset type on the other leg."`
	Legs           []LegEntry `json:"legs,omitempty" jsonschema:"For complex relation types only: the two or more legs, each with its own role and asset type. Absent for simple relations."`
}

type LegEntry struct {
	Role                 string `json:"role" jsonschema:"Role name of this leg."`
	CoRole               string `json:"coRole,omitempty" jsonschema:"Reverse role name of this leg."`
	AssetTypeID          string `json:"assetTypeId,omitempty" jsonschema:"UUID of the asset type on this leg."`
	AssetTypeName        string `json:"assetTypeName,omitempty" jsonschema:"Display name of the asset type on this leg."`
	RelationTypePublicID string `json:"relationTypePublicId,omitempty" jsonschema:"PublicId of the simple relation type underlying this leg."`
	Min                  int    `json:"min" jsonschema:"Minimum number of occurrences for this leg."`
	Max                  *int   `json:"max,omitempty" jsonschema:"Maximum number of occurrences for this leg. Absent when unbounded."`
}

type StatusOption struct {
	ID   string `json:"id" jsonschema:"UUID of the status."`
	Name string `json:"name" jsonschema:"Display name of the status."`
}

func NewTool(collibraClient *http.Client) *chip.Tool[Input, Output] {
	return &chip.Tool[Input, Output]{
		Name:        "get_asset_operating_model",
		Title:       "Get Asset Operating Model",
		Description: "Get the attributes and relations an asset type has in a given domain of this Collibra instance's operating model. Use it before querying so the first query uses valid attribute and relation names, and before create_asset to learn which attributes are required or read-only, which are rich text, which relations apply and which statuses can be set. assetType (UUID, publicId or name) is required. domain (UUID or name) is optional: pass it to get the model that applies in that domain, which is what create_asset needs; omit it to get the asset type's default model. Replaces prepare_create_asset.",
		Handler:     handler(collibraClient),
		Permissions: []string{},
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, DestructiveHint: chip.Ptr(false), IdempotentHint: true, OpenWorldHint: chip.Ptr(false)},
	}
}

func handler(collibraClient *http.Client) chip.ToolHandlerFunc[Input, Output] {
	return func(ctx context.Context, input Input) (Output, error) {
		assetTypeValue := strings.TrimSpace(input.AssetType)
		if assetTypeValue == "" {
			return Output{}, errors.New("assetType is required")
		}
		domainValue := strings.TrimSpace(input.Domain)

		assetType, err := resolveAssetType(ctx, collibraClient, assetTypeValue)
		if err != nil {
			return needsClarification(notResolvedMessage("Asset type", assetTypeValue, err)), nil
		}

		var domain *clients.PrepareCreateDomain
		if domainValue != "" {
			domain, err = resolveDomain(ctx, collibraClient, domainValue)
			if err != nil {
				return needsClarification(notResolvedMessage("Domain", domainValue, err)), nil
			}
		}

		assignment, err := fetchAssignment(ctx, collibraClient, assetType, domain)
		if err != nil {
			return assignmentFailed(ctx, collibraClient, err, assetType, domain), nil
		}

		out := Output{
			Status:          StatusReady,
			Message:         readyMessage(assetType, domain, assignment),
			Resolved:        resolved(assetType, domain),
			AttributeSchema: attributeEntries(assignment.Attributes),
			RelationTypes:   relationEntries(assignment.Relations),
		}

		if err := hydrateAttributes(ctx, collibraClient, out.AttributeSchema); err != nil {
			out.Message += " (attribute details could not be fully hydrated: " + err.Error() + ")"
		}
		if err := hydrateRelations(ctx, collibraClient, out.RelationTypes); err != nil {
			out.Message += " (relation details could not be fully hydrated: " + err.Error() + ")"
		}

		statuses, _ := clients.ListStatusesAll(ctx, collibraClient)
		out.AvailableStatuses = statusOptions(statuses)

		return out, nil
	}
}

func needsClarification(message string) Output {
	return Output{Status: StatusNeedsClarification, Message: message}
}

func notResolvedMessage(label, value string, err error) string {
	return fmt.Sprintf("%s %q could not be resolved: %v.", label, value, err)
}

// fetchAssignment looks up the assignment for the asset type in the domain, or
// the asset type's default assignment when no domain was given.
func fetchAssignment(ctx context.Context, collibraClient *http.Client, assetType *clients.PrepareCreateAssetType, domain *clients.PrepareCreateDomain) (*clients.PrepareCreateScopedAssignment, error) {
	if domain == nil {
		return clients.GetDefaultAssignmentForAssetType(ctx, collibraClient, assetType.ID)
	}
	return clients.GetAssignmentForAssetTypeAndDomain(ctx, collibraClient, assetType.ID, domain.ID)
}

func readyMessage(assetType *clients.PrepareCreateAssetType, domain *clients.PrepareCreateDomain, assignment *clients.PrepareCreateScopedAssignment) string {
	counts := fmt.Sprintf("%d attribute slot(s), %d relation slot(s) in scope.", len(assignment.Attributes), len(assignment.Relations))
	if domain == nil {
		return fmt.Sprintf("Resolved asset type %q without a domain: this is its default operating model, and a domain can change which attributes and relations apply. %s", assetType.Name, counts)
	}
	return fmt.Sprintf("Resolved asset type %q in domain %q. %s", assetType.Name, domain.Name, counts)
}

// assignmentFailed explains why no assignment could be returned. A missing
// assignment means the asset type is not allowed in the domain; anything else is
// reported as a lookup failure so it isn't mistaken for a modelling problem.
func assignmentFailed(ctx context.Context, collibraClient *http.Client, err error, assetType *clients.PrepareCreateAssetType, domain *clients.PrepareCreateDomain) Output {
	switch {
	case errors.Is(err, clients.ErrNoDefaultAssignment):
		return needsClarification(fmt.Sprintf("Asset type %q only has attributes and relations in specific domains, so a domain is required. Pass the domain the asset is or will be in.", assetType.Name))
	case !errors.Is(err, clients.ErrAssignmentNotFound):
		return needsClarification(fmt.Sprintf("Failed to retrieve operating model: %s", err.Error()))
	case domain == nil:
		return needsClarification(fmt.Sprintf("Asset type %q has no attributes or relations assigned in this instance.", assetType.Name))
	}
	domainTypeName := ""
	if domain.Type != nil {
		domainTypeName = domain.Type.Name
	}
	return needsClarification(clients.NotAllowedMessage(ctx, collibraClient, assetType.ID, assetType.Name, domain.Name, domainTypeName))
}

func resolved(assetType *clients.PrepareCreateAssetType, domain *clients.PrepareCreateDomain) *Resolved {
	r := &Resolved{
		AssetTypeID:       assetType.ID,
		AssetTypeName:     assetType.Name,
		AssetTypePublicID: assetType.PublicID,
	}
	if domain == nil {
		return r
	}
	r.DomainID = domain.ID
	r.DomainName = domain.Name
	if domain.Type != nil {
		r.DomainTypeID = domain.Type.ID
		r.DomainTypeName = domain.Type.Name
	}
	return r
}

func attributeEntries(in []clients.PrepareCreateScopedAttribute) []AttributeSchemaEntry {
	out := make([]AttributeSchemaEntry, len(in))
	for i, a := range in {
		out[i] = AttributeSchemaEntry{
			AttributeTypeID: a.AttributeTypeID,
			Name:            a.AttributeTypeName,
			PublicID:        a.AttributeTypePublicID,
			Kind:            a.Kind,
			Required:        a.Required,
			Min:             a.Min,
			Max:             a.Max,
			ReadOnly:        a.ReadOnly,
		}
	}
	return out
}

func relationEntries(in []clients.PrepareCreateScopedRelation) []RelationSchemaEntry {
	out := make([]RelationSchemaEntry, len(in))
	for i, r := range in {
		entry := RelationSchemaEntry{
			RelationTypeID: r.RelationTypeID,
			PublicID:       r.RelationTypePublicID,
			Kind:           r.Kind,
			Direction:      r.Direction,
		}
		if r.TargetType != nil {
			entry.TargetTypeID = r.TargetType.ID
			entry.TargetTypeName = r.TargetType.Name
		}
		out[i] = entry
	}
	return out
}

func statusOptions(in []clients.PrepareCreateStatus) []StatusOption {
	out := make([]StatusOption, len(in))
	for i, s := range in {
		out[i] = StatusOption{ID: s.ID, Name: s.Name}
	}
	return out
}

// hydrateAttributes adds stringType, description and allowedValues, which the
// assignment payload omits. Failures on individual attributes are tolerated so
// one missing detail doesn't blank the whole schema; the first error is returned.
func hydrateAttributes(ctx context.Context, collibraClient *http.Client, schema []AttributeSchemaEntry) error {
	var firstErr error
	for i := range schema {
		details, err := clients.GetAttributeTypeFull(ctx, collibraClient, schema[i].AttributeTypeID)
		if err != nil {
			firstErr = firstError(firstErr, err)
			continue
		}
		schema[i].StringType = details.StringType
		schema[i].Description = details.Description
		if len(details.AllowedValues) > 0 {
			schema[i].AllowedValues = details.AllowedValues
		}
		if schema[i].PublicID == "" {
			schema[i].PublicID = details.PublicID
		}
	}
	return firstErr
}

// hydrateRelations adds the role names and leg types that the assignment payload
// omits. Failures on individual relations are tolerated; the first error is returned.
func hydrateRelations(ctx context.Context, collibraClient *http.Client, relations []RelationSchemaEntry) error {
	var firstErr error
	for i := range relations {
		var err error
		if relations[i].Kind == complexRelationKind {
			err = hydrateComplexRelation(ctx, collibraClient, &relations[i])
		} else {
			err = hydrateSimpleRelation(ctx, collibraClient, &relations[i])
		}
		firstErr = firstError(firstErr, err)
	}
	return firstErr
}

func hydrateSimpleRelation(ctx context.Context, collibraClient *http.Client, rel *RelationSchemaEntry) error {
	details, err := clients.GetRelationTypeFull(ctx, collibraClient, rel.RelationTypeID)
	if err != nil {
		return err
	}
	rel.Role = details.Role
	rel.CoRole = details.CoRole
	if rel.PublicID == "" {
		rel.PublicID = details.PublicID
	}
	if rel.TargetTypeID != "" {
		return nil
	}
	other := details.TargetType
	if rel.Direction == "TO_SOURCE" {
		other = details.SourceType
	}
	if other != nil {
		rel.TargetTypeID = other.ID
		rel.TargetTypeName = other.Name
	}
	return nil
}

func hydrateComplexRelation(ctx context.Context, collibraClient *http.Client, rel *RelationSchemaEntry) error {
	details, err := clients.GetComplexRelationTypeFull(ctx, collibraClient, rel.RelationTypeID)
	if err != nil {
		return err
	}
	if rel.PublicID == "" {
		rel.PublicID = details.PublicID
	}
	rel.Legs = make([]LegEntry, len(details.Legs))
	for i, leg := range details.Legs {
		rel.Legs[i] = LegEntry{
			Role:                 leg.Role,
			CoRole:               leg.CoRole,
			AssetTypeID:          leg.AssetTypeID,
			AssetTypeName:        leg.AssetTypeName,
			RelationTypePublicID: leg.RelationTypePublicID,
			Min:                  leg.Min,
			Max:                  leg.Max,
		}
	}
	return nil
}

func firstError(current, next error) error {
	if current != nil {
		return current
	}
	return next
}
