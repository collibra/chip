// Package delete_user_group implements the delete_user_group MCP tool: DGC's DELETE
// /rest/2.0/userGroups/{id}, behind a confirm checkpoint.
package delete_user_group

import (
	"context"
	"fmt"
	"net/http"

	"github.com/collibra/chip/pkg/chip"
	"github.com/collibra/chip/pkg/clients"
	"github.com/collibra/chip/pkg/tools/outcome"
	"github.com/collibra/chip/pkg/tools/validation"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// StatusDeleted is the answer to a confirmed call that deleted the group.
const StatusDeleted outcome.Status = "deleted"

type Input struct {
	UserGroupID string `json:"userGroupId" jsonschema:"Required. UUID of the user group to delete, in the form 00000000-0000-0000-0000-000000000000, from search_user_groups."`
	Confirm     bool   `json:"confirm,omitempty" jsonschema:"Safety checkpoint. false (default) writes nothing and returns the group with how many members and role assignments it has, to review with the user. true permanently deletes the group."`
}

type Output struct {
	Status          outcome.Status `json:"status" jsonschema:"deleted | confirm_required | needs_input | error."`
	Message         string         `json:"message" jsonschema:"What happened, or what would happen on confirm."`
	Guidance        string         `json:"guidance,omitempty" jsonschema:"What to do next, when the call did not delete the group."`
	ID              string         `json:"id,omitempty" jsonschema:"UUID of the user group."`
	Name            string         `json:"name,omitempty" jsonschema:"Name of the user group."`
	MemberCount     int64          `json:"memberCount,omitempty" jsonschema:"On confirm_required: how many users are in the group."`
	AssignmentCount int64          `json:"assignmentCount,omitempty" jsonschema:"On confirm_required: how many role assignments the group holds. All of them are lost with the group."`
}

func NewTool(collibraClient *http.Client) *chip.Tool[Input, Output] {
	return &chip.Tool[Input, Output]{
		Name:  "delete_user_group",
		Title: "Delete User Group",
		Description: "PERMANENTLY DELETE a Collibra user group (a named set of users that can be given roles together). THIS CANNOT BE UNDONE. " +
			"The users in the group stay in Collibra, but every role the group held on a community, domain or asset is lost with it, and so is the access that came with those roles. " +
			"Built around a confirm checkpoint: confirm=false (the default) deletes nothing and returns the group with how many members and role assignments it has, to review with the user; confirm=true deletes it. " +
			"Find the group's id with search_user_groups; see what it holds with find_responsibilities (ownerIds). To only take people out of a group, use remove_users_from_user_group instead. " +
			"System groups cannot be deleted. Requires the permission to delete user groups. " +
			"Example user requests: \"Delete the group Old Stewards\"; \"Remove the Product Owners group, we don't use it any more\".",
		Handler:     handler(collibraClient),
		Permissions: []string{},
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: false, DestructiveHint: chip.Ptr(true), IdempotentHint: false, OpenWorldHint: chip.Ptr(false)},
	}
}

func handler(collibraClient *http.Client) chip.ToolHandlerFunc[Input, Output] {
	return func(ctx context.Context, input Input) (Output, error) {
		if err := validation.DashedUUID("userGroupId", input.UserGroupID); err != nil {
			return Output{Status: outcome.NeedsInput, Message: err.Error(), Guidance: "Get the id from search_user_groups."}, nil
		}
		group, err := clients.GetUserGroup(ctx, collibraClient, input.UserGroupID)
		if err != nil {
			return failed("read the user group", err), nil
		}
		if group.System {
			return Output{Status: outcome.NeedsInput, Message: fmt.Sprintf("%q is a system group and cannot be deleted.", group.Name), ID: group.ID, Name: group.Name}, nil
		}
		if !input.Confirm {
			return preview(ctx, collibraClient, group)
		}
		if err := clients.RemoveUserGroup(ctx, collibraClient, group.ID); err != nil {
			return failed("delete the user group", err), nil
		}
		return Output{Status: StatusDeleted, Message: fmt.Sprintf("Permanently deleted the user group %q.", group.Name), ID: group.ID, Name: group.Name}, nil
	}
}

// preview counts what the group holds, so the user can see what is lost with it.
func preview(ctx context.Context, collibraClient *http.Client, group *clients.DirectoryUserGroup) (Output, error) {
	one := clients.DirectoryPaging{Limit: chip.Ptr(1)}
	members, err := clients.SearchUsers(ctx, collibraClient, clients.SearchUsersRequest{GroupID: group.ID, IncludeDisabled: chip.Ptr(true), DirectoryPaging: one})
	if err != nil {
		return failed("count the members of the user group", err), nil
	}
	assignments, err := clients.SearchResponsibilities(ctx, collibraClient, clients.SearchResponsibilitiesRequest{OwnerIDs: []string{group.ID}, DirectoryPaging: one})
	if err != nil {
		return failed("count the role assignments of the user group", err), nil
	}
	return Output{
		Status: outcome.ConfirmRequired,
		Message: fmt.Sprintf("Would permanently delete the user group %q, which has %d members and holds %d role assignments. The role assignments are lost with it.",
			group.Name, members.Total, assignments.Total),
		Guidance:        "Show this to the user. Call again with the same userGroupId and confirm=true to delete the group.",
		ID:              group.ID,
		Name:            group.Name,
		MemberCount:     members.Total,
		AssignmentCount: assignments.Total,
	}, nil
}

func failed(action string, err error) Output {
	message, guidance := outcome.Describe(action, err)
	return Output{Status: outcome.Error, Message: message, Guidance: guidance}
}
