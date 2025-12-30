package tools

import (
	"context"
	"net/http"

	"github.com/collibra/chip/pkg/chip"
	"github.com/collibra/chip/pkg/clients"
)

type ListDataContractsInput struct {
	ManifestFilter string `json:"manifestId,omitempty" jsonschema:"Optional. Filter by the unique identifier of the Data Contract manifest."`
	Cursor         string `json:"cursor,omitempty" jsonschema:"Optional. The cursor pointing to the first resource to be included in the response. This cursor must have been extracted from a previous API call response."`
	Limit          int    `json:"limit,omitempty" jsonschema:"Optional. Maximum number of results to return. The maximum allowed limit is 500. Default: 100."`
}

type ListDataContractsOutput struct {
	Total      *int           `json:"total,omitempty" jsonschema:"The total number of data contracts available matching the search criteria (only included if includeTotal was true)"`
	Limit      int            `json:"limit" jsonschema:"The maximum number of results returned"`
	NextCursor string         `json:"nextCursor,omitempty" jsonschema:"The cursor pointing to the next page. If missing, there are no additional pages available."`
	Contracts  []DataContract `json:"contracts" jsonschema:"The list of data contracts"`
}

type DataContract struct {
	ID         string `json:"id" jsonschema:"The UUID of the data contract asset"`
	DomainID   string `json:"domainId" jsonschema:"The UUID of the domain where the data contract asset is located"`
	ManifestID string `json:"manifestId" jsonschema:"The unique identifier of the data contract manifest"`
}

func NewListDataContractsTool(collibraClient *http.Client) *chip.Tool[ListDataContractsInput, ListDataContractsOutput] {
	return &chip.Tool[ListDataContractsInput, ListDataContractsOutput]{
		Name:        "data_contract_list",
		Description: "List data contracts available in Collibra. Returns a paginated list of data contract metadata, sorted by the last modified date in descending order.",
		Handler:     handleListDataContracts(collibraClient),
	}
}

func handleListDataContracts(collibraClient *http.Client) chip.ToolHandlerFunc[ListDataContractsInput, ListDataContractsOutput] {
	return func(ctx context.Context, input ListDataContractsInput) (ListDataContractsOutput, error) {
		if input.Limit == 0 {
			input.Limit = 100
		}

		response, err := clients.ListDataContracts(ctx, collibraClient, input.Cursor, input.Limit, input.ManifestFilter)
		if err != nil {
			return ListDataContractsOutput{}, err
		}

		contracts := make([]DataContract, len(response.Items))
		for i, dc := range response.Items {
			contracts[i] = DataContract{
				ID:         dc.ID,
				DomainID:   dc.DomainID,
				ManifestID: dc.ManifestID,
			}
		}

		output := ListDataContractsOutput{
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
