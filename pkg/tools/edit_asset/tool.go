// Package edit_asset implements the edit_asset MCP tool: a single entry point
// for updating properties, attributes, relations, responsibilities, and tags
// on any existing Collibra asset via a typed list of operations. The MCP
// server resolves names to IDs internally and validates each operation against
// the asset's scoped assignment before executing any writes, so the calling
// agent never needs to know which REST endpoint to hit.
package edit_asset

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/collibra/chip/pkg/chip"
	"github.com/collibra/chip/pkg/clients"
	"github.com/collibra/chip/pkg/tools/validation"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// normalize lowercases and trims whitespace, used as a key for all
// human-name lookups (attributes, roles, statuses, relation roles).
// Collibra rarely distinguishes names by case in practice, so a forgiving
// match prevents a class of LLM-typos.
func normalize(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}

// suggestionSuffix renders a short list of valid names to append to a
// "not valid" error so the calling model can self-correct in one step
// instead of round-tripping through another tool.
func suggestionSuffix(label string, names []string, max int) string {
	if len(names) == 0 {
		return ""
	}
	sort.Strings(names)
	if len(names) <= max {
		return fmt.Sprintf(" %s available: %s.", label, strings.Join(names, ", "))
	}
	return fmt.Sprintf(" %s available: %s (and %d more).", label, strings.Join(names[:max], ", "), len(names)-max)
}

// OperationType enumerates the kinds of edits edit_asset can perform. Phases 2+
// add add_relation, remove_relation, add_tag, set_responsibility.
type OperationType string

const (
	OpUpdateAttribute      OperationType = "update_attribute"
	OpAddAttribute         OperationType = "add_attribute"
	OpRemoveAttribute      OperationType = "remove_attribute"
	OpUpdateProperty       OperationType = "update_property"
	OpAddRelation          OperationType = "add_relation"
	OpRemoveRelation       OperationType = "remove_relation"
	OpAddTag               OperationType = "add_tag"
	OpSetResponsibility    OperationType = "set_responsibility"
	OpRemoveResponsibility OperationType = "remove_responsibility"
)

// Whitelisted fields for update_property. Keeping this narrow avoids letting
// the agent PATCH fields that require a different flow (typeId, domainId).
const (
	PropertyName        = "name"
	PropertyDisplayName = "displayName"
	PropertyStatusID    = "statusId"
)

// Input is the tool's typed input.
type Input struct {
	AssetID    string      `json:"assetId" jsonschema:"Required. UUID of the asset to edit."`
	Operations []Operation `json:"operations" jsonschema:"Required. Non-empty list of operations to apply. Each operation's type selects which additional fields are used (see Operation)."`
}

// Operation is a discriminated union: the 'type' field selects which other
// fields are interpreted. Unused fields are ignored. Server-side validation
// catches missing or incompatible fields and returns a per-operation error.
type Operation struct {
	Type OperationType `json:"type" jsonschema:"Required. One of: update_attribute, add_attribute, remove_attribute, update_property, add_relation, remove_relation, add_tag, set_responsibility, remove_responsibility."`

	// Attribute ops — used by update_attribute, add_attribute, remove_attribute.
	AttributeName string `json:"attributeName,omitempty" jsonschema:"Attribute type name (e.g. 'Definition', 'Note'). Used by update_attribute, add_attribute, remove_attribute. The server resolves this to the attribute type UUID via the asset's scoped assignment."`
	Value         string `json:"value,omitempty" jsonschema:"New value. Used by update_attribute, add_attribute, and update_property."`

	// update_property — whitelisted fields only.
	Field string `json:"field,omitempty" jsonschema:"For update_property: one of 'name', 'displayName', 'statusId'. When field is 'statusId', value may be either the status UUID or the status name (e.g. 'Candidate', 'Accepted'); the server resolves names automatically. When field is 'name' and the asset's current displayName equals its current name (Collibra's create-time default), displayName is also updated to the new value so the user-facing label stays in sync — set field=displayName separately if the user has already customized it differently."`

	// Relation ops.
	RelationType  string `json:"relationType,omitempty" jsonschema:"For add_relation: the forward role name of the relation type (e.g. 'is synonym of'). The edited asset is assumed to be the source (head) of the relation; if the named relation type expects the opposite direction, Collibra will return an error."`
	TargetAssetID string `json:"targetAssetId,omitempty" jsonschema:"For add_relation: UUID of the asset on the target (tail) side of the relation."`
	RelationID    string `json:"relationId,omitempty" jsonschema:"For remove_relation: UUID of the relation instance to delete."`

	// Tag op — appends a tag to the asset (does not replace existing tags).
	Tag string `json:"tag,omitempty" jsonschema:"For add_tag: a free-text tag to append to the asset (e.g. 'finance'). Existing tags are preserved."`

	// Responsibility ops — set_responsibility and remove_responsibility.
	Role   string `json:"role,omitempty" jsonschema:"For set_responsibility / remove_responsibility: resource role name (e.g. 'Steward', 'Owner'). The server resolves this to the role UUID. remove_responsibility deletes only a responsibility defined directly on this asset (not one inherited from a parent domain or community)."`
	UserID string `json:"userId,omitempty" jsonschema:"For set_responsibility / remove_responsibility: identifies the user (or user group) the role is assigned to. Accepts a UUID, a username (e.g. 'jane.smith'), or an email address (e.g. 'jane@example.com'). Names are resolved server-side."`
}

// OutputStatus summarises the result of the call.
type OutputStatus string

const (
	StatusSuccess        OutputStatus = "success"
	StatusPartialSuccess OutputStatus = "partial_success"
	StatusError          OutputStatus = "error"
)

// Output is the tool's typed output.
type Output struct {
	Status  OutputStatus      `json:"status" jsonschema:"Overall status: success if every operation applied, partial_success if some succeeded and some failed, error if every operation failed or the request could not be executed."`
	Results []OperationResult `json:"results" jsonschema:"Per-operation outcomes, in the same order as the input operations."`
	Asset   *AssetSummary     `json:"asset,omitempty" jsonschema:"The asset's state after applying successful operations. Present on success or partial_success."`
	Error   string            `json:"error,omitempty" jsonschema:"Populated only when the overall request could not start (e.g. the asset was not found). Per-operation errors live in Results."`
}

// AssetSummary is the post-edit snapshot of the asset.
type AssetSummary struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	DisplayName string `json:"displayName,omitempty"`
	Type        string `json:"type"`
	Domain      string `json:"domain"`
	Status      string `json:"status,omitempty"`
}

// OperationResult is the outcome of a single operation in the input array.
type OperationResult struct {
	Operation           OperationType `json:"operation"`
	Status              string        `json:"status" jsonschema:"'success' or 'error'."`
	AttributeName       string        `json:"attributeName,omitempty"`
	Field               string        `json:"field,omitempty"`
	RelationType        string        `json:"relationType,omitempty"`
	RelationID          string        `json:"relationId,omitempty"`
	TargetAssetID       string        `json:"targetAssetId,omitempty"`
	Tag                 string        `json:"tag,omitempty"`
	Role                string        `json:"role,omitempty"`
	UserID              string        `json:"userId,omitempty"`
	PreviousValue       string        `json:"previousValue,omitempty"`
	NewValue            string        `json:"newValue,omitempty"`
	CascadedDisplayName bool          `json:"cascadedDisplayName,omitempty" jsonschema:"True when update_property field=name also updated displayName because the asset's previous displayName matched its previous name (Collibra's create-time default). Only set on update_property results."`
	Error               string        `json:"error,omitempty"`
}

// NewTool returns the registered tool.
func NewTool(collibraClient *http.Client) *chip.Tool[Input, Output] {
	return &chip.Tool[Input, Output]{
		Name:  "edit_asset",
		Title: "Edit Asset",
		Description: "Edit an existing Collibra asset by submitting a list of typed operations against a single assetId. " +
			"Supported operations: " +
			"update_attribute / add_attribute / remove_attribute (change, append, or clear an attribute value such as 'Definition' or 'Note', identified by attribute type name); " +
			"update_property (whitelisted fields only: 'name' to rename — also updates displayName when it tracks the current name, so the user-facing label stays in sync; 'displayName' to change the display name; or 'statusId' which accepts either a status UUID or a status name like 'Candidate'/'Accepted'); " +
			"add_relation / remove_relation (link or unlink the asset to another asset; add_relation takes a forward role name like 'is synonym of' plus the target assetId, remove_relation takes the relation instance UUID); " +
			"add_tag (append a free-text tag without replacing existing tags); " +
			"set_responsibility (assign a user or group to a resource role such as 'Steward' or 'Owner'; the user can be given as a UUID, username, or email); " +
			"remove_responsibility (unassign a user or group from a resource role given the same role and user; removes only a responsibility assigned directly on the asset, not one inherited from a parent domain or community). " +
			"Names (attribute names, relation roles, status names, resource role names, and user identifiers) are resolved server-side and matching is case- and whitespace-insensitive. " +
			"Each operation is validated against the asset's scoped assignment before any writes; invalid ops return per-operation errors while valid siblings still apply, yielding status=success, partial_success, or error. " +
			"On success the response includes a post-edit snapshot of the asset and per-operation before/after values.",
		Handler:     handler(collibraClient),
		Permissions: []string{},
		Annotations: &mcp.ToolAnnotations{DestructiveHint: chip.Ptr(true)},
	}
}

func handler(collibraClient *http.Client) chip.ToolHandlerFunc[Input, Output] {
	return func(ctx context.Context, input Input) (Output, error) {
		if err := validation.UUID("assetId", input.AssetID); err != nil {
			return Output{}, err
		}
		if len(input.Operations) == 0 {
			return Output{}, fmt.Errorf("operations must not be empty")
		}

		ec, err := newEditContext(ctx, collibraClient, input.AssetID, input.Operations)
		if err != nil {
			return Output{Status: StatusError, Error: err.Error()}, nil
		}

		// Two-phase execution: validate every op first, then run the ones that
		// passed. Per-op validation errors become per-op results (partial_success)
		// rather than failing the whole request. executeValidPlans groups
		// bulk-eligible ops (2+ of the same type) into single bulk requests
		// where Collibra supports them.
		plans := make([]opPlan, len(input.Operations))
		for i, op := range input.Operations {
			plans[i] = validateOperation(ec, op)
		}
		executeValidPlans(ctx, collibraClient, ec, plans)

		results := make([]OperationResult, len(plans))
		successes := 0
		for i, plan := range plans {
			results[i] = plan.result
			if plan.result.Status == "success" {
				successes++
			}
		}

		out := Output{Results: results}
		switch {
		case successes == len(plans):
			out.Status = StatusSuccess
		case successes == 0:
			out.Status = StatusError
		default:
			out.Status = StatusPartialSuccess
		}

		// Re-fetch the asset to return an authoritative post-edit snapshot. If
		// the re-fetch fails we still return the per-op results — don't mask a
		// partial success with a read error.
		if successes > 0 {
			if updated, err := clients.GetAssetCore(ctx, collibraClient, input.AssetID); err == nil {
				out.Asset = summariseAsset(updated)
			}
		} else {
			out.Asset = summariseAsset(ec.asset)
		}

		return out, nil
	}
}

// editContext holds the pre-fetched state that every operation consults.
type editContext struct {
	asset                *clients.EditAssetCore
	attributes           []clients.EditAssetAttributeInstance
	assignment           *clients.EditAssetAssignment
	attributeTypeByName  map[string]clients.EditAssetAssignmentAttributeType
	attributesByTypeName map[string][]clients.EditAssetAttributeInstance
	relationTypeByRole   map[string]clients.EditAssetAssignmentRelationType
	// relationTypeByCoRole indexes inverse (TARGET_TO_SOURCE) relation types
	// by their CoRole name so add_relation can author from the tail asset.
	relationTypeByCoRole map[string]clients.EditAssetAssignmentRelationType
	// roleByName is populated only when the request contains at least one
	// set_responsibility op, saving a GET on calls that don't need roles.
	roleByName map[string]clients.EditAssetRole
	// statusByName is populated only when the request contains an
	// update_property op with field=statusId, so plain attribute/relation
	// edits don't pay for a /statuses fetch.
	statusByName map[string]clients.EditAssetStatus
}

// newEditContext fetches the asset, its current attributes, and the scoped
// assignment in one go so per-operation validation can be cheap and consistent.
// Roles are fetched lazily — only when the request contains a set_responsibility op.
func newEditContext(ctx context.Context, client *http.Client, assetID string, ops []Operation) (*editContext, error) {
	asset, err := clients.GetAssetCore(ctx, client, assetID)
	if err != nil {
		return nil, err
	}

	attrs, err := clients.ListAttributesForAsset(ctx, client, assetID)
	if err != nil {
		return nil, fmt.Errorf("fetching current attributes: %w", err)
	}

	// Attributes come from the per-asset endpoint, which Collibra resolves
	// correctly for the asset's exact type+domain (fixes attributes being
	// dropped when an asset's domain type isn't in its type's assignment scope).
	effective, err := clients.GetEffectiveAssignmentForAsset(ctx, client, assetID)
	if err != nil {
		return nil, fmt.Errorf("fetching effective assignment: %w", err)
	}

	// Relations stay on the parent-chain walk: the per-asset endpoint omits some
	// inherited relation types for certain asset types, and the walk is what #74
	// shipped. It needs the asset's domain type to scope the lookup.
	domain, err := clients.GetDomainDetails(ctx, client, asset.Domain.ID)
	if err != nil {
		return nil, fmt.Errorf("fetching domain for relation assignment: %w", err)
	}
	var domainTypeID string
	if domain.Type != nil {
		domainTypeID = domain.Type.ID
	}
	relationAssignment, err := clients.GetAssignmentForAssetType(ctx, client, asset.Type.ID, domainTypeID)
	if err != nil {
		return nil, fmt.Errorf("fetching relation assignment: %w", err)
	}

	// Combine: attributes from the per-asset assignment, relations from the
	// type-chain assignment. Everything downstream reads this one struct.
	assignment := &clients.EditAssetAssignment{
		AssetType:      effective.AssetType,
		AttributeTypes: effective.AttributeTypes,
		RelationTypes:  relationAssignment.RelationTypes,
	}

	byName := make(map[string]clients.EditAssetAssignmentAttributeType, len(assignment.AttributeTypes))
	for _, at := range assignment.AttributeTypes {
		byName[normalize(at.Name)] = at
	}

	attrsByTypeName := make(map[string][]clients.EditAssetAttributeInstance)
	for _, a := range attrs {
		key := normalize(a.Type.Name)
		attrsByTypeName[key] = append(attrsByTypeName[key], a)
	}

	relationByRole := make(map[string]clients.EditAssetAssignmentRelationType)
	relationByCoRole := make(map[string]clients.EditAssetAssignmentRelationType)
	for _, rt := range assignment.RelationTypes {
		if rt.Reversed {
			if rt.CoRole != "" {
				relationByCoRole[normalize(rt.CoRole)] = rt
			}
		} else {
			if rt.Role != "" {
				relationByRole[normalize(rt.Role)] = rt
			}
		}
	}

	var rolesByName map[string]clients.EditAssetRole
	if opsNeedRoles(ops) {
		roles, err := clients.ListRoles(ctx, client)
		if err != nil {
			return nil, fmt.Errorf("fetching roles: %w", err)
		}
		rolesByName = make(map[string]clients.EditAssetRole, len(roles))
		for _, r := range roles {
			rolesByName[normalize(r.Name)] = r
		}
	}

	var statusesByName map[string]clients.EditAssetStatus
	if opsNeedStatuses(ops) {
		statuses, err := clients.ListStatuses(ctx, client)
		if err != nil {
			return nil, fmt.Errorf("fetching statuses: %w", err)
		}
		statusesByName = make(map[string]clients.EditAssetStatus, len(statuses))
		for _, s := range statuses {
			statusesByName[normalize(s.Name)] = s
		}
	}

	return &editContext{
		asset:                asset,
		attributes:           attrs,
		assignment:           assignment,
		attributeTypeByName:  byName,
		attributesByTypeName: attrsByTypeName,
		relationTypeByRole:   relationByRole,
		relationTypeByCoRole: relationByCoRole,
		roleByName:           rolesByName,
		statusByName:         statusesByName,
	}, nil
}

// availableAttributeNames returns the original (un-normalized) attribute
// names from the assignment, for inclusion in error suggestions.
func (ec *editContext) availableAttributeNames() []string {
	names := make([]string, 0, len(ec.assignment.AttributeTypes))
	for _, at := range ec.assignment.AttributeTypes {
		names = append(names, at.Name)
	}
	return names
}

// availableRelationRoles returns all usable role names (forward and inverse)
// for inclusion in error suggestions, deduped — distinct relation types can
// share a role name (e.g. "impacted by" toward different target types).
func (ec *editContext) availableRelationRoles() []string {
	seen := make(map[string]struct{}, len(ec.assignment.RelationTypes))
	names := make([]string, 0, len(ec.assignment.RelationTypes))
	for _, rt := range ec.assignment.RelationTypes {
		name := rt.Role
		if rt.Reversed {
			name = rt.CoRole
		}
		if name == "" {
			continue
		}
		if _, dup := seen[name]; dup {
			continue
		}
		seen[name] = struct{}{}
		names = append(names, name)
	}
	return names
}

// availableRoleNames returns role names from the resolved roles map.
func (ec *editContext) availableRoleNames() []string {
	names := make([]string, 0, len(ec.roleByName))
	for _, r := range ec.roleByName {
		names = append(names, r.Name)
	}
	return names
}

// availableStatusNames returns status names from the resolved statuses map.
func (ec *editContext) availableStatusNames() []string {
	names := make([]string, 0, len(ec.statusByName))
	for _, s := range ec.statusByName {
		names = append(names, s.Name)
	}
	return names
}

// opsNeedRoles reports whether the request contains a responsibility op, so
// newEditContext can skip the roles fetch otherwise.
func opsNeedRoles(ops []Operation) bool {
	for _, op := range ops {
		if op.Type == OpSetResponsibility || op.Type == OpRemoveResponsibility {
			return true
		}
	}
	return false
}

// opsNeedStatuses reports whether the request contains an update_property op
// targeting statusId, so newEditContext can skip the statuses fetch otherwise.
func opsNeedStatuses(ops []Operation) bool {
	for _, op := range ops {
		if op.Type == OpUpdateProperty && op.Field == PropertyStatusID {
			return true
		}
	}
	return false
}

// opPlan is the result of validating an operation — it carries enough state to
// execute the op or, if validation failed, a populated error result.
type opPlan struct {
	op     Operation
	result OperationResult

	// Attribute ops (resolved during validation)
	attributeTypeID   string
	targetAttributeID string
	previousValue     string

	// Property op (resolved during validation)
	propertyPatch clients.EditAssetPatchRequest

	// Relation ops (resolved during validation)
	relationTypeID   string
	relationReversed bool // true when add_relation matched a CoRole; flip source/target on execute

	// Responsibility op (resolved during validation)
	roleID string
}

func newErrorResult(op Operation, msg string) OperationResult {
	return OperationResult{
		Operation:     op.Type,
		Status:        "error",
		AttributeName: op.AttributeName,
		Field:         op.Field,
		RelationType:  op.RelationType,
		RelationID:    op.RelationID,
		TargetAssetID: op.TargetAssetID,
		Tag:           op.Tag,
		Role:          op.Role,
		UserID:        op.UserID,
		Error:         msg,
	}
}

func newSuccessResult(op Operation) OperationResult {
	return OperationResult{
		Operation:     op.Type,
		Status:        "success",
		AttributeName: op.AttributeName,
		Field:         op.Field,
		RelationType:  op.RelationType,
		RelationID:    op.RelationID,
		TargetAssetID: op.TargetAssetID,
		Tag:           op.Tag,
		Role:          op.Role,
		UserID:        op.UserID,
	}
}

// validateOperation does all the checks that don't require a write and records
// resolved IDs on the plan so execution doesn't need to re-check them.
func validateOperation(ec *editContext, op Operation) opPlan {
	plan := opPlan{op: op}
	switch op.Type {
	case OpUpdateAttribute:
		return validateUpdateAttribute(ec, plan)
	case OpAddAttribute:
		return validateAddAttribute(ec, plan)
	case OpRemoveAttribute:
		return validateRemoveAttribute(ec, plan)
	case OpUpdateProperty:
		return validateUpdateProperty(ec, plan)
	case OpAddRelation:
		return validateAddRelation(ec, plan)
	case OpRemoveRelation:
		return validateRemoveRelation(plan)
	case OpAddTag:
		return validateAddTag(plan)
	case OpSetResponsibility, OpRemoveResponsibility:
		return validateResponsibilityOp(ec, plan)
	default:
		plan.result = newErrorResult(op, fmt.Sprintf("unsupported operation type %q", op.Type))
		return plan
	}
}

// executePlan runs the side effect for a validated plan and records the final
// result (previous/new values on success, error message on failure).
func executePlan(ctx context.Context, client *http.Client, ec *editContext, plan opPlan) opPlan {
	switch plan.op.Type {
	case OpUpdateAttribute:
		return executeUpdateAttribute(ctx, client, plan)
	case OpAddAttribute:
		return executeAddAttribute(ctx, client, ec, plan)
	case OpRemoveAttribute:
		return executeRemoveAttribute(ctx, client, ec, plan)
	case OpUpdateProperty:
		return executeUpdateProperty(ctx, client, ec, plan)
	case OpAddRelation:
		return executeAddRelation(ctx, client, ec, plan)
	case OpRemoveRelation:
		return executeRemoveRelation(ctx, client, plan)
	case OpAddTag:
		return executeAddTag(ctx, client, ec, plan)
	case OpSetResponsibility:
		return executeSetResponsibility(ctx, client, ec, plan)
	case OpRemoveResponsibility:
		return executeRemoveResponsibility(ctx, client, ec, plan)
	default:
		plan.result = newErrorResult(plan.op, fmt.Sprintf("unsupported operation type %q", plan.op.Type))
		return plan
	}
}

func summariseAsset(a *clients.EditAssetCore) *AssetSummary {
	if a == nil {
		return nil
	}
	s := &AssetSummary{
		ID:          a.ID,
		Name:        a.Name,
		DisplayName: a.DisplayName,
		Type:        a.Type.Name,
		Domain:      a.Domain.Name,
	}
	if a.Status != nil {
		s.Status = a.Status.Name
	}
	return s
}
