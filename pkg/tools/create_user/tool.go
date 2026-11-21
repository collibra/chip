// Package create_user implements the create_user MCP tool: DGC's POST /rest/2.0/users, behind a
// confirm checkpoint.
package create_user

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

// StatusCreated is the answer to a confirmed call that created the user.
const StatusCreated outcome.Status = "created"

type Input struct {
	UserName     string   `json:"userName" jsonschema:"Required. The user name the person signs in with, for example 'jane.smith'. DGC requires it to be unique, so the tool refuses one that is taken."`
	EmailAddress string   `json:"emailAddress" jsonschema:"Required. The email address of the new user."`
	FirstName    string   `json:"firstName,omitempty" jsonschema:"Optional. First name."`
	LastName     string   `json:"lastName,omitempty" jsonschema:"Optional. Last name."`
	Title        string   `json:"title,omitempty" jsonschema:"Optional. Organizational title, for example 'Data Steward'."`
	Department   string   `json:"department,omitempty" jsonschema:"Optional. Organizational department."`
	Language     string   `json:"language,omitempty" jsonschema:"Optional. Language tag such as en or fr. DGC uses its default when omitted."`
	UserGroupIDs []string `json:"userGroupIds,omitempty" jsonschema:"Optional. UUIDs of user groups, from search_user_groups, to put the new user in."`
	Confirm      bool     `json:"confirm,omitempty" jsonschema:"Safety checkpoint. false (default) writes nothing and returns the user that would be created, to review with the user. true creates the user."`
}

type Output struct {
	Status       outcome.Status `json:"status" jsonschema:"created | confirm_required | needs_input | error."`
	Message      string         `json:"message" jsonschema:"What happened, or what would happen on confirm."`
	Guidance     string         `json:"guidance,omitempty" jsonschema:"What to do next, when the call did not create the user."`
	ID           string         `json:"id,omitempty" jsonschema:"UUID of the new user, once created."`
	UserName     string         `json:"userName,omitempty"`
	FirstName    string         `json:"firstName,omitempty"`
	LastName     string         `json:"lastName,omitempty"`
	EmailAddress string         `json:"emailAddress,omitempty" jsonschema:"Echoed from the call, not read from DGC, so the user can check what will be written."`
	Title        string         `json:"title,omitempty"`
	Department   string         `json:"department,omitempty"`
	Language     string         `json:"language,omitempty"`
	UserGroups   []string       `json:"userGroups,omitempty" jsonschema:"Names of the user groups the user is, or would be, put in."`
}

func NewTool(collibraClient *http.Client) *chip.Tool[Input, Output] {
	return &chip.Tool[Input, Output]{
		Name:  "create_user",
		Title: "Create User",
		Description: "Create a Collibra user: a person's account, identified by a unique user name and an email address, optionally with a name, title, department and language and with membership of user groups (named sets of users that can be given roles together). " +
			"The tool sets no password: the person gets one through Collibra's own sign-in or password-reset flow. A new user can use up a license, so confirm with the user before creating one. " +
			"Use search_users first if the person may already have an account: the tool refuses a user name that is taken. Get group ids from search_user_groups. " +
			"Built around a confirm checkpoint: confirm=false (the default) writes nothing and returns every property that would be written, to review with the user; confirm=true creates the user. " +
			"Returns the id of the new user. Requires the permission to add users. " +
			"Example user requests: \"Create an account for Jane Smith, jane.smith@example.com\"; \"Add a new user john.doe and put him in the Data Stewards group\".",
		Handler:     handler(collibraClient),
		Permissions: []string{},
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: false, DestructiveHint: chip.Ptr(false), IdempotentHint: false, OpenWorldHint: chip.Ptr(false)},
	}
}

func handler(collibraClient *http.Client) chip.ToolHandlerFunc[Input, Output] {
	return func(ctx context.Context, input Input) (Output, error) {
		userName := strings.TrimSpace(input.UserName)
		email := strings.TrimSpace(input.EmailAddress)
		if output, ok := invalid(userName, email, input.UserGroupIDs); ok {
			return output, nil
		}
		if existing, err := findByUserName(ctx, collibraClient, userName); err != nil {
			return failed("look for a user with that user name", err), nil
		} else if existing != nil {
			return Output{Status: outcome.NeedsInput, Message: fmt.Sprintf("A user named %q already exists.", existing.UserName), Guidance: "Use the existing user, or choose another user name.", ID: existing.ID, UserName: existing.UserName}, nil
		}
		groupNames, err := groupNames(ctx, collibraClient, input.UserGroupIDs)
		if err != nil {
			return failed("read the user groups", err), nil
		}
		report := Output{UserName: userName, FirstName: input.FirstName, LastName: input.LastName, EmailAddress: email, Title: input.Title, Department: input.Department, Language: input.Language, UserGroups: groupNames}
		if !input.Confirm {
			report.Status = outcome.ConfirmRequired
			report.Message = fmt.Sprintf("Would create the user %q with the properties below.", userName)
			report.Guidance = "Show the properties to the user. Call again with the same arguments and confirm=true to create the user."
			return report, nil
		}
		user, err := clients.AddUser(ctx, collibraClient, clients.AddUserRequest{
			UserName: userName, EmailAddress: email, FirstName: input.FirstName, LastName: input.LastName,
			Title: input.Title, Department: input.Department, Language: input.Language, UserGroupIDs: input.UserGroupIDs,
		})
		if err != nil {
			return failed("create the user", err), nil
		}
		report.Status = StatusCreated
		report.Message = fmt.Sprintf("Created the user %q. They have no password yet: it is set through Collibra's sign-in or reset flow.", user.UserName)
		report.ID = user.ID
		return report, nil
	}
}

func invalid(userName, email string, groupIDs []string) (Output, bool) {
	if userName == "" {
		return Output{Status: outcome.NeedsInput, Message: "userName is required.", Guidance: "Give the user name the person signs in with."}, true
	}
	if !strings.Contains(email, "@") {
		return Output{Status: outcome.NeedsInput, Message: fmt.Sprintf("emailAddress %q is not an email address.", email), Guidance: "Give an address such as jane.smith@example.com."}, true
	}
	if err := validation.DashedUUIDs("userGroupIds", groupIDs); err != nil {
		return Output{Status: outcome.NeedsInput, Message: err.Error(), Guidance: "Get group ids from search_user_groups."}, true
	}
	return Output{}, false
}

// findByUserName looks for the account that already has the user name: DGC matches the name as text
// anywhere in the user name or full name, so only an exact user name counts.
func findByUserName(ctx context.Context, client *http.Client, userName string) (*clients.DirectoryUser, error) {
	page, err := clients.SearchUsers(ctx, client, clients.SearchUsersRequest{
		Name: userName, IncludeDisabled: chip.Ptr(true), DirectoryPaging: clients.DirectoryPaging{Limit: chip.Ptr(50)},
	})
	if err != nil {
		return nil, err
	}
	for _, user := range page.Results {
		if strings.EqualFold(user.UserName, userName) {
			return &user, nil
		}
	}
	return nil, nil
}

func groupNames(ctx context.Context, client *http.Client, groupIDs []string) ([]string, error) {
	var names []string
	for _, id := range groupIDs {
		group, err := clients.GetUserGroup(ctx, client, id)
		if err != nil {
			return nil, err
		}
		names = append(names, group.Name)
	}
	return names, nil
}

func failed(action string, err error) Output {
	message, guidance := outcome.Describe(action, err)
	return Output{Status: outcome.Error, Message: message, Guidance: guidance}
}
