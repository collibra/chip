package search_data_classes

import (
	"context"
	"net/http"
	"strings"

	"github.com/collibra/chip/pkg/chip"
	"github.com/collibra/chip/pkg/clients"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type Input struct {
	Name          string `json:"name,omitempty" jsonschema:"Optional. Filter by data class name. The name of a Data Class. Matching is case-insensitive and supports partial matches."`
	Description   string `json:"description,omitempty" jsonschema:"Optional. Filter by description. The description of a Data Class. Matching is case-insensitive and supports partial matches."`
	ContainsRules bool   `json:"containsRules,omitempty" jsonschema:"Optional. If true, only data classes that have rules are returned. Filters the Data Classes based on whether or not they contain rules. Example: true."`
	Limit         int    `json:"limit,omitempty" jsonschema:"Optional. Maximum number of results to return. The maximum value is 1000. Default: 50."`
	Offset        int    `json:"offset,omitempty" jsonschema:"Optional. Index of first result (pagination offset). Default: 0."`
}

type Output struct {
	Total       int                 `json:"total" jsonschema:"Total number of matching data classes"`
	Count       int                 `json:"count" jsonschema:"Number of data classes returned in this response"`
	DataClasses []clients.DataClass `json:"dataClasses" jsonschema:"List of data classes"`
	Error       string              `json:"error,omitempty" jsonschema:"HTTP or other error message if the request failed"`
}

func NewTool(collibraClient *http.Client) *chip.Tool[Input, Output] {
	return &chip.Tool[Input, Output]{
		Name:        "search_data_class",
		Description: "Search for data classes in Collibra's classification service. Supports filtering by name, description, and whether they contain rules.",
		Handler:     handler(collibraClient),
		Permissions: []string{"dgc.data-classes-read"},
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}
}

func handler(collibraClient *http.Client) chip.ToolHandlerFunc[Input, Output] {
	return func(ctx context.Context, input Input) (Output, error) {
		input.sanitizePagination()

		params := buildQueryParams(input)
		results, total, err := clients.SearchDataClasses(ctx, collibraClient, params)
		if err != nil {
			return Output{Error: err.Error(), Total: total, Count: 0, DataClasses: results}, nil
		}

		if len(results) == 0 {
			return Output{Total: total, Count: 0, DataClasses: results}, nil
		}

		return Output{Total: total, Count: len(results), DataClasses: results}, nil
	}
}

func (in *Input) sanitizePagination() {
	if in.Limit < 0 {
		in.Limit = 0
	}
	if in.Offset < 0 {
		in.Offset = 0
	}
}

func buildQueryParams(in Input) clients.DataClassQueryParams {

	params := &clients.DataClassQueryParams{
		Description: strings.TrimSpace(in.Description),
		Name:        strings.TrimSpace(in.Name),
	}

	if in.Limit != 0 {
		params.Limit = &in.Limit
	}

	if in.Offset != 0 {
		params.Offset = &in.Offset
	}

	if in.ContainsRules {
		params.ContainsRules = &in.ContainsRules
	}

	return *params
}
