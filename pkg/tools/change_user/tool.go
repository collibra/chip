// Package change_user implements the change_user MCP tool: DGC's PATCH /rest/2.0/users/{id}, behind
// a confirm checkpoint.
package change_user

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/collibra/chip/pkg/chip"
	"github.com/collibra/chip/pkg/clients"
	"github.com/collibra/chip/pkg/tools/outcome"
	"github.com/collibra/chip/pkg/tools/validation"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// StatusChanged is the answer to a confirmed call that changed the user.
const StatusChanged outcome.Status = "changed"

type Input struct {
	UserID       string `json:"userId" jsonschema:"Required. UUID of the user to change, in the form 00000000-0000-0000-0000-000000000000, from search_users."`
	UserName     string `json:"userName,omitempty" jsonschema:"Optional. New user name, which DGC requires to be unique."`
	EmailAddress string `json:"emailAddress,omitempty" jsonschema:"Optional. New email address."`
	FirstName    string `json:"firstName,omitempty" jsonschema:"Optional. New first name."`
	LastName     string `json:"lastName,omitempty" jsonschema:"Optional. New last name."`
	Title        string `json:"title,omitempty" jsonschema:"Optional. New organizational title."`
	Department   string `json:"department,omitempty" jsonschema:"Optional. New organizational department."`
	Language     string `json:"language,omitempty" jsonschema:"Optional. New language tag such as en or fr."`
	Enabled      *bool  `json:"enabled,omitempty" jsonschema:"Optional. false disables the account, so the person can no longer sign in; true enables it again."`
	Confirm      bool   `json:"confirm,omitempty" jsonschema:"Safety checkpoint. false (default) writes nothing and returns the changes that would be made, to review with the user. true applies them."`
}

// Change is one property that changes: its name, what it is now and what it becomes.
type Change struct {
	Field string `json:"field"`
	From  string `json:"from,omitempty" jsonschema:"The current value. Absent for the email address, which no tool reads out."`
	To    string `json:"to"`
}

type Output struct {
	Status    outcome.Status `json:"status" jsonschema:"changed | confirm_required | needs_input | error."`
	Message   string         `json:"message" jsonschema:"What happened, or what would happen on confirm."`
	Guidance  string         `json:"guidance,omitempty" jsonschema:"What to do next, when the call did not change the user."`
	ID        string         `json:"id,omitempty"`
	UserName  string         `json:"userName,omitempty" jsonschema:"The user name before the change."`
	Changes   []Change       `json:"changes,omitempty" jsonschema:"Every property that is, or would be, written."`
	Enabled   bool           `json:"enabled" jsonschema:"Whether the account is enabled after the change, or as it is now when nothing was written."`
	FirstName string         `json:"firstName,omitempty"`
	LastName  string         `json:"lastName,omitempty"`
}

func NewTool(collibraClient *http.Client) *chip.Tool[Input, Output] {
	return &chip.Tool[Input, Output]{
		Name:  "change_user",
		Title: "Change User",
		Description: "Change the properties of an existing Collibra user: user name, email address, first and last name, title, department, language, or whether the account is enabled. " +
			"Only the properties you send change; the others stay as they are. Disabling an account stops that person signing in, so confirm with the user before you do. " +
			"Find the user's id with search_users first. Group membership is not changed here: use add_users_to_user_group and remove_users_from_user_group. " +
			"Built around a confirm checkpoint: confirm=false (the default) writes nothing and returns each property with its current and its new value, to review with the user; confirm=true applies the changes. " +
			"Requires the permission to change users. " +
			"Example user requests: \"Disable the account of john.doe\"; \"Change Jane Smith's title to Head of Data\"; \"Move Pat Lee to the Finance department\".",
		Handler:     handler(collibraClient),
		Permissions: []string{},
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: false, DestructiveHint: chip.Ptr(true), IdempotentHint: true, OpenWorldHint: chip.Ptr(false)},
	}
}

func handler(collibraClient *http.Client) chip.ToolHandlerFunc[Input, Output] {
	return func(ctx context.Context, input Input) (Output, error) {
		if err := validation.DashedUUID("userId", input.UserID); err != nil {
			return Output{Status: outcome.NeedsInput, Message: err.Error(), Guidance: "Get the id from search_users."}, nil
		}
		page, err := clients.SearchUsers(ctx, collibraClient, clients.SearchUsersRequest{
			UserIDs: []string{input.UserID}, IncludeDisabled: chip.Ptr(true), DirectoryPaging: clients.DirectoryPaging{Limit: chip.Ptr(1)},
		})
		if err != nil {
			return failed("read the user", err), nil
		}
		if len(page.Results) == 0 {
			return Output{Status: outcome.NeedsInput, Message: fmt.Sprintf("No user has the id %s.", input.UserID), Guidance: "Look the user up again with search_users."}, nil
		}
		current := page.Results[0]
		changes, request := plan(current, input)
		report := Output{ID: current.ID, UserName: current.UserName, Changes: changes, Enabled: current.Enabled, FirstName: current.FirstName, LastName: current.LastName}
		if len(changes) == 0 {
			report.Status = outcome.NeedsInput
			report.Message = "Nothing to change: no property was sent, or each one already has the value sent."
			report.Guidance = "Send at least one of userName, emailAddress, firstName, lastName, title, department, language or enabled with a new value."
			return report, nil
		}
		if !input.Confirm {
			report.Status = outcome.ConfirmRequired
			report.Message = fmt.Sprintf("Would change %d properties of the user %q.", len(changes), current.UserName)
			report.Guidance = "Show the changes to the user. Call again with the same arguments and confirm=true to apply them."
			return report, nil
		}
		changed, err := clients.ChangeUser(ctx, collibraClient, current.ID, request)
		if err != nil {
			return failed("change the user", err), nil
		}
		report.Status = StatusChanged
		report.Message = fmt.Sprintf("Changed %d properties of the user %q.", len(changes), current.UserName)
		report.Enabled, report.FirstName, report.LastName = changed.Enabled, changed.FirstName, changed.LastName
		return report, nil
	}
}

// plan compares what was asked with what the user is now: it lists each property that really changes,
// and builds the request that writes only those.
func plan(current clients.DirectoryUser, input Input) ([]Change, clients.ChangeUserRequest) {
	var changes []Change
	var request clients.ChangeUserRequest
	text := func(field, from, to string, set func()) {
		to = strings.TrimSpace(to)
		if to != "" && to != from {
			changes = append(changes, Change{Field: field, From: from, To: to})
			set()
		}
	}
	text("userName", current.UserName, input.UserName, func() { request.UserName = strings.TrimSpace(input.UserName) })
	text("firstName", current.FirstName, input.FirstName, func() { request.FirstName = strings.TrimSpace(input.FirstName) })
	text("lastName", current.LastName, input.LastName, func() { request.LastName = strings.TrimSpace(input.LastName) })
	text("title", current.Title, input.Title, func() { request.Title = strings.TrimSpace(input.Title) })
	text("department", current.Department, input.Department, func() { request.Department = strings.TrimSpace(input.Department) })
	if language := strings.TrimSpace(input.Language); language != "" {
		changes = append(changes, Change{Field: "language", To: language})
		request.Language = language
	}
	if email := strings.TrimSpace(input.EmailAddress); email != "" {
		changes = append(changes, Change{Field: "emailAddress", To: email})
		request.EmailAddress = email
	}
	if input.Enabled != nil && *input.Enabled != current.Enabled {
		changes = append(changes, Change{Field: "enabled", From: strconv.FormatBool(current.Enabled), To: strconv.FormatBool(*input.Enabled)})
		request.Enabled = input.Enabled
	}
	return changes, request
}

func failed(action string, err error) Output {
	message, guidance := outcome.Describe(action, err)
	return Output{Status: outcome.Error, Message: message, Guidance: guidance}
}
