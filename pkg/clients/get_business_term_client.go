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
	ID          string                        `json:"id"`
	Name        string                        `json:"name"`
	DisplayName string                        `json:"displayName"`
	Type        GetBusinessTermAssetType      `json:"type"`
	Status      GetBusinessTermAssetStatus    `json:"status"`
	Domain      GetBusinessTermAssetDomain    `json:"domain"`
}

// GetBusinessTermAssetType represents the type field of an asset.
type GetBusinessTermAssetType struct {
	Name string `json:"name"`
}

// GetBusinessTermAssetStatus represents the status field of an asset.
type GetBusinessTermAssetStatus struct {
	Name string `json:"name"`
}

// GetBusinessTermAssetDomain represents the domain field of an asset.
type GetBusinessTermAssetDomain struct {
	Name string `json:"name"`
}

// GetBusinessTermRelationsResponse represents the response from GET /rest/2.0/relations.
type GetBusinessTermRelationsResponse struct {
	Results []GetBusinessTermRelation `json:"results"`
	Total   int                      `json:"total"`
}

// GetBusinessTermRelation represents a single relation between two assets.
type GetBusinessTermRelation struct {
	Source GetBusinessTermRelationAsset `json:"source"`
	Target GetBusinessTermRelationAsset `json:"target"`
	Type   GetBusinessTermRelationType  `json:"type"`
}

// GetBusinessTermRelationAsset represents an asset within a relation.
type GetBusinessTermRelationAsset struct {
	ID   string                   `json:"id"`
	Name string                   `json:"name"`
	Type GetBusinessTermAssetType `json:"type"`
}

// GetBusinessTermRelationType represents the type of a relation.
type GetBusinessTermRelationType struct {
	Name string `json:"name"`
}

// GetBusinessTermAsset fetches a Collibra asset by ID using GET /rest/2.0/assets/{assetId}.
func GetBusinessTermAsset(ctx context.Context, client *http.Client, assetID string) (*GetBusinessTermAssetResponse, error) {
	path := fmt.Sprintf("/rest/2.0/assets/%s", url.PathEscape(assetID))

	req, err := http.NewRequestWithContext(ctx, "GET", path, nil)
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
		return nil, fmt.Errorf("Business Term not found")
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

// GetBusinessTermRelations fetches relations for a source asset using GET /rest/2.0/relations.
func GetBusinessTermRelations(ctx context.Context, client *http.Client, sourceID string) (*GetBusinessTermRelationsResponse, error) {
	q := url.Values{}
	q.Set("sourceId", sourceID)
	q.Set("limit", "1000")
	q.Set("offset", "0")

	req, err := http.NewRequestWithContext(ctx, "GET", "/rest/2.0/relations?"+q.Encode(), nil)
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
