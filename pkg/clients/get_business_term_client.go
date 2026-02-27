package clients

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
)

// Types for the get_business_term tool.

// BusinessTermNamedRef is a reference with an ID and Name used in asset responses.
type BusinessTermNamedRef struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// BusinessTermAssetResponse is the response from GET /rest/2.0/assets/{assetId}.
type BusinessTermAssetResponse struct {
	ID          string                `json:"id"`
	Name        string                `json:"name"`
	DisplayName string                `json:"displayName"`
	Type        BusinessTermNamedRef  `json:"type"`
	Status      BusinessTermNamedRef  `json:"status"`
	Domain      BusinessTermNamedRef  `json:"domain"`
}

// BusinessTermAttributeType is the type metadata for an asset attribute.
type BusinessTermAttributeType struct {
	Name string `json:"name"`
}

// BusinessTermAttributeResponse is a single attribute returned from the attributes endpoint.
type BusinessTermAttributeResponse struct {
	Type  BusinessTermAttributeType `json:"type"`
	Value interface{}               `json:"value"`
}

// BusinessTermRelationAssetType is the type metadata for an asset in a relation.
type BusinessTermRelationAssetType struct {
	Name string `json:"name"`
}

// BusinessTermRelationAsset is an asset reference within a relation.
type BusinessTermRelationAsset struct {
	ID   string                        `json:"id"`
	Name string                        `json:"name"`
	Type BusinessTermRelationAssetType `json:"type"`
}

// BusinessTermRelation is a single relation between two assets.
type BusinessTermRelation struct {
	ID     string                    `json:"id"`
	Type   BusinessTermNamedRef      `json:"type"`
	Source BusinessTermRelationAsset `json:"source"`
	Target BusinessTermRelationAsset `json:"target"`
}

// BusinessTermRelationsResponse is the paginated response from GET /rest/2.0/relations.
type BusinessTermRelationsResponse struct {
	Total   int64                  `json:"total"`
	Offset  int64                  `json:"offset"`
	Limit   int64                  `json:"limit"`
	Results []BusinessTermRelation `json:"results"`
}

// GetBusinessTermAsset retrieves an asset by ID from the Collibra REST API.
func GetBusinessTermAsset(ctx context.Context, client *http.Client, assetID string) (*BusinessTermAssetResponse, error) {
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

	var result BusinessTermAssetResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}
	return &result, nil
}

// BusinessTermAttributesResponse is the paginated response from GET /rest/2.0/attributes.
type BusinessTermAttributesResponse struct {
	Total   int64                          `json:"total"`
	Offset  int64                          `json:"offset"`
	Limit   int64                          `json:"limit"`
	Results []BusinessTermAttributeResponse `json:"results"`
}

// GetBusinessTermAttributes retrieves the attributes of an asset by ID.
func GetBusinessTermAttributes(ctx context.Context, client *http.Client, assetID string) ([]BusinessTermAttributeResponse, error) {
	q := url.Values{}
	q.Set("assetId", assetID)
	q.Set("limit", "1000")

	reqURL := "/rest/2.0/attributes?" + q.Encode()
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

	var result BusinessTermAttributesResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}
	return result.Results, nil
}

// GetBusinessTermRelations retrieves relations where the given asset is the source.
func GetBusinessTermRelations(ctx context.Context, client *http.Client, sourceID string) (*BusinessTermRelationsResponse, error) {
	q := url.Values{}
	q.Set("sourceId", sourceID)
	q.Set("limit", "1000")

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

	var result BusinessTermRelationsResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}
	return &result, nil
}
