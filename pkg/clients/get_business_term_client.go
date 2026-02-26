package clients

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
)

// GetBusinessTermAssetResponse represents the response from GET /rest/2.0/assets/{assetId}.
type GetBusinessTermAssetResponse struct {
	ID           string                  `json:"id"`
	Name         string                  `json:"name"`
	DisplayName  string                  `json:"displayName"`
	Type         GetBusinessTermNamedRef `json:"type"`
	Status       GetBusinessTermNamedRef `json:"status"`
	Domain       GetBusinessTermNamedRef `json:"domain"`
	ResourceType string                  `json:"resourceType"`
}

// GetBusinessTermNamedRef represents a named reference with an ID and Name.
type GetBusinessTermNamedRef struct {
	Name string `json:"name"`
	ID   string `json:"id"`
}

// GetBusinessTermRelationsResponse represents the response from GET /rest/2.0/relations.
type GetBusinessTermRelationsResponse struct {
	Results []GetBusinessTermRelation `json:"results"`
	Total   int                      `json:"total"`
	Limit   int                      `json:"limit"`
	Offset  int                      `json:"offset"`
}

// GetBusinessTermRelation represents a single relation between two assets.
type GetBusinessTermRelation struct {
	ID     string                      `json:"id"`
	Source GetBusinessTermRelationAsset `json:"source"`
	Target GetBusinessTermRelationAsset `json:"target"`
	Type   GetBusinessTermNamedRef     `json:"type"`
}

// GetBusinessTermRelationAsset represents an asset referenced in a relation.
type GetBusinessTermRelationAsset struct {
	ID   string                  `json:"id"`
	Name string                  `json:"name"`
	Type GetBusinessTermNamedRef `json:"type"`
}

// GetBusinessTermAsset retrieves a Collibra asset by ID using the REST API.
func GetBusinessTermAsset(ctx context.Context, client *http.Client, assetID string) (*GetBusinessTermAssetResponse, error) {
	reqURL := fmt.Sprintf("/rest/2.0/assets/%s", url.PathEscape(assetID))

	req, err := http.NewRequestWithContext(ctx, "GET", reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("executing request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("asset not found: %s", assetID)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}

	var result GetBusinessTermAssetResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}
	return &result, nil
}

// GetBusinessTermRelations retrieves relations from a source asset using the REST API.
func GetBusinessTermRelations(ctx context.Context, client *http.Client, sourceID string) (*GetBusinessTermRelationsResponse, error) {
	q := url.Values{}
	q.Set("sourceId", sourceID)
	q.Set("limit", "1000")
	q.Set("offset", "0")

	reqURL := "/rest/2.0/relations?" + q.Encode()

	req, err := http.NewRequestWithContext(ctx, "GET", reqURL, nil)
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

	var result GetBusinessTermRelationsResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}
	return &result, nil
}
