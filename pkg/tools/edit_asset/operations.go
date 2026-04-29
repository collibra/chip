package edit_asset

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/collibra/chip/pkg/clients"
	"github.com/collibra/chip/pkg/tools/validation"
	"github.com/google/uuid"
)

// --- update_attribute ---------------------------------------------------------

func validateUpdateAttribute(ec *editContext, plan opPlan) opPlan {
	op := plan.op
	if strings.TrimSpace(op.AttributeName) == "" {
		plan.result = newErrorResult(op, "attributeName is required for update_attribute")
		return plan
	}
	key := normalize(op.AttributeName)
	attrType, ok := ec.attributeTypeByName[key]
	if !ok {
		plan.result = newErrorResult(op, fmt.Sprintf(
			"attribute %q is not valid for asset type %q in this domain.%s",
			op.AttributeName, ec.asset.Type.Name,
			suggestionSuffix("Attributes", ec.availableAttributeNames(), 10)))
		return plan
	}
	instances := ec.attributesByTypeName[key]
	switch len(instances) {
	case 0:
		plan.result = newErrorResult(op, fmt.Sprintf("no existing %q attribute to update on this asset (use add_attribute)", op.AttributeName))
		return plan
	case 1:
		plan.targetAttributeID = instances[0].ID
		plan.previousValue = instances[0].Value
	default:
		plan.result = newErrorResult(op, fmt.Sprintf("%d %q attributes on this asset — cannot disambiguate by name alone", len(instances), op.AttributeName))
		return plan
	}
	if err := validateAttributeValue(attrType, op.Value); err != nil {
		plan.result = newErrorResult(op, err.Error())
		return plan
	}
	plan.result = newSuccessResult(op)
	return plan
}

func executeUpdateAttribute(ctx context.Context, client *http.Client, plan opPlan) opPlan {
	updated, err := clients.PatchAttributeValue(ctx, client, plan.targetAttributeID, plan.op.Value)
	if err != nil {
		plan.result = newErrorResult(plan.op, err.Error())
		return plan
	}
	plan.result = OperationResult{
		Operation:     plan.op.Type,
		Status:        "success",
		AttributeName: plan.op.AttributeName,
		PreviousValue: plan.previousValue,
		NewValue:      updated.Value,
	}
	return plan
}

// --- add_attribute ------------------------------------------------------------

func validateAddAttribute(ec *editContext, plan opPlan) opPlan {
	op := plan.op
	if strings.TrimSpace(op.AttributeName) == "" {
		plan.result = newErrorResult(op, "attributeName is required for add_attribute")
		return plan
	}
	attrType, ok := ec.attributeTypeByName[normalize(op.AttributeName)]
	if !ok {
		plan.result = newErrorResult(op, fmt.Sprintf(
			"attribute %q is not valid for asset type %q in this domain.%s",
			op.AttributeName, ec.asset.Type.Name,
			suggestionSuffix("Attributes", ec.availableAttributeNames(), 10)))
		return plan
	}
	if err := validateAttributeValue(attrType, op.Value); err != nil {
		plan.result = newErrorResult(op, err.Error())
		return plan
	}
	plan.attributeTypeID = attrType.ID
	plan.result = newSuccessResult(op)
	return plan
}

func executeAddAttribute(ctx context.Context, client *http.Client, ec *editContext, plan opPlan) opPlan {
	created, err := clients.CreateAttributeOnAsset(ctx, client, ec.asset.ID, plan.attributeTypeID, plan.op.Value)
	if err != nil {
		plan.result = newErrorResult(plan.op, err.Error())
		return plan
	}
	plan.result = OperationResult{
		Operation:     plan.op.Type,
		Status:        "success",
		AttributeName: plan.op.AttributeName,
		NewValue:      created.Value,
	}
	return plan
}

// --- remove_attribute ---------------------------------------------------------

func validateRemoveAttribute(ec *editContext, plan opPlan) opPlan {
	op := plan.op
	if strings.TrimSpace(op.AttributeName) == "" {
		plan.result = newErrorResult(op, "attributeName is required for remove_attribute")
		return plan
	}
	instances := ec.attributesByTypeName[normalize(op.AttributeName)]
	switch len(instances) {
	case 0:
		plan.result = newErrorResult(op, fmt.Sprintf("no %q attribute to remove on this asset", op.AttributeName))
		return plan
	case 1:
		plan.targetAttributeID = instances[0].ID
		plan.previousValue = instances[0].Value
	default:
		plan.result = newErrorResult(op, fmt.Sprintf("%d %q attributes on this asset — cannot disambiguate by name alone", len(instances), op.AttributeName))
		return plan
	}
	plan.result = newSuccessResult(op)
	return plan
}

func executeRemoveAttribute(ctx context.Context, client *http.Client, _ *editContext, plan opPlan) opPlan {
	if err := clients.DeleteAttribute(ctx, client, plan.targetAttributeID); err != nil {
		plan.result = newErrorResult(plan.op, err.Error())
		return plan
	}
	plan.result = OperationResult{
		Operation:     plan.op.Type,
		Status:        "success",
		AttributeName: plan.op.AttributeName,
		PreviousValue: plan.previousValue,
	}
	return plan
}

// --- update_property ----------------------------------------------------------

func validateUpdateProperty(ec *editContext, plan opPlan) opPlan {
	op := plan.op
	switch op.Field {
	case PropertyName:
		if strings.TrimSpace(op.Value) == "" {
			plan.result = newErrorResult(op, "value is required for update_property name")
			return plan
		}
		v := op.Value
		plan.propertyPatch = clients.EditAssetPatchRequest{Name: &v}
	case PropertyDisplayName:
		v := op.Value
		plan.propertyPatch = clients.EditAssetPatchRequest{DisplayName: &v}
	case PropertyStatusID:
		if strings.TrimSpace(op.Value) == "" {
			plan.result = newErrorResult(op, "value is required for update_property statusId")
			return plan
		}
		// Accept either a UUID or a human-friendly status name. If the value
		// isn't a UUID, look it up by name in the pre-fetched statuses map.
		statusID := op.Value
		if _, parseErr := uuid.Parse(op.Value); parseErr != nil {
			st, ok := ec.statusByName[normalize(op.Value)]
			if !ok {
				plan.result = newErrorResult(op, fmt.Sprintf(
					"status %q is not defined in Collibra.%s",
					op.Value, suggestionSuffix("Statuses", ec.availableStatusNames(), 10)))
				return plan
			}
			statusID = st.ID
		}
		plan.propertyPatch = clients.EditAssetPatchRequest{StatusID: &statusID}
	case "":
		plan.result = newErrorResult(op, "field is required for update_property (one of: name, displayName, statusId)")
		return plan
	default:
		plan.result = newErrorResult(op, fmt.Sprintf("field %q is not supported for update_property; allowed: name, displayName, statusId", op.Field))
		return plan
	}
	plan.result = newSuccessResult(op)
	return plan
}

func executeUpdateProperty(ctx context.Context, client *http.Client, ec *editContext, plan opPlan) opPlan {
	updated, err := clients.PatchAsset(ctx, client, ec.asset.ID, plan.propertyPatch)
	if err != nil {
		plan.result = newErrorResult(plan.op, err.Error())
		return plan
	}

	prev, next := previousAndNewProperty(ec.asset, updated, plan.op.Field)
	plan.result = OperationResult{
		Operation:     plan.op.Type,
		Status:        "success",
		Field:         plan.op.Field,
		PreviousValue: prev,
		NewValue:      next,
	}
	// Keep our in-memory snapshot current for subsequent ops in the same request.
	ec.asset = updated
	return plan
}

func previousAndNewProperty(before, after *clients.EditAssetCore, field string) (string, string) {
	switch field {
	case PropertyName:
		return before.Name, after.Name
	case PropertyDisplayName:
		return before.DisplayName, after.DisplayName
	case PropertyStatusID:
		var prev, next string
		if before.Status != nil {
			prev = before.Status.ID
		}
		if after.Status != nil {
			next = after.Status.ID
		}
		return prev, next
	}
	return "", ""
}

// --- add_relation -------------------------------------------------------------

func validateAddRelation(ec *editContext, plan opPlan) opPlan {
	op := plan.op
	if strings.TrimSpace(op.RelationType) == "" {
		plan.result = newErrorResult(op, "relationType is required for add_relation")
		return plan
	}
	if err := validation.UUID("targetAssetId", op.TargetAssetID); err != nil {
		plan.result = newErrorResult(op, err.Error())
		return plan
	}
	rt, ok := ec.relationTypeByRole[normalize(op.RelationType)]
	if !ok {
		plan.result = newErrorResult(op, fmt.Sprintf(
			"relation type %q is not valid for asset type %q in this domain (edited asset must be the source/head; try the forward role name).%s",
			op.RelationType, ec.asset.Type.Name,
			suggestionSuffix("Relation roles", ec.availableRelationRoles(), 10)))
		return plan
	}
	plan.relationTypeID = rt.ID
	plan.result = newSuccessResult(op)
	return plan
}

func executeAddRelation(ctx context.Context, client *http.Client, ec *editContext, plan opPlan) opPlan {
	created, err := clients.CreateRelation(ctx, client, clients.EditAssetCreateRelationRequest{
		SourceID: ec.asset.ID,
		TargetID: plan.op.TargetAssetID,
		TypeID:   plan.relationTypeID,
	})
	if err != nil {
		plan.result = newErrorResult(plan.op, err.Error())
		return plan
	}
	res := newSuccessResult(plan.op)
	res.RelationID = created.ID
	plan.result = res
	return plan
}

// --- remove_relation ----------------------------------------------------------

func validateRemoveRelation(plan opPlan) opPlan {
	op := plan.op
	if err := validation.UUID("relationId", op.RelationID); err != nil {
		plan.result = newErrorResult(op, err.Error())
		return plan
	}
	plan.result = newSuccessResult(op)
	return plan
}

func executeRemoveRelation(ctx context.Context, client *http.Client, plan opPlan) opPlan {
	if err := clients.DeleteRelation(ctx, client, plan.op.RelationID); err != nil {
		plan.result = newErrorResult(plan.op, err.Error())
		return plan
	}
	plan.result = newSuccessResult(plan.op)
	return plan
}

// --- add_tag ------------------------------------------------------------------

func validateAddTag(plan opPlan) opPlan {
	op := plan.op
	if strings.TrimSpace(op.Tag) == "" {
		plan.result = newErrorResult(op, "tag is required for add_tag")
		return plan
	}
	plan.result = newSuccessResult(op)
	return plan
}

func executeAddTag(ctx context.Context, client *http.Client, ec *editContext, plan opPlan) opPlan {
	if err := clients.AddTagsToAsset(ctx, client, ec.asset.ID, []string{plan.op.Tag}); err != nil {
		plan.result = newErrorResult(plan.op, err.Error())
		return plan
	}
	res := newSuccessResult(plan.op)
	res.NewValue = plan.op.Tag
	plan.result = res
	return plan
}

// --- set_responsibility -------------------------------------------------------

func validateSetResponsibility(ec *editContext, plan opPlan) opPlan {
	op := plan.op
	if strings.TrimSpace(op.Role) == "" {
		plan.result = newErrorResult(op, "role is required for set_responsibility")
		return plan
	}
	if strings.TrimSpace(op.UserID) == "" {
		plan.result = newErrorResult(op, "userId is required for set_responsibility (UUID, username, or email)")
		return plan
	}
	role, ok := ec.roleByName[normalize(op.Role)]
	if !ok {
		plan.result = newErrorResult(op, fmt.Sprintf(
			"role %q is not defined in Collibra.%s",
			op.Role, suggestionSuffix("Roles", ec.availableRoleNames(), 10)))
		return plan
	}
	plan.roleID = role.ID
	plan.result = newSuccessResult(op)
	return plan
}

func executeSetResponsibility(ctx context.Context, client *http.Client, ec *editContext, plan opPlan) opPlan {
	// Resolve userId at execution time. UUIDs pass through; emails go to
	// the email lookup; anything else is treated as a username. Failures
	// surface as per-op errors.
	ownerID := plan.op.UserID
	if _, parseErr := uuid.Parse(plan.op.UserID); parseErr != nil {
		var (
			user *clients.EditAssetUser
			err  error
		)
		if strings.Contains(plan.op.UserID, "@") {
			user, err = clients.FindUserByEmail(ctx, client, plan.op.UserID)
		} else {
			user, err = clients.FindUserByUsername(ctx, client, plan.op.UserID)
		}
		if err != nil {
			plan.result = newErrorResult(plan.op, fmt.Sprintf("resolving user %q: %s", plan.op.UserID, err.Error()))
			return plan
		}
		if user == nil {
			plan.result = newErrorResult(plan.op, fmt.Sprintf("no user found matching %q (try the user's username, email, or UUID)", plan.op.UserID))
			return plan
		}
		ownerID = user.ID
	}

	created, err := clients.CreateResponsibility(ctx, client, clients.EditAssetCreateResponsibilityRequest{
		RoleID:     plan.roleID,
		OwnerID:    ownerID,
		ResourceID: ec.asset.ID,
	})
	if err != nil {
		plan.result = newErrorResult(plan.op, err.Error())
		return plan
	}
	res := newSuccessResult(plan.op)
	res.NewValue = created.ID
	plan.result = res
	return plan
}

// --- shared helpers -----------------------------------------------------------

func validateAttributeValue(attrType clients.EditAssetAssignmentAttributeType, value string) error {
	c := attrType.Constraints
	if c == nil {
		return nil
	}
	if c.MinLength != nil && len(value) < *c.MinLength {
		return fmt.Errorf("value for %q is shorter than minimum length %d", attrType.Name, *c.MinLength)
	}
	if c.MaxLength != nil && len(value) > *c.MaxLength {
		return fmt.Errorf("value for %q exceeds maximum length %d", attrType.Name, *c.MaxLength)
	}
	return nil
}
