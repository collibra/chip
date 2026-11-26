// Package remove_users_from_user_group implements the remove_users_from_user_group MCP tool: DGC's
// DELETE /rest/2.0/userGroups/{id}/users, behind a confirm checkpoint.
package remove_users_from_user_group

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

// StatusRemoved is the answer to a confirmed call that removed the users.
const StatusRemoved outcome.Status = "removed"

type Input struct {
	UserGroupID string   `json:"userGroupId" jsonschema:"Required. UUID of the user group, in the form 00000000-0000-0000-0000-000000000000, from search_user_groups."`
	UserIDs     []string `json:"userIds" jsonschema:"Required. UUIDs of the users to take out of the group, from search_users (groupId). At least one. Named users who are not in the group are skipped, and unknown ids are reported."`
	Confirm     bool     `json:"confirm,omitempty" jsonschema:"Safety checkpoint. false (default) writes nothing and returns who would be removed, to review with the user. true removes them."`
}

type Output struct {
	Status         outcome.Status          `json:"status" jsonschema:"removed | confirm_required | needs_input | error."`
	Message        string                  `json:"message" jsonschema:"What happened, or what would happen on confirm."`
	Guidance       string                  `json:"guidance,omitempty" jsonschema:"What to do next, when the call did not remove anyone."`
	UserGroupID    string                  `json:"userGroupId,omitempty"`
	UserGroupName  string                  `json:"userGroupName,omitempty"`
	Users          []clients.DirectoryUser `json:"users,omitempty" jsonschema:"The users removed, or on confirm_required the users that would be removed."`
	NotMembers     []clients.DirectoryUser `json:"notMembers,omitempty" jsonschema:"Named users who are not in the group, and so are left alone."`
	UnknownUserIDs []string                `json:"unknownUserIds,omitempty" jsonschema:"Named ids that match no user."`
}

func NewTool(collibraClient *http.Client) *chip.Tool[Input, Output] {
	return &chip.Tool[Input, Output]{
		Name:  "remove_users_from_user_group",
		Title: "Remove Users from User Group",
		Description: "Take users out of a Collibra user group (a named set of users that can be given roles together). " +
			"The users stay in Collibra, but they lose every role and every notification they received only through this group. " +
			"Give the group's id from search_user_groups and the users' ids from search_users (groupId lists the members). " +
			"Named users who are not in the group are skipped and ids that match no user are reported instead of failing the call. " +
			"Built around a confirm checkpoint: confirm=false (the default) writes nothing and returns who would be removed, to review with the user; confirm=true removes them. " +
			"To delete the whole group use delete_user_group. Requires the permission to change user groups. " +
			"Example user requests: \"Take John Doe out of the Data Stewards group\"; \"Remove everyone who left the company from the Product Owners group\".",
		Handler:     handler(collibraClient),
		Permissions: []string{},
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: false, DestructiveHint: chip.Ptr(true), IdempotentHint: true, OpenWorldHint: chip.Ptr(false)},
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
		toRemove := lookup.InGroup()
		report := Output{UserGroupID: group.ID, UserGroupName: group.Name, Users: toRemove, NotMembers: lookup.NotInGroup(), UnknownUserIDs: lookup.Missing}
		if len(toRemove) == 0 {
			report.Status = outcome.NeedsInput
			report.Message = fmt.Sprintf("Nobody to remove from %q: %d are not members and %d ids match no user.", group.Name, len(report.NotMembers), len(lookup.Missing))
			report.Guidance = "List the members with search_users (groupId) and retry."
			return report, nil
		}
		if !input.Confirm {
			report.Status = outcome.ConfirmRequired
			report.Message = fmt.Sprintf("Would remove %d users from the user group %q (%d not members, %d unknown ids skipped).", len(toRemove), group.Name, len(report.NotMembers), len(lookup.Missing))
			report.Guidance = "Show the users to the user. Call again with the same arguments and confirm=true to remove them."
			return report, nil
		}
		if err := clients.RemoveUsersFromUserGroup(ctx, collibraClient, group.ID, membership.IDs(toRemove)); err != nil {
			return failed("remove the users from the user group", err), nil
		}
		report.Status = StatusRemoved
		report.Message = fmt.Sprintf("Removed %d users from the user group %q.", len(toRemove), group.Name)
		return report, nil
	}
}

func invalid(input Input) (Output, bool) {
	if err := validation.DashedUUID("userGroupId", input.UserGroupID); err != nil {
		return Output{Status: outcome.NeedsInput, Message: err.Error(), Guidance: "Get the id from search_user_groups."}, true
	}
	if len(input.UserIDs) == 0 {
		return Output{Status: outcome.NeedsInput, Message: "userIds needs at least one user id.", Guidance: "List the members with search_users (groupId)."}, true
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
