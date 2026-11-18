// Package add_users_to_user_group implements the add_users_to_user_group MCP tool: DGC's POST
// /rest/2.0/userGroups/{id}/users, behind a confirm checkpoint.
package add_users_to_user_group

import (
	"context"
	"fmt"
	"net/http"

	"github.com/collibra/chip/pkg/chip"
	"github.com/collibra/chip/pkg/clients"
	"github.com/collibra/chip/pkg/tools/membership"
	"github.com/collibra/chip/pkg/tools/outcome"
	"github.com/collibra/chip/pkg/tools/validation"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// StatusAdded is the answer to a confirmed call that added the users.
const StatusAdded outcome.Status = "added"

type Input struct {
	UserGroupID string   `json:"userGroupId" jsonschema:"Required. UUID of the user group, in the form 00000000-0000-0000-0000-000000000000, from search_user_groups or create_user_group."`
	UserIDs     []string `json:"userIds" jsonschema:"Required. UUIDs of the users to add, from search_users or find_responsibilities (owners of type User). At least one. Users who are already in the group are left out, and unknown ids are reported."`
	Confirm     bool     `json:"confirm,omitempty" jsonschema:"Safety checkpoint. false (default) writes nothing and returns who would be added, to review with the user. true adds them."`
}

type Output struct {
	Status         outcome.Status          `json:"status" jsonschema:"added | confirm_required | needs_input | error."`
	Message        string                  `json:"message" jsonschema:"What happened, or what would happen on confirm."`
	Guidance       string                  `json:"guidance,omitempty" jsonschema:"What to do next, when the call did not add anyone."`
	UserGroupID    string                  `json:"userGroupId,omitempty"`
	UserGroupName  string                  `json:"userGroupName,omitempty"`
	Users          []clients.DirectoryUser `json:"users,omitempty" jsonschema:"The users added, or on confirm_required the users that would be added."`
	AlreadyMembers []clients.DirectoryUser `json:"alreadyMembers,omitempty" jsonschema:"Named users who are already in the group, and so are not added again."`
	UnknownUserIDs []string                `json:"unknownUserIds,omitempty" jsonschema:"Named ids that match no user, and so are not added."`
}

func NewTool(collibraClient *http.Client) *chip.Tool[Input, Output] {
	return &chip.Tool[Input, Output]{
		Name:  "add_users_to_user_group",
		Title: "Add Users to User Group",
		Description: "Add users to an existing Collibra user group (a named set of users that can be given roles together). " +
			"The users are added on top of the current members; nobody is removed. Every member gets all the roles the group holds, so check who holds what with find_responsibilities (ownerIds) before adding people to a group that has powerful roles. " +
			"Give the group's id from search_user_groups (or create_user_group) and the users' ids from search_users or find_responsibilities. " +
			"Users already in the group are skipped, and ids that match no user are reported instead of failing the call. " +
			"Built around a confirm checkpoint: confirm=false (the default) writes nothing and returns who would be added, who is already a member and which ids are unknown, to review with the user; confirm=true adds the users. " +
			"Requires the permission to change user groups. " +
			"Example user requests: \"Put Jane Smith in the Data Stewards group\"; \"Add all the product owners to the Product Owners group\"; \"Make these five people members of the finance group\".",
		Handler:     handler(collibraClient),
		Permissions: []string{},
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: false, DestructiveHint: chip.Ptr(false), IdempotentHint: true, OpenWorldHint: chip.Ptr(false)},
	}
}

func handler(collibraClient *http.Client) chip.ToolHandlerFunc[Input, Output] {
	return func(ctx context.Context, input Input) (Output, error) {
		if output, ok := invalid(input); ok {
			return output, nil
		}
		group, err := clients.GetUserGroup(ctx, collibraClient, input.UserGroupID)
		if err != nil {
			return failed("read the user group", err), nil
		}
		lookup, err := membership.Resolve(ctx, collibraClient, group.ID, input.UserIDs)
		if err != nil {
			return failed("look up the users", err), nil
		}
		toAdd := lookup.NotInGroup()
		report := Output{UserGroupID: group.ID, UserGroupName: group.Name, Users: toAdd, AlreadyMembers: lookup.InGroup(), UnknownUserIDs: lookup.Missing}
		if len(toAdd) == 0 {
			report.Status = outcome.NeedsInput
			report.Message = fmt.Sprintf("Nobody to add to %q: %d are already members and %d ids match no user.", group.Name, len(report.AlreadyMembers), len(lookup.Missing))
			report.Guidance = "Check the user ids with search_users."
			return report, nil
		}
		if !input.Confirm {
			report.Status = outcome.ConfirmRequired
			report.Message = fmt.Sprintf("Would add %d users to the user group %q (%d already members, %d unknown ids skipped).", len(toAdd), group.Name, len(report.AlreadyMembers), len(lookup.Missing))
			report.Guidance = "Show the users to the user. Call again with the same arguments and confirm=true to add them."
			return report, nil
		}
		if _, err := clients.AddUsersToUserGroup(ctx, collibraClient, group.ID, membership.IDs(toAdd)); err != nil {
			return failed("add the users to the user group", err), nil
		}
		report.Status = StatusAdded
		report.Message = fmt.Sprintf("Added %d users to the user group %q.", len(toAdd), group.Name)
		return report, nil
	}
}

func invalid(input Input) (Output, bool) {
	if err := validation.DashedUUID("userGroupId", input.UserGroupID); err != nil {
		return Output{Status: outcome.NeedsInput, Message: err.Error(), Guidance: "Get the id from search_user_groups."}, true
	}
	if len(input.UserIDs) == 0 {
		return Output{Status: outcome.NeedsInput, Message: "userIds needs at least one user id.", Guidance: "Get user ids from search_users."}, true
	}
	if err := validation.DashedUUIDs("userIds", input.UserIDs); err != nil {
		return Output{Status: outcome.NeedsInput, Message: err.Error(), Guidance: "Get user ids from search_users."}, true
	}
	return Output{}, false
}

func failed(action string, err error) Output {
	message, guidance := outcome.Describe(action, err)
	return Output{Status: outcome.Error, Message: message, Guidance: guidance}
}
