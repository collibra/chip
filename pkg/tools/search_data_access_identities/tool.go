package search_data_access_identities

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/collibra/chip/pkg/chip"
	"github.com/collibra/chip/pkg/clients"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// statusValidationError is returned when the call is rejected before any request is sent.
const statusValidationError = "validation_error"

// maxPageSize caps the results a single call may ask for. The cap keeps the response small
// enough for an agent's context; a larger pageSize would list users beyond it.
const maxPageSize = 25

type SearchDataAccessIdentitiesInput struct {
	Email    string `json:"email,omitempty" jsonschema:"Optional. Exact email address to look up the user by."`
	Name     string `json:"name,omitempty" jsonschema:"Optional. Search string for a case-insensitive contains match on the user's display name. When used without email, ListUsers is called server-side. When used with email, it is applied as a client-side filter on the result."`
	PageSize int    `json:"pageSize,omitempty" jsonschema:"Optional. Maximum number of results to return, between 1 and 25. Omit it for the default of 25; a larger value is rejected. Only applicable for name-based searches — narrow the name instead of asking for a bigger page."`
}

type SearchDataAccessIdentitiesOutput struct {
	Status  string                        `json:"status,omitempty" jsonschema:"validation_error when the call was rejected before any request was sent — see message."`
	Message string                        `json:"message,omitempty" jsonschema:"What is wrong and what a valid call looks like, when status is validation_error."`
	Results []*clients.DataAccessIdentity `json:"results" jsonschema:"The matching Data Access users."`
	Error   string                        `json:"error,omitempty" jsonschema:"Error message if the search could not be completed."`
}

func NewTool(collibraClient *http.Client) *chip.Tool[SearchDataAccessIdentitiesInput, SearchDataAccessIdentitiesOutput] {
	return &chip.Tool[SearchDataAccessIdentitiesInput, SearchDataAccessIdentitiesOutput]{
		Name:        "search_data_access_identities",
		Title:       "Search Data Access Identities",
		Description: "Search for Data Access users (identities) by name and/or email. At least one of name or email is required — an unfiltered call is rejected rather than listing every user on the instance. Providing email performs an exact lookup; providing name performs a case-insensitive contains search via ListUsers. Both can be combined: email resolves the user, name filters the result client-side. Name-only searches return up to pageSize matches (1-25, default 25); a larger pageSize is rejected, so narrow the name instead.",
		Handler:     handleSearchDataAccessIdentities(collibraClient),
		Permissions: []string{"dgc.data-access-view-all-access-and-usage"},
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, DestructiveHint: new(false), IdempotentHint: true, OpenWorldHint: new(false)},
	}
}

func handleSearchDataAccessIdentities(collibraClient *http.Client) chip.ToolHandlerFunc[SearchDataAccessIdentitiesInput, SearchDataAccessIdentitiesOutput] {
	return func(ctx context.Context, input SearchDataAccessIdentitiesInput) (SearchDataAccessIdentitiesOutput, error) {
		name, email := strings.TrimSpace(input.Name), strings.TrimSpace(input.Email)
		if name == "" && email == "" {
			return SearchDataAccessIdentitiesOutput{
				Status:  statusValidationError,
				Message: "Provide at least one filter: email (exact address) and/or name (case-insensitive contains) — an unfiltered search would list every Data Access user on the instance. pageSize does not count as a filter.",
			}, nil
		}

		if input.PageSize < 0 || input.PageSize > maxPageSize {
			return SearchDataAccessIdentitiesOutput{
				Status:  statusValidationError,
				Message: fmt.Sprintf("pageSize %d is out of range — use a value between 1 and %d, or omit it for the default of %d. To find users beyond that, narrow the name filter rather than asking for a bigger page.", input.PageSize, maxPageSize, maxPageSize),
			}, nil
		}

		result, err := clients.SearchDataAccessIdentities(ctx, collibraClient, name, email, input.PageSize)
		if err != nil {
			return SearchDataAccessIdentitiesOutput{
				Error: fmt.Sprintf("Failed to search Data Access identities: %s", err.Error()),
			}, nil
		}

		return SearchDataAccessIdentitiesOutput{
			Results: result.Items,
		}, nil
	}
}
