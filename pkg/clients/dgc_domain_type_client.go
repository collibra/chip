package clients

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

// DomainType is a DGC domain type, as returned by GET /rest/2.0/domainTypes.
type DomainTypeDetails struct {
	ID          string `json:"id"`
	Name        string `json:"name,omitempty"`
	Description string `json:"description,omitempty"`
	PublicID    string `json:"publicId,omitempty"`
}

// DomainTypePagedResponse is the response shape for GET /rest/2.0/domainTypes.
type DomainTypePagedResponse struct {
	Total   int64               `json:"total"`
	Offset  int64               `json:"offset"`
	Limit   int64               `json:"limit"`
	Results []DomainTypeDetails `json:"results"`
}

// FindDomainTypesQueryParams are the supported query parameters for
// GET /rest/2.0/domainTypes. Name matches case-insensitively as a substring by
// default (nameMatchMode=ANYWHERE server-side).
type FindDomainTypesQueryParams struct {
	Name   string `url:"name,omitempty"`
	Limit  int    `url:"limit,omitempty"`
	Offset int    `url:"offset,omitempty"`
}

// FindDomainTypes searches DGC domain types via GET /rest/2.0/domainTypes.
func FindDomainTypes(ctx context.Context, client *http.Client, params FindDomainTypesQueryParams) (*DomainTypePagedResponse, error) {
	endpoint, err := buildUrl("/rest/2.0/domainTypes", params)
	if err != nil {
		return nil, fmt.Errorf("finding domain types: building endpoint: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("finding domain types: building request: %w", err)
	}

	body, err := executeRequest(client, req)
	if err != nil {
		return nil, fmt.Errorf("finding domain types: %w", err)
	}

	var result DomainTypePagedResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("finding domain types: decoding response: %w", err)
	}
	return &result, nil
}
