package edit_asset

import (
	"context"
	"net/http"

	"github.com/collibra/chip/pkg/clients"
)

// bulkThreshold is the minimum number of same-type ops that triggers a bulk
// endpoint call instead of per-op individual requests. Keeping this at 2 means
// a single add/update/relation still uses the cheap individual endpoint and
// 2+ amortize a single round trip.
const bulkThreshold = 2

// executeValidPlans runs every plan whose validation step succeeded. It groups
// bulk-eligible ops by type and fires a single bulk request per group; any
// remaining ops (or groups below bulkThreshold) fall through to individual
// execution via executePlan. Order of results is preserved via in-place
// updates to plans.
func executeValidPlans(ctx context.Context, client *http.Client, ec *editContext, plans []opPlan) {
	// Collect indices of valid plans grouped by bulk-eligible op type.
	var addAttrIdx, updAttrIdx, addRelIdx []int
	for i, p := range plans {
		if p.result.Status == "error" {
			continue
		}
		switch p.op.Type {
		case OpAddAttribute:
			addAttrIdx = append(addAttrIdx, i)
		case OpUpdateAttribute:
			updAttrIdx = append(updAttrIdx, i)
		case OpAddRelation:
			addRelIdx = append(addRelIdx, i)
		}
	}

	// Bulk-eligible groups: dispatch as bulk if at-or-above threshold.
	bulked := map[int]bool{}
	if len(addAttrIdx) >= bulkThreshold {
		executeBulkAddAttributes(ctx, client, ec, plans, addAttrIdx)
		for _, i := range addAttrIdx {
			bulked[i] = true
		}
	}
	if len(updAttrIdx) >= bulkThreshold {
		executeBulkUpdateAttributes(ctx, client, plans, updAttrIdx)
		for _, i := range updAttrIdx {
			bulked[i] = true
		}
	}
	if len(addRelIdx) >= bulkThreshold {
		executeBulkAddRelations(ctx, client, ec, plans, addRelIdx)
		for _, i := range addRelIdx {
			bulked[i] = true
		}
	}

	// Everything not bulked runs through the per-op executor.
	for i := range plans {
		if plans[i].result.Status == "error" || bulked[i] {
			continue
		}
		plans[i] = executePlan(ctx, client, ec, plans[i])
	}
}

// executeBulkAddAttributes issues POST /rest/2.0/attributes/bulk for every
// add_attribute plan in indices. On batch failure every op in the batch gets
// the same error message.
func executeBulkAddAttributes(ctx context.Context, client *http.Client, ec *editContext, plans []opPlan, indices []int) {
	reqs := make([]clients.CreateAttributeRequest, len(indices))
	for j, idx := range indices {
		reqs[j] = clients.CreateAttributeRequest{
			AssetID: ec.asset.ID,
			TypeID:  plans[idx].attributeTypeID,
			Value:   plans[idx].op.Value,
		}
	}

	created, err := clients.BulkCreateAttributes(ctx, client, reqs)
	if err != nil {
		for _, idx := range indices {
			plans[idx].result = newErrorResult(plans[idx].op, err.Error())
		}
		return
	}
	// Collibra's bulk endpoint returns results in input order.
	for j, idx := range indices {
		res := newSuccessResult(plans[idx].op)
		if j < len(created) {
			res.NewValue = created[j].Value
		} else {
			res.NewValue = plans[idx].op.Value
		}
		plans[idx].result = res
	}
}

// executeBulkUpdateAttributes issues PATCH /rest/2.0/attributes/bulk for every
// update_attribute plan in indices.
func executeBulkUpdateAttributes(ctx context.Context, client *http.Client, plans []opPlan, indices []int) {
	reqs := make([]clients.EditAssetBulkPatchAttributeItem, len(indices))
	for j, idx := range indices {
		reqs[j] = clients.EditAssetBulkPatchAttributeItem{
			ID:    plans[idx].targetAttributeID,
			Value: plans[idx].op.Value,
		}
	}

	updated, err := clients.BulkPatchAttributes(ctx, client, reqs)
	if err != nil {
		for _, idx := range indices {
			plans[idx].result = newErrorResult(plans[idx].op, err.Error())
		}
		return
	}
	for j, idx := range indices {
		res := OperationResult{
			Operation:     plans[idx].op.Type,
			Status:        "success",
			AttributeName: plans[idx].op.AttributeName,
			PreviousValue: plans[idx].previousValue,
		}
		if j < len(updated) {
			res.NewValue = updated[j].Value
		} else {
			res.NewValue = plans[idx].op.Value
		}
		plans[idx].result = res
	}
}

// executeBulkAddRelations issues POST /rest/2.0/relations/bulk.
func executeBulkAddRelations(ctx context.Context, client *http.Client, ec *editContext, plans []opPlan, indices []int) {
	reqs := make([]clients.EditAssetCreateRelationRequest, len(indices))
	for j, idx := range indices {
		reqs[j] = clients.EditAssetCreateRelationRequest{
			SourceID: ec.asset.ID,
			TargetID: plans[idx].op.TargetAssetID,
			TypeID:   plans[idx].relationTypeID,
		}
	}

	created, err := clients.BulkCreateRelations(ctx, client, reqs)
	if err != nil {
		for _, idx := range indices {
			plans[idx].result = newErrorResult(plans[idx].op, err.Error())
		}
		return
	}
	for j, idx := range indices {
		res := newSuccessResult(plans[idx].op)
		if j < len(created) {
			res.RelationID = created[j].ID
		}
		plans[idx].result = res
	}
}
