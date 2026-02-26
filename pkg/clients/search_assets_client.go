package clients

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
)

// SearchAssetsParams holds the query parameters for searching Collibra assets.
type SearchAssetsParams struct {
	Name          string
	NameMatchMode string
	DomainID      string
	TypeIDs       []string
	Offset        int32
	Limit         int32
	SortField     string
	SortOrder     string
}

// SearchAssetsResponse is the paginated response from the assets search endpoint.
type SearchAssetsResponse struct {
	Total   int                  `json:"total"`
	Offset  int                  `json:"offset"`
	Limit   int                  `json:"limit"`
	Results []SearchAssetsResult `json:"results"`
}

// SearchAssetsStatusField represents the status object returned by the Collibra API.
type SearchAssetsStatusField struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// SearchAssetsResult represents a single asset returned from the search.
type SearchAssetsResult struct {
	ID          string                  `json:"id"`
	Name        string                  `json:"name"`
	DisplayName string                  `json:"displayName"`
	DomainID    string                  `json:"domainId"`
	TypeID      string                  `json:"typeId"`
	Status      SearchAssetsStatusField `json:"status"`
}

// SearchAssets searches for Collibra assets using the REST API.
func SearchAssets(ctx context.Context, client *http.Client, params SearchAssetsParams) (*SearchAssetsResponse, error) {
	q := url.Values{}

	if params.Name != "" {
		q.Set("name", params.Name)
	}
	if params.NameMatchMode != "" {
		q.Set("nameMatchMode", params.NameMatchMode)
	}
	if params.DomainID != "" {
		q.Set("domainId", params.DomainID)
	}
	for _, typeID := range params.TypeIDs {
		q.Add("typeIds", typeID)
	}
	if params.Offset > 0 {
		q.Set("offset", strconv.FormatInt(int64(params.Offset), 10))
	}
	if params.Limit > 0 {
		q.Set("limit", strconv.FormatInt(int64(params.Limit), 10))
	}

	sortField := params.SortField
	if sortField == "" {
		sortField = "NAME"
	}
	q.Set("sortField", sortField)

	if params.SortOrder != "" {
		q.Set("sortOrder", params.SortOrder)
	}

	req, err := http.NewRequestWithContext(ctx, "GET", "/rest/2.0/assets?"+q.Encode(), nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("executing request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}

	var result SearchAssetsResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}
	return &result, nil
}
