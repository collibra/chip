package clients

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/google/go-querystring/query"
)

type AddDataClassificationMatchRequest struct {
	AssetID          string `json:"assetId"`
	ClassificationID string `json:"classificationId"`
}
type DataClassificationMatch struct {
	ID             string                 `json:"id"`
	CreatedBy      string                 `json:"createdBy"`
	CreatedOn      int64                  `json:"createdOn"`
	LastModifiedBy string                 `json:"lastModifiedBy"`
	LastModifiedOn int64                  `json:"lastModifiedOn"`
	System         bool                   `json:"system"`
	ResourceType   string                 `json:"resourceType"`
	Status         string                 `json:"status"`
	Confidence     float64                `json:"confidence"`
	Asset          NamedResourceReference `json:"asset"`
	Classification DataClassification     `json:"classification"`
}
type NamedResourceReference struct {
	ID                    string `json:"id"`
	ResourceType          string `json:"resourceType"`
	ResourceDiscriminator string `json:"resourceDiscriminator,omitempty"`
	Name                  string `json:"name"`
}
type DataClassification struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}
type DataClassificationMatchQueryParams struct {
	Offset            *int     `url:"offset,omitempty"`
	Limit             *int     `url:"limit,omitempty"`
	CountLimit        *int     `url:"countLimit,omitempty"`
	AssetIDs          []string `url:"assetIds,omitempty"`
	Statuses          []string `url:"statuses,omitempty"`
	ClassificationIDs []string `url:"classificationIds,omitempty"`
	AssetTypeIDs      []string `url:"assetTypeIds,omitempty"`
}
type PagedResponseDataClassificationMatch struct {
	Total   int64                     `json:"total"`
	Offset  int64                     `json:"offset"`
	Limit   int64                     `json:"limit"`
	Results []DataClassificationMatch `json:"results"`
}

func AddDataClassificationMatch(ctx context.Context, httpClient *http.Client, request AddDataClassificationMatchRequest) (*DataClassificationMatch, error) {
	endpoint := "/rest/catalog/1.0/dataClassification/classificationMatches"

	jsonData, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to make request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode == 404 {
		return nil, fmt.Errorf("classification or asset not found (HTTP 404): %s", string(body))
	}

	if resp.StatusCode == 422 {
		return nil, fmt.Errorf("classification match already exists between this asset and classification (HTTP 422): %s", string(body))
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("http %d: %s", resp.StatusCode, string(body))
	}

	var match DataClassificationMatch
	if err := json.Unmarshal(body, &match); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &match, nil
}

func SearchDataClassificationMatches(ctx context.Context, httpClient *http.Client, params DataClassificationMatchQueryParams) ([]DataClassificationMatch, int64, error) {
	endpoint := "/rest/catalog/1.0/dataClassification/classificationMatches/bulk"
	values, err := query.Values(params)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to encode query params: %w", err)
	}
	if encoded := values.Encode(); encoded != "" {
		endpoint += "?" + encoded
	}
	req, err := http.NewRequestWithContext(ctx, "GET", endpoint, nil)

	if err != nil {
		return nil, 0, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Accept", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to make request: %w", err)
	}

	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, 0, fmt.Errorf("http %d: %s", resp.StatusCode, string(body))
	}
	var pagedResponse PagedResponseDataClassificationMatch

	if err := json.Unmarshal(body, &pagedResponse); err != nil {
		return nil, 0, fmt.Errorf("failed to parse response: %w", err)
	}
	return pagedResponse.Results, pagedResponse.Total, nil
}

func RemoveDataClassificationMatch(ctx context.Context, httpClient *http.Client, classificationMatchID string) error {
	// REF: https://developer.collibra.com/api/rest/catalog-classification#/operations/removeClassificationMatch
	endpoint := fmt.Sprintf("/rest/catalog/1.0/dataClassification/classificationMatches/%s", classificationMatchID)

	req, err := http.NewRequestWithContext(ctx, "DELETE", endpoint, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Accept", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to make request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == 404 {
		return fmt.Errorf("classification match not found")
	}

	if resp.StatusCode != 204 {
		body, _ := io.ReadAll(resp.Body)
		if len(body) > 0 {
			return fmt.Errorf("%s (HTTP %d)", string(body), resp.StatusCode)
		}
		return fmt.Errorf("unexpected response (HTTP %d)", resp.StatusCode)
	}

	return nil
}
