// Package search_users implements the search_users MCP tool, a thin interface to DGC's GET
// /rest/2.0/users: DGC's parameter names and defaults, except a default limit of 20.
package search_users

import (
	"context"
	"fmt"
	"net/http"

	"github.com/collibra/chip/pkg/chip"
	"github.com/collibra/chip/pkg/clients"
	"github.com/collibra/chip/pkg/tools/validation"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type Input struct {
	Name            string   `json:"name,omitempty" jsonschema:"Optional. Text to find in the user name, first name, last name or full name, case-insensitive."`
	GroupID         string   `json:"groupId,omitempty" jsonschema:"Optional. UUID of a user group, from search_user_groups, to return only its members."`
	UserIDs         []string `json:"userId,omitempty" jsonschema:"Optional. UUIDs of users to return, to look up names for known ids."`
	Title           string   `json:"title,omitempty" jsonschema:"Optional. Text to find in the organizational title."`
	Department      string   `json:"department,omitempty" jsonschema:"Optional. Text to find in the organizational department."`
	IncludeDisabled *bool    `json:"includeDisabled,omitempty" jsonschema:"Optional. Also return disabled users. Default: false."`
	Offset          *int     `json:"offset,omitempty" jsonschema:"Optional. Rows to skip. Default: 0."`
	Limit           *int     `json:"limit,omitempty" jsonschema:"Optional. Rows to return, at most 1000; 0 means 1000. Default: 20."`
	CountLimit      *int     `json:"countLimit,omitempty" jsonschema:"Optional. -1 counts every match in total, 0 skips the count (total is 0), n caps total at n. Default: -1."`
}

type Output struct {
	Total   int64                   `json:"total" jsonschema:"Matching users, as counted per countLimit."`
	Offset  int                     `json:"offset"`
	Limit   int                     `json:"limit"`
	Results []clients.DirectoryUser `json:"results"`
}

func NewTool(collibraClient *http.Client) *chip.Tool[Input, Output] {
	return &chip.Tool[Input, Output]{
		Name:  "search_users",
		Title: "Search Users",
		Description: "Find Collibra users (people with an account) by name, title or department, the members of a user group (groupId), or specific users by id (userId). " +
			"Returns each user's id, user name, first and last name, title, department and whether the account is enabled, plus total, offset and limit for paging; email addresses are never returned. " +
			"Use it to turn a person's name into the user id that other tools need, to see who is in a group, or to check an account. " +
			"At least one filter is required: an unfiltered call is refused rather than returning the whole directory. Disabled accounts are left out unless includeDisabled is true. " +
			"For the groups themselves use search_user_groups; for who holds a role on a resource use find_responsibilities. " +
			"Example user requests: \"Who is Jane Smith?\"; \"Who is in the Data Stewards group?\"; \"List the people in the Finance department\"; \"Find the account for jsmith\".",
		Handler:     handler(collibraClient),
		Permissions: []string{},
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, DestructiveHint: chip.Ptr(false), IdempotentHint: true, OpenWorldHint: chip.Ptr(false)},
	}
}

func handler(collibraClient *http.Client) chip.ToolHandlerFunc[Input, Output] {
	return func(ctx context.Context, input Input) (Output, error) {
		if input.Name == "" && input.GroupID == "" && len(input.UserIDs) == 0 && input.Title == "" && input.Department == "" {
			return Output{}, fmt.Errorf("give at least one filter: name, groupId, userId, title or department. An unfiltered search of every user is refused")
		}
		if err := validation.DashedUUIDOptional("groupId", input.GroupID); err != nil {
			return Output{}, err
		}
		if err := validation.DashedUUIDs("userId", input.UserIDs); err != nil {
			return Output{}, err
		}
		page, err := clients.SearchUsers(ctx, collibraClient, clients.SearchUsersRequest{
			UserIDs:         input.UserIDs,
			Name:            input.Name,
			GroupID:         input.GroupID,
			Title:           input.Title,
			Department:      input.Department,
			IncludeDisabled: input.IncludeDisabled,
			DirectoryPaging: clients.DirectoryPaging{Offset: input.Offset, Limit: input.Limit, CountLimit: input.CountLimit},
		})
		if err != nil {
			return Output{}, err
		}
		return Output{Total: page.Total, Offset: page.Offset, Limit: page.Limit, Results: page.Results}, nil
	}
}
