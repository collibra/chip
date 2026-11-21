// Package create_user_group implements the create_user_group MCP tool: DGC's POST
// /rest/2.0/userGroups, behind a confirm checkpoint.
package create_user_group

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/collibra/chip/pkg/chip"
	"github.com/collibra/chip/pkg/clients"
	"github.com/collibra/chip/pkg/tools/outcome"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// StatusCreated is the answer to a confirmed call that created the group.
const StatusCreated outcome.Status = "created"

type Input struct {
	Name    string `json:"name" jsonschema:"Required. The name of the new user group, for example 'Product Owners'. DGC does not allow two user groups with the same name, so the tool refuses a name that is taken."`
	Confirm bool   `json:"confirm,omitempty" jsonschema:"Safety checkpoint. false (default) writes nothing and returns what would be created, to review with the user. true creates the group."`
}

type Output struct {
	Status   outcome.Status `json:"status" jsonschema:"created | confirm_required | needs_input | error."`
	Message  string         `json:"message" jsonschema:"What happened, or what would happen on confirm."`
	Guidance string         `json:"guidance,omitempty" jsonschema:"What to do next, when the call did not create the group."`
	ID       string         `json:"id,omitempty" jsonschema:"UUID of the user group: the new group once created, or the existing group that already has the name. Pass it to add_users_to_user_group."`
	Name     string         `json:"name,omitempty" jsonschema:"The group name that is, or would be, created."`
}

func NewTool(collibraClient *http.Client) *chip.Tool[Input, Output] {
	return &chip.Tool[Input, Output]{
		Name:  "create_user_group",
		Title: "Create User Group",
		Description: "Create an empty Collibra user group. A user group is a named set of users that can be given roles, and be notified, together; " +
			"every member gets what the group is given. The new group has no members and holds no roles: add members with add_users_to_user_group. " +
			"Use search_user_groups first if the group may already exist: user group names must be unique, and the tool refuses a name that is taken and returns the id of the existing group. " +
			"Built around a confirm checkpoint: confirm=false (the default) writes nothing and returns the group that would be created, to review with the user; confirm=true creates it. " +
			"Returns the id of the new group. " +
			"Requires the permission to create user groups. " +
			"Example user requests: \"Create a user group called Product Owners\"; \"Make a group for our data stewards\"; \"I need a new group for the finance analysts\".",
		Handler:     handler(collibraClient),
		Permissions: []string{},
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: false, DestructiveHint: chip.Ptr(false), IdempotentHint: false, OpenWorldHint: chip.Ptr(false)},
	}
}

func handler(collibraClient *http.Client) chip.ToolHandlerFunc[Input, Output] {
	return func(ctx context.Context, input Input) (Output, error) {
		name := strings.TrimSpace(input.Name)
		if name == "" {
			return Output{Status: outcome.NeedsInput, Message: "name is required.", Guidance: "Give the name of the user group to create."}, nil
		}
		taken, err := clients.SearchUserGroups(ctx, collibraClient, clients.SearchUserGroupsRequest{
			Name: name, NameMatchMode: "EXACT", DirectoryPaging: clients.DirectoryPaging{Limit: chip.Ptr(1)},
		})
		if err != nil {
			return failed("look for a user group with that name", err), nil
		}
		if len(taken.Results) > 0 {
			return Output{
				Status:   outcome.NeedsInput,
				Message:  fmt.Sprintf("A user group named %q already exists.", taken.Results[0].Name),
				Guidance: "Use the existing group, or choose another name.",
				ID:       taken.Results[0].ID,
				Name:     taken.Results[0].Name,
			}, nil
		}
		if !input.Confirm {
			return Output{
				Status:   outcome.ConfirmRequired,
				Message:  fmt.Sprintf("Would create an empty user group named %q.", name),
				Guidance: "Show this to the user. Call again with the same name and confirm=true to create the group.",
				Name:     name,
			}, nil
		}
		group, err := clients.AddUserGroup(ctx, collibraClient, name)
		if err != nil {
			return failed("create the user group", err), nil
		}
		return Output{Status: StatusCreated, Message: fmt.Sprintf("Created the user group %q. It has no members yet.", group.Name), ID: group.ID, Name: group.Name}, nil
	}
}

func failed(action string, err error) Output {
	message, guidance := outcome.Describe(action, err)
	return Output{Status: outcome.Error, Message: message, Guidance: guidance}
}
