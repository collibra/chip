package list_data_contracts

import (
	"context"
	"net/http"

	"github.com/collibra/chip/pkg/chip"
	"github.com/collibra/chip/pkg/clients"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type Input struct {
	ManifestFilter string `json:"manifestId,omitempty" jsonschema:"Optional. Filter by the unique identifier of the Data Contract manifest."`
	Cursor         string `json:"cursor,omitempty" jsonschema:"Optional. The cursor pointing to the first resource to be included in the response. This cursor must have been extracted from a previous API call response."`
	Limit          int    `json:"limit,omitempty" jsonschema:"Optional. Maximum number of results to return. The maximum allowed limit is 500. Default: 100."`
}

type Output struct {
	Total      *int       `json:"total,omitempty" jsonschema:"The total number of data contracts available matching the search criteria (only included if includeTotal was true)"`
	Limit      int        `json:"limit" jsonschema:"The maximum number of results returned"`
	NextCursor string     `json:"nextCursor,omitempty" jsonschema:"The cursor pointing to the next page. If missing, there are no additional pages available."`
	Contracts  []Contract `json:"contracts" jsonschema:"The list of data contracts"`
}

type Contract struct {
	ID         string `json:"id" jsonschema:"The UUID of the data contract asset"`
	DomainID   string `json:"domainId" jsonschema:"The UUID of the domain where the data contract asset is located"`
	ManifestID string `json:"manifestId" jsonschema:"The unique identifier of the data contract manifest"`
}

func NewTool(collibraClient *http.Client) *chip.Tool[Input, Output] {
	return &chip.Tool[Input, Output]{
		Name:        "list_data_contract",
		Description: "List data contracts available in Collibra. Returns a paginated list of data contract metadata, sorted by the last modified date in descending order.",
		Handler:     handler(collibraClient),
		Permissions: []string{},
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}
}

func handler(collibraClient *http.Client) chip.ToolHandlerFunc[Input, Output] {
	return func(ctx context.Context, input Input) (Output, error) {
		if input.Limit == 0 {
			input.Limit = 100
		}

		response, err := clients.ListDataContracts(ctx, collibraClient, input.Cursor, input.Limit, input.ManifestFilter)
		if err != nil {
			return Output{}, err
		}

		contracts := make([]Contract, len(response.Items))
		for i, dc := range response.Items {
			contracts[i] = Contract{
				ID:         dc.ID,
				DomainID:   dc.DomainID,
				ManifestID: dc.ManifestID,
			}
		}

		output := Output{
			Limit:      response.Limit,
			NextCursor: response.NextCursor,
			Contracts:  contracts,
		}
		if response.Total > 0 {
			output.Total = &response.Total
		}
		return output, nil
	}
}
