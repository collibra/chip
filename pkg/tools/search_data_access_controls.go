package tools

import (
	"context"
	"fmt"
	"net/http"

	"github.com/collibra/chip/pkg/chip"
	"github.com/collibra/chip/pkg/clients"
)

type SearchDataAccessControlsInput struct {
	Name     string   `json:"name,omitempty" jsonschema:"Optional. Filter by name (case-insensitive contains match)."`
	Actions  []string `json:"actions,omitempty" jsonschema:"Optional. Filter by one or more action types. Valid values: Grant, Mask, Filter, Share, Group, FilterRule."`
	States   []string `json:"states,omitempty" jsonschema:"Optional. Filter by one or more states. Valid values: Active, Inactive, Deleted."`
	Cursor   string   `json:"cursor,omitempty" jsonschema:"Optional. Cursor from a previous response to fetch the next page of results."`
	PageSize int      `json:"pageSize,omitempty" jsonschema:"Optional. Number of results per page (default: 25, max: 25)."`
}

type SearchDataAccessRolesInput struct {
	Name     string   `json:"name,omitempty" jsonschema:"Optional. Filter by name (case-insensitive contains match)."`
	States   []string `json:"states,omitempty" jsonschema:"Optional. Filter by one or more states. Valid values: Active, Inactive, Deleted."`
	Cursor   string   `json:"cursor,omitempty" jsonschema:"Optional. Cursor from a previous response to fetch the next page of results."`
	PageSize int      `json:"pageSize,omitempty" jsonschema:"Optional. Number of results per page (default: 25, max: 25)."`
}

type SearchDataAccessControlsOutput struct {
	Results    []*clients.DataAccessControlDetails `json:"results" jsonschema:"The matching data access controls."`
	NextCursor *string                             `json:"nextCursor,omitempty" jsonschema:"Cursor to pass in the next request to fetch the following page. Absent when there are no more results."`
	Error      string                              `json:"error,omitempty" jsonschema:"Error message if the search could not be completed."`
}

func NewSearchDataAccessControlsTool(collibraClient *http.Client) *chip.Tool[SearchDataAccessControlsInput, SearchDataAccessControlsOutput] {
	return &chip.Tool[SearchDataAccessControlsInput, SearchDataAccessControlsOutput]{
		Name:        "search_data_access_controls",
		Description: "Search for data access controls in Collibra Data Access. Results can be filtered by name (case-insensitive contains), action type (Grant, Mask, Filter, Share, Group, FilterRule), and/or state (Active, Inactive, Deleted). All filters are optional and can be combined. Returns a paginated list — use the returned cursor to fetch subsequent pages.",
		Handler:     handleSearchDataAccessControls(collibraClient),
		Permissions: []string{},
	}
}

func handleSearchDataAccessControls(collibraClient *http.Client) chip.ToolHandlerFunc[SearchDataAccessControlsInput, SearchDataAccessControlsOutput] {
	return func(ctx context.Context, input SearchDataAccessControlsInput) (SearchDataAccessControlsOutput, error) {
		result, err := clients.SearchDataAccessControls(ctx, collibraClient, input.Name, input.Actions, input.States, input.Cursor, input.PageSize)
		if err != nil {
			return SearchDataAccessControlsOutput{
				Error: fmt.Sprintf("Failed to search data access controls: %s", err.Error()),
			}, nil
		}

		return SearchDataAccessControlsOutput{
			Results:    result.Items,
			NextCursor: result.NextCursor,
		}, nil
	}
}

func NewSearchDataAccessRolesTool(collibraClient *http.Client) *chip.Tool[SearchDataAccessRolesInput, SearchDataAccessControlsOutput] {
	return &chip.Tool[SearchDataAccessRolesInput, SearchDataAccessControlsOutput]{
		Name:        "search_data_access_roles",
		Description: "Search for data access roles (Grant-type access controls) in Collibra Data Access. Results can be filtered by name (case-insensitive contains) and/or state (Active, Inactive, Deleted). All filters are optional and can be combined. Returns a paginated list — use the returned cursor to fetch subsequent pages.",
		Handler:     handleSearchDataAccessRoles(collibraClient),
		Permissions: []string{},
	}
}

func handleSearchDataAccessRoles(collibraClient *http.Client) chip.ToolHandlerFunc[SearchDataAccessRolesInput, SearchDataAccessControlsOutput] {
	return func(ctx context.Context, input SearchDataAccessRolesInput) (SearchDataAccessControlsOutput, error) {
		result, err := clients.SearchDataAccessControls(ctx, collibraClient, input.Name, []string{"Grant"}, input.States, input.Cursor, input.PageSize)
		if err != nil {
			return SearchDataAccessControlsOutput{
				Error: fmt.Sprintf("Failed to search data access roles: %s", err.Error()),
			}, nil
		}

		return SearchDataAccessControlsOutput{
			Results:    result.Items,
			NextCursor: result.NextCursor,
		}, nil
	}
}
