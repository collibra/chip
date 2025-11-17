package clients

import (
	"encoding/json"
	"fmt"
)

// SearchRequest represents the request payload for the Collibra search API
type SearchRequest struct {
	Keywords       string         `json:"keywords"`
	SearchInFields []SearchField  `json:"searchInFields,omitempty"`
	Filters        []SearchFilter `json:"filters,omitempty"`
	Limit          int            `json:"limit"`
	Offset         int            `json:"offset"`
}

type SearchField struct {
	ResourceType string   `json:"resourceType"`
	Fields       []string `json:"fields,omitempty"`
}

type SearchFilter struct {
	Field  string   `json:"field"`
	Values []string `json:"values"`
}

// SearchResponse represents the response from the Collibra search API
type SearchResponse struct {
	Total        int                 `json:"total"`
	Results      []SearchResult      `json:"results"`
	Aggregations []SearchAggregation `json:"aggregations"`
}

type SearchResult struct {
	Resource   SearchResource    `json:"resource"`
	Highlights []SearchHighlight `json:"highlights"`
}

type SearchResource struct {
	ResourceType   string `json:"resourceType"`
	ID             string `json:"id"`
	CreatedBy      string `json:"createdBy"`
	CreatedOn      int64  `json:"createdOn"`
	LastModifiedOn int64  `json:"lastModifiedOn"`
	Name           string `json:"name"`
}

type SearchHighlight struct {
}

type SearchAggregation struct {
	Field  string                   `json:"field"`
	Values []SearchAggregationValue `json:"values"`
}

type SearchAggregationValue struct {
}

func CreateSearchRequest(question string, resourceTypes []string, filters []SearchFilter, limit int, offset int) SearchRequest {
	// Map of allowed resource types and their searchable fields
	allowedTypesWithFields := map[string][]string{
		"Asset":     {"name", "displayName", "comments", "tags", "dataClassification", "attributes"},
		"Domain":    {"name", "comments"},
		"Community": {"name", "comments"},
		"User":      {"name"},
		"UserGroup": {"name"},
	}

	var validatedResourceTypes []string
	for _, t := range resourceTypes {
		if _, ok := allowedTypesWithFields[t]; ok {
			validatedResourceTypes = append(validatedResourceTypes, t)
		}
	}

	searchRequest := SearchRequest{
		Keywords: "*" + question + "*", // Add wildcards for partial matching
		Filters:  filters,
		Limit:    limit,
		Offset:   offset,
	}

	// Only set searchInFields if specific resource types are requested
	// If empty, the API will search across all resource types and all fields
	if len(validatedResourceTypes) > 0 {
		var searchInFields []SearchField
		for _, t := range validatedResourceTypes {
			searchInFields = append(searchInFields, SearchField{
				ResourceType: t,
				Fields:       allowedTypesWithFields[t],
			})
		}
		searchRequest.SearchInFields = searchInFields
	}

	return searchRequest
}

func ParseSearchResponse(jsonData []byte) (*SearchResponse, error) {
	var searchResponse SearchResponse
	if err := json.Unmarshal(jsonData, &searchResponse); err != nil {
		return nil, fmt.Errorf("failed to parse search response: %w", err)
	}

	return &searchResponse, nil
}
