// Package edit_community implements the edit_community MCP tool: a single
// entry point for renaming a community, changing its description, moving it
// under another parent (or to the top level), and managing its role
// assignments (responsibilities) via a typed list of operations. Each
// operation is validated independently and applied on its own, so one failing
// operation never blocks its valid siblings.
package edit_community

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/collibra/chip/pkg/chip"
	"github.com/collibra/chip/pkg/clients"
	"github.com/collibra/chip/pkg/tools/resolve"
	"github.com/collibra/chip/pkg/tools/responsibility"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// OperationType enumerates the edits edit_community can perform.
type OperationType string

const (
	OpSetName              OperationType = "set_name"
	OpSetDescription       OperationType = "set_description"
	OpSetParent            OperationType = "set_parent"
	OpSetResponsibility    OperationType = "set_responsibility"
	OpRemoveResponsibility OperationType = "remove_responsibility"
)

// Input is the tool's typed input.
type Input struct {
	Community  string      `json:"community" jsonschema:"Required. The community to edit — its UUID or its name (an exact, case-insensitive match; an ambiguous name returns the candidates with their UUIDs)."`
	Operations []Operation `json:"operations" jsonschema:"Required. Non-empty list of operations to apply. Each operation is validated and applied independently. Each operation's type selects which additional fields are used (see Operation)."`
}

// Operation is a discriminated union: the 'type' field selects which other
// fields are interpreted. Unused fields are ignored.
type Operation struct {
	Type OperationType `json:"type" jsonschema:"Required. One of: set_name, set_description, set_parent, set_responsibility, remove_responsibility."`

	Value string `json:"value,omitempty" jsonschema:"For set_name: the new community name (must not be empty). For set_description: the new description (an empty string clears it)."`

	Parent       string `json:"parent,omitempty" jsonschema:"For set_parent: the community to move this one under, given as its UUID or name. Provide exactly one of parent or makeTopLevel."`
	MakeTopLevel bool   `json:"makeTopLevel,omitempty" jsonschema:"For set_parent: true to remove the parent and make the community top-level. Provide exactly one of parent or makeTopLevel."`

	Role   string `json:"role,omitempty" jsonschema:"For set_responsibility and remove_responsibility: the resource role name, e.g. 'Steward' or 'Owner'."`
	UserID string `json:"userId,omitempty" jsonschema:"For set_responsibility and remove_responsibility: the user or group, given as a UUID, email address, username, or full name such as 'Jane Smith'. A full name shared by several users returns a per-operation error listing the candidates."`
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
	Status    OutputStatus      `json:"status" jsonschema:"Overall status: success if every operation applied, partial_success if some succeeded and some failed, error if every operation failed."`
	Results   []OperationResult `json:"results" jsonschema:"Per-operation outcomes, in the same order as the input operations."`
	Community *CommunitySummary `json:"community,omitempty" jsonschema:"The community's state after the edits. Present when the community could be re-read."`
}

// CommunitySummary is the post-edit snapshot of the community.
type CommunitySummary struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	ParentID    string `json:"parentId,omitempty"`
	Parent      string `json:"parent,omitempty"`
}

// OperationResult is the outcome of a single operation.
type OperationResult struct {
	Operation     OperationType `json:"operation"`
	Status        string        `json:"status" jsonschema:"'success' or 'error'."`
	PreviousValue string        `json:"previousValue,omitempty"`
	NewValue      string        `json:"newValue,omitempty"`
	Error         string        `json:"error,omitempty"`
}

// NewTool returns the registered tool.
func NewTool(collibraClient *http.Client) *chip.Tool[Input, Output] {
	return &chip.Tool[Input, Output]{
		Name:  "edit_community",
		Title: "Edit Community",
		Description: "Edit an existing Collibra community (identified by UUID or name) by submitting a list of typed operations. " +
			"Supported operations: " +
			"set_name (rename the community); " +
			"set_description (change the description; an empty value clears it); " +
			"set_parent (move the community under a different parent community given as a UUID or name via 'parent', or pass makeTopLevel=true to remove the parent — exactly one of the two is required, neither is a validation error); " +
			"set_responsibility (assign a user or group to a resource role such as 'Steward' or 'Owner'; the user can be a UUID, username, email address, or full name such as 'Jane Smith'); " +
			"remove_responsibility (unassign a user or group from a role; removes only a responsibility assigned directly on the community, not one inherited from a parent). " +
			"Each operation is validated independently and applied on its own: invalid or failing operations return per-operation errors while valid siblings still apply, yielding status=success, partial_success, or error. " +
			"set_parent verifies the result: if Collibra reports success but the parent did not change, the operation is reported as an error. " +
			"On completion the response includes the community's state after the edits.",
		Handler:     handler(collibraClient),
		Permissions: []string{},
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: false, DestructiveHint: chip.Ptr(true), IdempotentHint: false, OpenWorldHint: chip.Ptr(false)},
	}
}

func handler(collibraClient *http.Client) chip.ToolHandlerFunc[Input, Output] {
	return func(ctx context.Context, input Input) (Output, error) {
		if len(input.Operations) == 0 {
			return Output{}, fmt.Errorf("operations must not be empty")
		}
		community, err := resolve.Community(ctx, collibraClient, input.Community, resolve.Hints{})
		if err != nil {
			return Output{}, err
		}

		// Validate every operation before any write, then apply the valid ones.
		results := make([]OperationResult, len(input.Operations))
		for i, op := range input.Operations {
			if verr := validateOperation(op); verr != nil {
				results[i] = errorResult(op, verr)
				continue
			}
			results[i] = executeOperation(ctx, collibraClient, community, op)
		}

		out := Output{Status: overallStatus(results), Results: results}
		out.Community = readCommunity(ctx, collibraClient, community.ID)
		return out, nil
	}
}

// validateOperation runs the checks that need no request.
func validateOperation(op Operation) error {
	switch op.Type {
	case OpSetName:
		if strings.TrimSpace(op.Value) == "" {
			return fmt.Errorf("value is required for set_name and must not be empty")
		}
	case OpSetDescription:
		// An empty value is a valid way to clear the description.
	case OpSetParent:
		hasParent := strings.TrimSpace(op.Parent) != ""
		if hasParent == op.MakeTopLevel {
			return fmt.Errorf("set_parent requires exactly one of parent (a community UUID or name) or makeTopLevel=true")
		}
	case OpSetResponsibility, OpRemoveResponsibility:
		return responsibility.Check(op.Role, op.UserID)
	default:
		return fmt.Errorf("unknown operation type %q; expected one of set_name, set_description, set_parent, set_responsibility, remove_responsibility", op.Type)
	}
	return nil
}

func executeOperation(ctx context.Context, client *http.Client, community clients.CommunityDetails, op Operation) OperationResult {
	switch op.Type {
	case OpSetName:
		return patchField(ctx, client, community, op, community.Name, op.Value, clients.CommunityPatch{Name: chip.Ptr(op.Value)})
	case OpSetDescription:
		return patchField(ctx, client, community, op, community.Description, op.Value, clients.CommunityPatch{Description: chip.Ptr(op.Value)})
	case OpSetParent:
		return executeSetParent(ctx, client, community, op)
	case OpSetResponsibility:
		return executeSetResponsibility(ctx, client, community, op)
	default:
		return executeRemoveResponsibility(ctx, client, community, op)
	}
}

func patchField(ctx context.Context, client *http.Client, community clients.CommunityDetails, op Operation, previous, next string, patch clients.CommunityPatch) OperationResult {
	if _, err := clients.PatchCommunity(ctx, client, community.ID, patch); err != nil {
		return errorResult(op, err)
	}
	return OperationResult{Operation: op.Type, Status: "success", PreviousValue: previous, NewValue: next}
}

func executeSetParent(ctx context.Context, client *http.Client, community clients.CommunityDetails, op Operation) OperationResult {
	previous := parentName(community.Parent)

	if op.MakeTopLevel {
		updated, err := clients.PatchCommunity(ctx, client, community.ID, clients.CommunityPatch{ClearParent: true})
		if err != nil {
			return errorResult(op, err)
		}
		// Trust the response, not the status code: Collibra has answered 200
		// to a clear-parent request without clearing it (DEV-217539).
		if updated.Parent != nil {
			return errorResult(op, fmt.Errorf("the request succeeded but the community is still under parent %q; the platform did not clear the parent", updated.Parent.Name))
		}
		return OperationResult{Operation: op.Type, Status: "success", PreviousValue: previous, NewValue: "(top-level)"}
	}

	parent, err := resolve.Community(ctx, client, op.Parent, resolve.Hints{})
	if err != nil {
		return errorResult(op, err)
	}
	if parent.ID == community.ID {
		return errorResult(op, fmt.Errorf("a community cannot be its own parent"))
	}
	updated, err := clients.PatchCommunity(ctx, client, community.ID, clients.CommunityPatch{ParentID: chip.Ptr(parent.ID)})
	if err != nil {
		return errorResult(op, err)
	}
	if updated.Parent == nil || updated.Parent.ID != parent.ID {
		return errorResult(op, fmt.Errorf("the request succeeded but the community is not under parent %q afterwards", parent.Name))
	}
	return OperationResult{Operation: op.Type, Status: "success", PreviousValue: previous, NewValue: parent.Name}
}

func executeSetResponsibility(ctx context.Context, client *http.Client, community clients.CommunityDetails, op Operation) OperationResult {
	id, err := responsibility.Set(ctx, client, target(community), op.Role, op.UserID)
	if err != nil {
		return errorResult(op, err)
	}
	return OperationResult{Operation: op.Type, Status: "success", NewValue: id}
}

func executeRemoveResponsibility(ctx context.Context, client *http.Client, community clients.CommunityDetails, op Operation) OperationResult {
	id, err := responsibility.Remove(ctx, client, target(community), op.Role, op.UserID)
	if err != nil {
		return errorResult(op, err)
	}
	return OperationResult{Operation: op.Type, Status: "success", PreviousValue: id}
}

func target(community clients.CommunityDetails) responsibility.Target {
	return responsibility.Target{Type: responsibility.ResourceCommunity, ID: community.ID}
}

func parentName(parent *clients.ContainerRef) string {
	if parent == nil {
		return "(top-level)"
	}
	return parent.Name
}

func errorResult(op Operation, err error) OperationResult {
	return OperationResult{Operation: op.Type, Status: "error", Error: err.Error()}
}

func overallStatus(results []OperationResult) OutputStatus {
	successes := 0
	for _, r := range results {
		if r.Status == "success" {
			successes++
		}
	}
	switch successes {
	case len(results):
		return StatusSuccess
	case 0:
		return StatusError
	default:
		return StatusPartialSuccess
	}
}

// readCommunity re-reads the community for the post-edit snapshot; nil if unreadable.
func readCommunity(ctx context.Context, client *http.Client, id string) *CommunitySummary {
	c, err := clients.GetCommunity(ctx, client, id)
	if err != nil {
		return nil
	}
	s := &CommunitySummary{ID: c.ID, Name: c.Name, Description: c.Description}
	if c.Parent != nil {
		s.ParentID, s.Parent = c.Parent.ID, c.Parent.Name
	}
	return s
}
