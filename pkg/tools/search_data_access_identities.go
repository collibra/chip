package tools

import (
	"context"
	"fmt"
	"net/http"

	"github.com/collibra/chip/pkg/chip"
	"github.com/collibra/chip/pkg/clients"
)

type SearchDataAccessIdentitiesInput struct {
	Email    string `json:"email,omitempty" jsonschema:"Optional. Exact email address to look up the user by."`
	Name     string `json:"name,omitempty" jsonschema:"Optional. Search string for a case-insensitive contains match on the user's display name. When used without email, SearchUsers is called server-side. When used with email, it is applied as a client-side filter on the result."`
	Cursor   string `json:"cursor,omitempty" jsonschema:"Optional. Cursor from a previous response to fetch the next page of results. Only applicable for name-based searches."`
	PageSize int    `json:"pageSize,omitempty" jsonschema:"Optional. Number of results per page (default: 25, max: 25). Only applicable for name-based searches."`
}

type SearchDataAccessIdentitiesOutput struct {
	Results    []*clients.DataAccessIdentity `json:"results" jsonschema:"The matching Data Access users."`
	NextCursor *string                       `json:"nextCursor,omitempty" jsonschema:"Cursor to pass in the next request to fetch the following page. Only present for name-based searches with more results available."`
	Error      string                        `json:"error,omitempty" jsonschema:"Error message if the search could not be completed."`
}

func NewSearchDataAccessIdentitiesTool(collibraClient *http.Client) *chip.Tool[SearchDataAccessIdentitiesInput, SearchDataAccessIdentitiesOutput] {
	return &chip.Tool[SearchDataAccessIdentitiesInput, SearchDataAccessIdentitiesOutput]{
		Name:        "search_data_access_identities",
		Description: "Search for Data Access users (identities) by name and/or email. Providing email performs an exact lookup; providing name performs a case-insensitive contains search via SearchUsers. Both can be combined: email resolves the user, name filters the result client-side. Name-only searches are paginated — use the returned cursor to fetch subsequent pages.",
		Handler:     handleSearchDataAccessIdentities(collibraClient),
		Permissions: []string{},
	}
}

func handleSearchDataAccessIdentities(collibraClient *http.Client) chip.ToolHandlerFunc[SearchDataAccessIdentitiesInput, SearchDataAccessIdentitiesOutput] {
	return func(ctx context.Context, input SearchDataAccessIdentitiesInput) (SearchDataAccessIdentitiesOutput, error) {
		result, err := clients.SearchDataAccessIdentities(ctx, collibraClient, input.Name, input.Email, input.Cursor, input.PageSize)
		if err != nil {
			return SearchDataAccessIdentitiesOutput{
				Error: fmt.Sprintf("Failed to search Data Access identities: %s", err.Error()),
			}, nil
		}

		return SearchDataAccessIdentitiesOutput{
			Results:    result.Items,
			NextCursor: result.NextCursor,
		}, nil
	}
}
