// Package rename_user_group implements the rename_user_group MCP tool: DGC's PATCH
// /rest/2.0/userGroups/{id}, which changes the name of a user group, behind a confirm checkpoint.
package rename_user_group

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/collibra/chip/pkg/chip"
	"github.com/collibra/chip/pkg/clients"
	"github.com/collibra/chip/pkg/tools/outcome"
	"github.com/collibra/chip/pkg/tools/validation"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// StatusRenamed is the answer to a confirmed call that renamed the group.
const StatusRenamed outcome.Status = "renamed"

type Input struct {
	UserGroupID string `json:"userGroupId" jsonschema:"Required. UUID of the user group to rename, in the form 00000000-0000-0000-0000-000000000000, from search_user_groups."`
	Name        string `json:"name" jsonschema:"Required. The new name. DGC does not allow two user groups with the same name, so the tool refuses a name that is taken."`
	Confirm     bool   `json:"confirm,omitempty" jsonschema:"Safety checkpoint. false (default) writes nothing and returns the change that would be made, to review with the user. true renames the group."`
}

type Output struct {
	Status       outcome.Status `json:"status" jsonschema:"renamed | confirm_required | needs_input | error."`
	Message      string         `json:"message" jsonschema:"What happened, or what would happen on confirm."`
	Guidance     string         `json:"guidance,omitempty" jsonschema:"What to do next, when the call did not rename the group."`
	ID           string         `json:"id,omitempty" jsonschema:"UUID of the user group."`
	PreviousName string         `json:"previousName,omitempty" jsonschema:"The name before the change."`
	Name         string         `json:"name,omitempty" jsonschema:"The name the group has, or would have after the change."`
}

func NewTool(collibraClient *http.Client) *chip.Tool[Input, Output] {
	return &chip.Tool[Input, Output]{
		Name:  "rename_user_group",
		Title: "Rename User Group",
		Description: "Change the name of a Collibra user group (a named set of users that can be given roles together). " +
			"Only the name changes: the group keeps its members and the roles it holds. " +
			"Find the group's id with search_user_groups first. The new name must not be used by another user group; the tool refuses a name that is taken. " +
			"Built around a confirm checkpoint: confirm=false (the default) writes nothing and returns the old and the new name, to review with the user; confirm=true renames the group. " +
			"Requires the permission to change user groups. " +
			"Example user requests: \"Rename the group Stewards to Data Stewards\"; \"Call the Product Owners group Product Owners EMEA instead\".",
		Handler:     handler(collibraClient),
		Permissions: []string{},
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: false, DestructiveHint: chip.Ptr(false), IdempotentHint: true, OpenWorldHint: chip.Ptr(false)},
	}
}

func handler(collibraClient *http.Client) chip.ToolHandlerFunc[Input, Output] {
	return func(ctx context.Context, input Input) (Output, error) {
		if err := validation.DashedUUID("userGroupId", input.UserGroupID); err != nil {
			return Output{Status: outcome.NeedsInput, Message: err.Error(), Guidance: "Get the id from search_user_groups."}, nil
		}
		name := strings.TrimSpace(input.Name)
		if name == "" {
			return Output{Status: outcome.NeedsInput, Message: "name is required.", Guidance: "Give the new name of the user group."}, nil
		}
		current, err := clients.GetUserGroup(ctx, collibraClient, input.UserGroupID)
		if err != nil {
			return failed("read the user group", err), nil
		}
		if current.Name == name {
			return Output{Status: outcome.NeedsInput, Message: fmt.Sprintf("The user group is already named %q.", name), ID: current.ID, Name: name}, nil
		}
		taken, err := clients.SearchUserGroups(ctx, collibraClient, clients.SearchUserGroupsRequest{
			Name: name, NameMatchMode: "EXACT", DirectoryPaging: clients.DirectoryPaging{Limit: chip.Ptr(1)},
		})
		if err != nil {
			return failed("look for a user group with that name", err), nil
		}
		if len(taken.Results) > 0 && taken.Results[0].ID != current.ID {
			return Output{
				Status:   outcome.NeedsInput,
				Message:  fmt.Sprintf("Another user group is already named %q.", name),
				Guidance: "Choose another name.",
				ID:       current.ID,
			}, nil
		}
		if !input.Confirm {
			return Output{
				Status:       outcome.ConfirmRequired,
				Message:      fmt.Sprintf("Would rename the user group %q to %q.", current.Name, name),
				Guidance:     "Show this to the user. Call again with the same arguments and confirm=true to rename the group.",
				ID:           current.ID,
				PreviousName: current.Name,
				Name:         name,
			}, nil
		}
		renamed, err := clients.ChangeUserGroup(ctx, collibraClient, input.UserGroupID, name)
		if err != nil {
			return failed("rename the user group", err), nil
		}
		return Output{
			Status:       StatusRenamed,
			Message:      fmt.Sprintf("Renamed the user group %q to %q.", current.Name, renamed.Name),
			ID:           renamed.ID,
			PreviousName: current.Name,
			Name:         renamed.Name,
		}, nil
	}
}

func failed(action string, err error) Output {
	message, guidance := outcome.Describe(action, err)
	return Output{Status: outcome.Error, Message: message, Guidance: guidance}
}
