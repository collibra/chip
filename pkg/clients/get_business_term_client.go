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
	ID          string                   `json:"id"`
	Name        string                   `json:"name"`
	DisplayName string                   `json:"displayName"`
	Type        GetBusinessTermAssetType `json:"type"`
	Domain      GetBusinessTermDomain    `json:"domain"`
	Status      GetBusinessTermStatus    `json:"status"`
}

// GetBusinessTermAssetType represents an asset type reference.
type GetBusinessTermAssetType struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// GetBusinessTermDomain represents a domain reference.
type GetBusinessTermDomain struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// GetBusinessTermStatus represents a status reference.
type GetBusinessTermStatus struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// GetBusinessTermAttributesResponse represents the paginated response from GET /rest/2.0/attributes.
type GetBusinessTermAttributesResponse struct {
	Total   int                                `json:"total"`
	Results []GetBusinessTermAttributeResponse `json:"results"`
}

// GetBusinessTermAttributeResponse represents a single attribute from GET /rest/2.0/attributes.
type GetBusinessTermAttributeResponse struct {
	ID    string                       `json:"id"`
	Type  GetBusinessTermAttributeType `json:"type"`
	Value string                       `json:"value"`
}

// GetBusinessTermAttributeType represents an attribute type reference.
type GetBusinessTermAttributeType struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// GetBusinessTermRelationsResponse represents the paginated response from GET /rest/2.0/relations.
type GetBusinessTermRelationsResponse struct {
	Total   int                        `json:"total"`
	Results []GetBusinessTermRelation `json:"results"`
}

// GetBusinessTermRelation represents a single relation between assets.
type GetBusinessTermRelation struct {
	ID     string                        `json:"id"`
	Source GetBusinessTermRelationAsset   `json:"source"`
	Target GetBusinessTermRelationAsset   `json:"target"`
	Type   GetBusinessTermRelationType   `json:"type"`
}

// GetBusinessTermRelationAsset represents an asset reference within a relation.
type GetBusinessTermRelationAsset struct {
	ID   string                   `json:"id"`
	Name string                   `json:"name"`
	Type GetBusinessTermAssetType `json:"type"`
}

// GetBusinessTermRelationType represents a relation type reference.
type GetBusinessTermRelationType struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// GetBusinessTermRelationsParams holds query parameters for the relations endpoint.
type GetBusinessTermRelationsParams struct {
	SourceID string
	TargetID string
	Limit    int
	Offset   int
}

// GetBusinessTermAsset retrieves a single asset by ID from the Collibra REST API.
func GetBusinessTermAsset(ctx context.Context, client *http.Client, assetID string) (*GetBusinessTermAssetResponse, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", fmt.Sprintf("/rest/2.0/assets/%s", url.PathEscape(assetID)), nil)
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

// GetBusinessTermAttributes retrieves all attributes for an asset by ID.
func GetBusinessTermAttributes(ctx context.Context, client *http.Client, assetID string) ([]GetBusinessTermAttributeResponse, error) {
	q := url.Values{}
	q.Set("assetId", assetID)

	req, err := http.NewRequestWithContext(ctx, "GET", "/rest/2.0/attributes?"+q.Encode(), nil)
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

	var result GetBusinessTermAttributesResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}
	return result.Results, nil
}

// GetBusinessTermRelations retrieves relations filtered by source/target asset IDs.
func GetBusinessTermRelations(ctx context.Context, client *http.Client, params GetBusinessTermRelationsParams) (*GetBusinessTermRelationsResponse, error) {
	q := url.Values{}
	if params.SourceID != "" {
		q.Set("sourceId", params.SourceID)
	}
	if params.TargetID != "" {
		q.Set("targetId", params.TargetID)
	}
	if params.Limit > 0 {
		q.Set("limit", fmt.Sprintf("%d", params.Limit))
	}
	if params.Offset > 0 {
		q.Set("offset", fmt.Sprintf("%d", params.Offset))
	}

	reqURL := "/rest/2.0/relations"
	if len(q) > 0 {
		reqURL += "?" + q.Encode()
	}

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
