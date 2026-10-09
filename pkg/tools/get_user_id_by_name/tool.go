// Package get_user_id_by_name implements the get_user_id_by_name MCP tool: a
// read-only lookup that turns any human form of a Collibra user — their full
// name, username or email address — into the user's UUID, which is what the
// write tools' user-valued parameters ultimately need. It reads the user
// directory and writes nothing.
package get_user_id_by_name

import (
	"context"
	"net/http"

	"github.com/collibra/chip/pkg/chip"
	"github.com/collibra/chip/pkg/tools/resolve"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Input is the tool's typed input.
type Input struct {
	Name string `json:"name" jsonschema:"Required. How the user was referred to: a person's full name ('Jane Smith'), a username ('jane.smith'), an email address ('jane.smith@example.com'), or a user UUID (returned unchanged). A full name is matched against the directory's first name + last name, case-insensitively, over enabled (non-deactivated) accounts only."`
}

// Output is the tool's typed output. It deliberately carries no email address:
// no CHIP tool returns one.
type Output struct {
	UserID   string `json:"userId" jsonschema:"UUID of the matched user. Pass this to any tool parameter that takes a user."`
	FullName string `json:"fullName,omitempty" jsonschema:"The matched user's display name ('First Last'), so the right person can be confirmed. Absent when the input was already a UUID (no lookup is made in that case) or when the account has no first/last name."`
	Username string `json:"username,omitempty" jsonschema:"The matched user's username. Absent when the input was already a UUID."`
}

// NewTool returns the registered tool.
func NewTool(collibraClient *http.Client) *chip.Tool[Input, Output] {
	return &chip.Tool[Input, Output]{
		Name:  "get_user_id_by_name",
		Title: "Get User ID by Name",
		Description: "Look up a Collibra user's UUID from how a person was referred to in conversation. " +
			"Accepts a full name ('Jane Smith'), a username ('jane.smith'), an email address, or a UUID (returned unchanged, with no request made). " +
			"Collibra's write APIs identify users by UUID, so use this when a user has been named in words and a UUID is needed — for example before setting an assessment's owner or assignees, or an asset's steward. " +
			"Resolution order is fixed: a UUID passes through, a value containing '@' is looked up as an exact email address, an exact username wins next, and only then is the value matched against users' full names. " +
			"Returns the user's UUID together with their full name and username so the right person can be confirmed; it never returns email addresses. " +
			"If several users share the name, the call FAILS with an error listing every candidate (full name, username, UUID) — do NOT pick one of them yourself: ask which person is meant, then call again with that user's username or UUID. " +
			"If nothing matches, the error names the accepted input forms. Only enabled (non-deactivated) accounts are searched by name or username, so a deactivated leaver will not be found that way; the email lookup goes to a different endpoint and may still find one. " +
			"Do NOT use it for a user GROUP (a named set of users, e.g. 'Data Stewards'): it resolves individual users only, group names are not resolvable at all, and a group must be given to the calling tool as the group's UUID. " +
			"When a person cannot be resolved here — too many namesakes, a deactivated account, or a group rather than a person — use search_asset_keyword with resourceTypeFilters ['User'] (or ['UserGroup'] for a group) and take the id from its results. " +
			"Read-only: it queries the user directory and creates or changes nothing. Any authenticated user can call it. " +
			"Example questions it answers: 'What's John Doe's user id?'; 'Make Jane Smith the owner of the Customer Data assessment' (call this first to get her UUID); 'Assign the Steward role on this table to jane.smith'; 'Who is bob@example.com in Collibra?'.",
		Handler:     handler(collibraClient),
		Permissions: []string{},
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, DestructiveHint: chip.Ptr(false), IdempotentHint: true, OpenWorldHint: chip.Ptr(false)},
	}
}

func handler(collibraClient *http.Client) chip.ToolHandlerFunc[Input, Output] {
	return func(ctx context.Context, input Input) (Output, error) {
		// A failure to resolve is returned as a Go error, which the MCP SDK
		// surfaces as a tool error: the message already says what would have
		// been valid, or which candidates collided, so the model can retry
		// without a round trip. Reporting candidates as a successful payload
		// would invite the model to choose one.
		user, err := resolve.UserRef(ctx, collibraClient, input.Name, resolve.Hints{})
		if err != nil {
			return Output{}, err
		}
		return Output{UserID: user.ID, FullName: user.FullName, Username: user.UserName}, nil
	}
}
