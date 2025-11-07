package tools

import (
	"context"
	"net/http"
	"strings"

	"github.com/collibra/chip/pkg/chip"
	"github.com/collibra/chip/pkg/clients"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type FindDataClassesInput struct {
	Name          string `json:"name,omitempty" jsonschema:"Optional. Filter by data class name. The name of a Data Class. Matching is case-insensitive and supports partial matches."`
	Description   string `json:"description,omitempty" jsonschema:"Optional. Filter by description. The description of a Data Class. Matching is case-insensitive and supports partial matches."`
	ContainsRules bool   `json:"containsRules,omitempty" jsonschema:"Optional. If true, only data classes that have rules are returned. Filters the Data Classes based on whether or not they contain rules. Example: true."`
	Limit         int    `json:"limit,omitempty" jsonschema:"Optional. Maximum number of results to return. The maximum value is 1000. Default: 50."`
	Offset        int    `json:"offset,omitempty" jsonschema:"Optional. Index of first result (pagination offset). Default: 0."`
}

type FindDataClassesOutput struct {
	Total       int                 `json:"total" jsonschema:"Total number of matching data classes"`
	Count       int                 `json:"count" jsonschema:"Number of data classes returned in this response"`
	DataClasses []clients.DataClass `json:"dataClasses" jsonschema:"List of data classes"`
	Error       string              `json:"error,omitempty" jsonschema:"HTTP or other error message if the request failed"`
}

func NewFindDataClassesTool() *chip.CollibraTool[FindDataClassesInput, FindDataClassesOutput] {
	return &chip.CollibraTool[FindDataClassesInput, FindDataClassesOutput]{
		Tool: &mcp.Tool{
			Name:        "findDataClasses",
			Description: "Search for data classes in Collibra's classification service using flexible filters such as name, description, rule types, status, containsRules, and limit.",
		},
		ToolHandler: handleFindDataClasses,
	}
}

func handleFindDataClasses(ctx context.Context, collibraHttpClient *http.Client, input FindDataClassesInput) (FindDataClassesOutput, error) {
	input.sanitizePagination()

	params := buildQueryParams(input)
	results, total, err := clients.FindDataClasses(ctx, collibraHttpClient, params)
	if err != nil {
		return FindDataClassesOutput{Error: err.Error(), Total: total, Count: 0, DataClasses: results}, nil
	}

	if len(results) == 0 {
		return FindDataClassesOutput{Total: total, Count: 0, DataClasses: results}, nil
	}

	return FindDataClassesOutput{Total: total, Count: len(results), DataClasses: results}, nil
}

func (in *FindDataClassesInput) sanitizePagination() {
	if in.Limit < 0 {
		in.Limit = 0
	}
	if in.Offset < 0 {
		in.Offset = 0
	}
}

func buildQueryParams(in FindDataClassesInput) clients.DataClassQueryParams {

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
