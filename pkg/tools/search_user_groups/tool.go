// Package search_user_groups implements the search_user_groups MCP tool, a thin interface to DGC's
// GET /rest/2.0/userGroups: DGC's parameter names and defaults, except a default limit of 20.
package search_user_groups

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
	Name          string `json:"name,omitempty" jsonschema:"Optional. Text to find in the group name."`
	NameMatchMode string `json:"nameMatchMode,omitempty" jsonschema:"Optional. START, END, ANYWHERE or EXACT, always case-insensitive. Default: ANYWHERE."`
	UserID        string `json:"userId,omitempty" jsonschema:"Optional. UUID of a user, from search_users, to return the groups that user belongs to."`
	Offset        *int   `json:"offset,omitempty" jsonschema:"Optional. Rows to skip. Default: 0."`
	Limit         *int   `json:"limit,omitempty" jsonschema:"Optional. Rows to return, at most 1000; 0 means 1000. Default: 20."`
	CountLimit    *int   `json:"countLimit,omitempty" jsonschema:"Optional. -1 counts every match in total, 0 skips the count (total is 0), n caps total at n. Default: -1."`
}

type Output struct {
	Total   int64                        `json:"total" jsonschema:"Matching groups, as counted per countLimit."`
	Offset  int                          `json:"offset"`
	Limit   int                          `json:"limit"`
	Results []clients.DirectoryUserGroup `json:"results"`
}

func NewTool(collibraClient *http.Client) *chip.Tool[Input, Output] {
	return &chip.Tool[Input, Output]{
		Name:  "search_user_groups",
		Title: "Search User Groups",
		Description: "Find Collibra user groups (named sets of users that can be given roles together) by name, or the groups a user belongs to (userId). " +
			"Returns each group's id, name and whether it is a system group, plus total, offset and limit for paging. " +
			"Use it to get the group id that the group tools need, or to check whether a group already exists before creating it. " +
			"At least one filter is required: an unfiltered call is refused. nameMatchMode controls how name is compared (default: the name appears anywhere, ignoring case). " +
			"This returns groups, not their members: list the members with search_users and the group's id as groupId. " +
			"Example user requests: \"Is there a group called Product Owners?\"; \"Which groups is Jane Smith in?\"; \"Find the stewards groups\".",
		Handler:     handler(collibraClient),
		Permissions: []string{},
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, DestructiveHint: chip.Ptr(false), IdempotentHint: true, OpenWorldHint: chip.Ptr(false)},
	}
}

func handler(collibraClient *http.Client) chip.ToolHandlerFunc[Input, Output] {
	return func(ctx context.Context, input Input) (Output, error) {
		if input.Name == "" && input.UserID == "" {
			return Output{}, fmt.Errorf("give at least one filter: name or userId. An unfiltered search of every user group is refused")
		}
		if err := validation.DashedUUIDOptional("userId", input.UserID); err != nil {
			return Output{}, err
		}
		page, err := clients.SearchUserGroups(ctx, collibraClient, clients.SearchUserGroupsRequest{
			Name:            input.Name,
			NameMatchMode:   input.NameMatchMode,
			UserID:          input.UserID,
			DirectoryPaging: clients.DirectoryPaging{Offset: input.Offset, Limit: input.Limit, CountLimit: input.CountLimit},
		})
		if err != nil {
			return Output{}, err
		}
		return Output{Total: page.Total, Offset: page.Offset, Limit: page.Limit, Results: page.Results}, nil
	}
}
