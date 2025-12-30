package tools

import (
	"context"
	"net/http"

	"github.com/collibra/chip/pkg/chip"
	"github.com/collibra/chip/pkg/clients"
)

type SearchClassificationMatchesInput struct {
	AssetIDs          []string `json:"assetIds,omitempty" jsonschema:"Optional. Filter by asset IDs. The list of asset IDs (with Column types) to filter the search results."`
	Statuses          []string `json:"statuses,omitempty" jsonschema:"Optional. Filter by classification match status. Valid values: ACCEPTED, REJECTED, SUGGESTED."`
	ClassificationIDs []string `json:"classificationIds,omitempty" jsonschema:"Optional. Filter by classification IDs. The list of classification IDs to filter the search results."`
	AssetTypeIDs      []string `json:"assetTypeIds,omitempty" jsonschema:"Optional. Filter by asset type IDs. The list of asset type IDs to filter the search results."`
	Limit             int      `json:"limit,omitempty" jsonschema:"Optional. Maximum number of results to return. The maximum value is 1000. Default: 50."`
	Offset            int      `json:"offset,omitempty" jsonschema:"Optional. Index of first result (pagination offset). Default: 0."`
	CountLimit        int      `json:"countLimit,omitempty" jsonschema:"Optional. Limits the number of elements that will be counted. -1 will count everything, 0 will skip counting. Default: -1."`
}

type SearchClassificationMatchesOutput struct {
	Total                 int                               `json:"total" jsonschema:"Total number of matching classification matches"`
	Count                 int                               `json:"count" jsonschema:"Number of classification matches returned in this response"`
	ClassificationMatches []clients.DataClassificationMatch `json:"classificationMatches" jsonschema:"List of classification matches"`
	Error                 string                            `json:"error,omitempty" jsonschema:"HTTP or other error message if the request failed"`
}

func NewSearchClassificationMatchesTool(collibraClient *http.Client) *chip.Tool[SearchClassificationMatchesInput, SearchClassificationMatchesOutput] {
	return &chip.Tool[SearchClassificationMatchesInput, SearchClassificationMatchesOutput]{
		Name:        "data_classification_match_search",
		Description: "Search for classification matches (associations between data classes and assets) in Collibra. Supports filtering by asset IDs, statuses (ACCEPTED/REJECTED/SUGGESTED), classification IDs, and asset type IDs.",
		Handler:     handleSearchClassificationMatches(collibraClient),
	}
}

func handleSearchClassificationMatches(collibraClient *http.Client) chip.ToolHandlerFunc[SearchClassificationMatchesInput, SearchClassificationMatchesOutput] {
	return func(ctx context.Context, input SearchClassificationMatchesInput) (SearchClassificationMatchesOutput, error) {
		input.sanitizePagination()

		params := buildClassificationMatchQueryParams(input)
		results, total, err := clients.SearchDataClassificationMatches(ctx, collibraClient, params)
		if err != nil {
			return SearchClassificationMatchesOutput{Error: err.Error(), Total: int(total), Count: 0, ClassificationMatches: results}, nil
		}

		if len(results) == 0 {
			return SearchClassificationMatchesOutput{Total: int(total), Count: 0, ClassificationMatches: results}, nil
		}

		return SearchClassificationMatchesOutput{Total: int(total), Count: len(results), ClassificationMatches: results}, nil
	}
}

func (in *SearchClassificationMatchesInput) sanitizePagination() {
	if in.Limit < 0 {
		in.Limit = 0
	}
	if in.Offset < 0 {
		in.Offset = 0
	}
	if in.CountLimit == 0 {
		in.CountLimit = -1
	}
}

func buildClassificationMatchQueryParams(in SearchClassificationMatchesInput) clients.DataClassificationMatchQueryParams {
	params := clients.DataClassificationMatchQueryParams{
		AssetIDs:          in.AssetIDs,
		Statuses:          in.Statuses,
		ClassificationIDs: in.ClassificationIDs,
		AssetTypeIDs:      in.AssetTypeIDs,
	}

	if in.Limit != 0 {
		params.Limit = &in.Limit
	}

	if in.Offset != 0 {
		params.Offset = &in.Offset
	}

	if in.CountLimit != -1 {
		params.CountLimit = &in.CountLimit
	}

	return params
}
