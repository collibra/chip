package tools

import (
	"context"
	"net/http"
	"time"

	"github.com/collibra/chip/pkg/chip"
	clients2 "github.com/collibra/chip/pkg/clients"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type KeywordSearchInput struct {
	Query               string   `json:"query" jsonschema:"Required. The keyword query to search for."`
	Limit               int      `json:"limit,omitempty" jsonschema:"Optional. Maximum number of results to return. The maximum value is 1000. Default: 50."`
	Offset              int      `json:"offset,omitempty" jsonschema:"Optional. Index of first result (pagination offset). Default: 0."`
	ResourceTypeFilters []string `json:"resourceTypeFilters,omitempty" jsonschema:"Optional. Restrict search results to the specified resource types across all of their fields. Supported values: Asset, Domain, Community, User, UserGroup. Default: all resource types are searched"`
	CommunityFilter     []string `json:"communityFilter,omitempty" jsonschema:"Optional. Filter by resources within the specified community UUIDs."`
	DomainFilter        []string `json:"domainFilter,omitempty" jsonschema:"Optional. Filter by resources within the specified domain UUIDs."`
	DomainTypeFilter    []string `json:"domainTypeFilter,omitempty" jsonschema:"Optional. Filter by resources with the specified domain type UUIDs."`
	AssetTypeFilter     []string `json:"assetTypeFilter,omitempty" jsonschema:"Optional. Filter by resources with the specified asset type UUIDs."`
	StatusFilter        []string `json:"statusFilter,omitempty" jsonschema:"Optional. Filter by resources with the specified status UUIDs."`
	CreatedByFilter     []string `json:"createdByFilter,omitempty" jsonschema:"Optional. Filter by resources created by the specified user UUIDs."`
}

type KeywordSearchOutput struct {
	Total   int                     `json:"total" jsonschema:"The total number of results available matching the search criteria"`
	Results []KeywordSearchResource `json:"results" jsonschema:"The list of search results"`
}

type KeywordSearchResource struct {
	ResourceType   string `json:"resourceType" jsonschema:"The type of the resource (e.g., Asset, Domain, Community, User, UserGroup)"`
	ID             string `json:"id" jsonschema:"The unique identifier of the resource"`
	CreatedBy      string `json:"createdBy" jsonschema:"The user who created the resource"`
	CreatedOn      string `json:"createdOn" jsonschema:"The timestamp when the resource was created (human-readable format)"`
	LastModifiedOn string `json:"lastModifiedOn" jsonschema:"The timestamp when the resource was last modified (human-readable format)"`
	Name           string `json:"name" jsonschema:"The name of the resource"`
}

func NewKeywordSearchTool() *chip.CollibraTool[KeywordSearchInput, KeywordSearchOutput] {
	return &chip.CollibraTool[KeywordSearchInput, KeywordSearchOutput]{
		Tool: &mcp.Tool{
			Name:        "keywordSearch",
			Description: "Perform a wildcard keyword search for asset names in the Collibra knowledge graph.",
		},
		ToolHandler: handleKeywordSearch,
	}
}

func handleKeywordSearch(ctx context.Context, collibraHttpClient *http.Client, input KeywordSearchInput) (KeywordSearchOutput, error) {
	if input.Limit == 0 {
		input.Limit = 50
	}

	filters := buildSearchFilters(input)

	searchResponse, err := clients2.KeywordSearch(ctx, collibraHttpClient, input.Query, input.ResourceTypeFilters, filters, input.Limit, input.Offset)
	if err != nil {
		return KeywordSearchOutput{}, err
	}

	output := mapSearchResponseToOutput(searchResponse)

	return output, nil
}

func buildSearchFilters(input KeywordSearchInput) []clients2.SearchFilter {
	var searchFilters []clients2.SearchFilter

	if len(input.CommunityFilter) > 0 {
		searchFilters = append(searchFilters, clients2.SearchFilter{
			Field:  "community",
			Values: input.CommunityFilter,
		})
	}

	if len(input.DomainFilter) > 0 {
		searchFilters = append(searchFilters, clients2.SearchFilter{
			Field:  "domain",
			Values: input.DomainFilter,
		})
	}

	if len(input.DomainTypeFilter) > 0 {
		searchFilters = append(searchFilters, clients2.SearchFilter{
			Field:  "domainType",
			Values: input.DomainTypeFilter,
		})
	}

	if len(input.AssetTypeFilter) > 0 {
		searchFilters = append(searchFilters, clients2.SearchFilter{
			Field:  "assetType",
			Values: input.AssetTypeFilter,
		})
	}

	if len(input.StatusFilter) > 0 {
		searchFilters = append(searchFilters, clients2.SearchFilter{
			Field:  "status",
			Values: input.StatusFilter,
		})
	}

	if len(input.CreatedByFilter) > 0 {
		searchFilters = append(searchFilters, clients2.SearchFilter{
			Field:  "createdBy",
			Values: input.CreatedByFilter,
		})
	}

	return searchFilters
}

func formatTimestamp(milliseconds int64) string {
	seconds := milliseconds / 1000
	t := time.Unix(seconds, 0)
	return t.Format(time.RFC3339)
}

func mapSearchResponseToOutput(searchResponse *clients2.SearchResponse) KeywordSearchOutput {
	resources := make([]KeywordSearchResource, len(searchResponse.Results))
	for i, result := range searchResponse.Results {
		resources[i] = KeywordSearchResource{
			ResourceType:   result.Resource.ResourceType,
			ID:             result.Resource.ID,
			CreatedBy:      result.Resource.CreatedBy,
			CreatedOn:      formatTimestamp(result.Resource.CreatedOn),
			LastModifiedOn: formatTimestamp(result.Resource.LastModifiedOn),
			Name:           result.Resource.Name,
		}
	}

	return KeywordSearchOutput{
		Total:   searchResponse.Total,
		Results: resources,
	}
}
