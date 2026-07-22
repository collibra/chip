// Package find_users implements the find_users MCP tool: resolves a user's name
// (or partial name) to their UUID, e.g. to fill in register_database's ownerIds
// without the caller needing to already know the id.
package find_users

import (
	"context"
	"net/http"

	"github.com/collibra/chip/pkg/chip"
	"github.com/collibra/chip/pkg/clients"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type Input struct {
	Name            string `json:"name,omitempty" jsonschema:"Optional. Matches case-insensitively as a substring against username, first name, last name, and first+last name combinations (e.g. 'admin', 'jane', 'jane doe'). Omit to list users."`
	IncludeDisabled bool   `json:"includeDisabled,omitempty" jsonschema:"Optional. If true, includes disabled user accounts. Defaults to false (enabled users only)."`
	Limit           int    `json:"limit,omitempty" jsonschema:"Optional. Maximum number of results to return. Default: 20."`
	Offset          int    `json:"offset,omitempty" jsonschema:"Optional. Index of first result (pagination offset). Default: 0."`
}

type Output struct {
	Total  int64  `json:"total" jsonschema:"The total number of users matching the search criteria."`
	Offset int64  `json:"offset" jsonschema:"The offset for the results."`
	Limit  int64  `json:"limit" jsonschema:"The maximum number of results returned."`
	Users  []User `json:"users" jsonschema:"The matching users."`
}

type User struct {
	ID           string `json:"id" jsonschema:"The user's UUID — use this for ownerIds, createdBy, etc."`
	UserName     string `json:"userName,omitempty" jsonschema:"The user's username."`
	FirstName    string `json:"firstName,omitempty"`
	LastName     string `json:"lastName,omitempty"`
	EmailAddress string `json:"emailAddress,omitempty"`
	Enabled      bool   `json:"enabled" jsonschema:"Whether the user account is enabled."`
}

func NewTool(collibraClient *http.Client) *chip.Tool[Input, Output] {
	return &chip.Tool[Input, Output]{
		Name:        "find_users",
		Title:       "Find Users",
		Description: "Finds DGC users by name (substring, case-insensitive), returning their UUIDs. Use this to resolve a name to an id for register_database's ownerIds or similar owner/assignee fields, instead of needing to already know the UUID.",
		Handler:     handler(collibraClient),
		Permissions: []string{},
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}
}

func handler(collibraClient *http.Client) chip.ToolHandlerFunc[Input, Output] {
	return func(ctx context.Context, input Input) (Output, error) {
		limit := input.Limit
		if limit == 0 {
			limit = 20
		}

		response, err := clients.FindUsers(ctx, collibraClient, clients.FindUsersQueryParams{
			Name:            input.Name,
			Limit:           limit,
			Offset:          input.Offset,
			IncludeDisabled: input.IncludeDisabled,
		})
		if err != nil {
			return Output{}, err
		}

		users := make([]User, len(response.Results))
		for i, u := range response.Results {
			users[i] = User{
				ID:           u.ID,
				UserName:     u.UserName,
				FirstName:    u.FirstName,
				LastName:     u.LastName,
				EmailAddress: u.EmailAddress,
				Enabled:      u.Enabled,
			}
		}

		return Output{Total: response.Total, Offset: response.Offset, Limit: response.Limit, Users: users}, nil
	}
}
