package search_data_access_objects

import (
	"context"
	"fmt"
	"net/http"

	"github.com/collibra/chip/pkg/chip"
	"github.com/collibra/chip/pkg/clients"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type SearchDataAccessObjectsInput struct {
	Name           string   `json:"name,omitempty" jsonschema:"Optional. Filter by name (case-insensitive contains match on data object name)."`
	DataSources    []string `json:"dataSources,omitempty" jsonschema:"Optional. Restrict to data objects belonging to one or more data sources (data source IDs)."`
	Types          []string `json:"types,omitempty" jsonschema:"Optional. Restrict to data objects of one or more types (e.g. table, column, schema, view)."`
	Parents        []string `json:"parents,omitempty" jsonschema:"Optional. Restrict to data objects whose direct parent matches one of the given data object IDs."`
	Ancestors      []string `json:"ancestors,omitempty" jsonschema:"Optional. Restrict to data objects whose ancestors include one of the given data object IDs."`
	IncludeDeleted bool     `json:"includeDeleted,omitempty" jsonschema:"Optional. If true, also includes data objects that no longer exist in the source. Defaults to false."`
	PageSize       int      `json:"pageSize,omitempty" jsonschema:"Optional. Maximum number of results to return (default: 25, max: 25)."`
}

type SearchDataAccessObjectsOutput struct {
	Results []*clients.DataAccessObject `json:"results" jsonschema:"The matching data objects."`
	Error   string                      `json:"error,omitempty" jsonschema:"Error message if the search could not be completed."`
}

func NewTool(collibraClient *http.Client) *chip.Tool[SearchDataAccessObjectsInput, SearchDataAccessObjectsOutput] {
	return &chip.Tool[SearchDataAccessObjectsInput, SearchDataAccessObjectsOutput]{
		Name:        "search_data_access_objects",
		Description: "Search for data objects in Collibra Data Access. Data objects represent tables, columns, schemas, views, and other entities tracked in registered data sources. Filters can be combined: name (case-insensitive contains), dataSources (data source IDs), types (e.g. table, column), parents/ancestors (other data object IDs), and includeDeleted. Returns up to pageSize matches (default 25, max 25). Each result also includes its applicablePermissions — the source-system permissions (with name and description) that can be requested on the object.",
		Handler:     handleSearchDataAccessObjects(collibraClient),
		Permissions: []string{},
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, DestructiveHint: new(false)},
	}
}

func handleSearchDataAccessObjects(collibraClient *http.Client) chip.ToolHandlerFunc[SearchDataAccessObjectsInput, SearchDataAccessObjectsOutput] {
	return func(ctx context.Context, input SearchDataAccessObjectsInput) (SearchDataAccessObjectsOutput, error) {
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
