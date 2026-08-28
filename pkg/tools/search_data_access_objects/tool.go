package search_data_access_objects

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
// enough for an agent's context; a larger pageSize would page through the instance.
const maxPageSize = 25

type SearchDataAccessObjectsInput struct {
	Name           string   `json:"name,omitempty" jsonschema:"Optional. Filter by name (case-insensitive contains match on data object name)."`
	DataSources    []string `json:"dataSources,omitempty" jsonschema:"Optional. Restrict to data objects belonging to one or more data sources (data source IDs)."`
	Types          []string `json:"types,omitempty" jsonschema:"Optional. Restrict to data objects of one or more types (e.g. table, column, schema, view)."`
	Parents        []string `json:"parents,omitempty" jsonschema:"Optional. Restrict to data objects whose direct parent matches one of the given data object IDs."`
	Ancestors      []string `json:"ancestors,omitempty" jsonschema:"Optional. Restrict to data objects whose ancestors include one of the given data object IDs."`
	IncludeDeleted bool     `json:"includeDeleted,omitempty" jsonschema:"Optional. If true, also includes data objects that no longer exist in the source. Defaults to false."`
	PageSize       int      `json:"pageSize,omitempty" jsonschema:"Optional. Maximum number of results to return, between 1 and 25. Omit it for the default of 25; a larger value is rejected. Narrow the filters instead of asking for a bigger page."`
}

type SearchDataAccessObjectsOutput struct {
	Status  string                      `json:"status,omitempty" jsonschema:"validation_error when the call was rejected before any request was sent — see message."`
	Message string                      `json:"message,omitempty" jsonschema:"What is wrong and what a valid call looks like, when status is validation_error."`
	Results []*clients.DataAccessObject `json:"results" jsonschema:"The matching data objects."`
	Error   string                      `json:"error,omitempty" jsonschema:"Error message if the search could not be completed."`
}

func NewTool(collibraClient *http.Client) *chip.Tool[SearchDataAccessObjectsInput, SearchDataAccessObjectsOutput] {
	return &chip.Tool[SearchDataAccessObjectsInput, SearchDataAccessObjectsOutput]{
		Name:        "search_data_access_objects",
		Title:       "Search Data Access Objects",
		Description: "Search for data objects in Collibra Data Access. Data objects represent tables, columns, schemas, views, and other entities tracked in registered data sources. At least one of name, dataSources, types, parents or ancestors is required — an unfiltered call is rejected rather than scanning every data object on the instance. Filters can be combined: name (case-insensitive contains), dataSources (data source IDs), types (e.g. table, column), parents/ancestors (other data object IDs), and includeDeleted (not a filter on its own). Returns up to pageSize matches (1-25, default 25); a larger pageSize is rejected, so narrow the filters instead. Each result also includes its applicablePermissions — the source-system permissions (with name and description) that can be requested on the object.",
		Handler:     handleSearchDataAccessObjects(collibraClient),
		Permissions: []string{"dgc.data-access-view-all-access-and-usage"},
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, DestructiveHint: new(false), IdempotentHint: true, OpenWorldHint: new(false)},
	}
}

func handleSearchDataAccessObjects(collibraClient *http.Client) chip.ToolHandlerFunc[SearchDataAccessObjectsInput, SearchDataAccessObjectsOutput] {
	return func(ctx context.Context, input SearchDataAccessObjectsInput) (SearchDataAccessObjectsOutput, error) {
		if !hasAnyFilter(input) {
			return SearchDataAccessObjectsOutput{
				Status:  statusValidationError,
				Message: "Provide at least one filter (name, dataSources, types, parents, or ancestors) — an unfiltered search would page through every data object on the instance. includeDeleted and pageSize do not count as filters.",
			}, nil
		}

		if input.PageSize < 0 || input.PageSize > maxPageSize {
			return SearchDataAccessObjectsOutput{
				Status:  statusValidationError,
				Message: fmt.Sprintf("pageSize %d is out of range — use a value between 1 and %d, or omit it for the default of %d. To see more objects, narrow the filters rather than asking for a bigger page.", input.PageSize, maxPageSize, maxPageSize),
			}, nil
		}

		result, err := clients.SearchDataAccessObjects(ctx, collibraClient, input.Name, input.DataSources, input.Types, input.Parents, input.Ancestors, input.IncludeDeleted, input.PageSize)
		if err != nil {
			return SearchDataAccessObjectsOutput{
				Error: fmt.Sprintf("Failed to search data access objects: %s", err.Error()),
			}, nil
		}

		return SearchDataAccessObjectsOutput{
			Results: result.Items,
		}, nil
	}
}

// hasAnyFilter reports whether the call narrows the search at all. includeDeleted only widens
// the result set and pageSize just caps it, so neither counts as a filter.
func hasAnyFilter(input SearchDataAccessObjectsInput) bool {
	return strings.TrimSpace(input.Name) != "" ||
		len(input.DataSources) > 0 ||
		len(input.Types) > 0 ||
		len(input.Parents) > 0 ||
		len(input.Ancestors) > 0
}
