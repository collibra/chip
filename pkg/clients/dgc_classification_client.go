package clients

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/google/go-querystring/query"
)

type DataClassQueryParams struct {
	ContainsRules    *bool    `url:"containsRules,omitempty"`
	CorrelationID    string   `url:"correlationId,omitempty"`
	DataClassGroupID string   `url:"dataClassGroupId,omitempty"`
	Description      string   `url:"description,omitempty"`
	Limit            *int     `url:"limit,omitempty"`
	Name             string   `url:"name,omitempty"`
	Offset           *int     `url:"offset,omitempty"`
	RuleType         []string `url:"ruleType,omitempty"`
	Status           []string `url:"status,omitempty"`
	View             string   `url:"view,omitempty"`
}

type DataClassesResponse struct {
	Total   int         `json:"total"`
	Results []DataClass `json:"results"`
}

type DataClass struct {
	ID                  string            `json:"id"`
	Name                string            `json:"name"`
	Description         string            `json:"description"`
	Status              string            `json:"status"`
	ColumnNameFilters   []string          `json:"columnNameFilters"`
	ColumnTypeFilters   []string          `json:"columnTypeFilters"`
	AllowNullValues     bool              `json:"allowNullValues"`
	AllowEmptyValues    bool              `json:"allowEmptyValues"`
	ConfidenceThreshold int               `json:"confidenceThreshold"`
	Examples            []string          `json:"examples"`
	CreatedBy           string            `json:"createdBy"`
	CreatedOn           int64             `json:"createdOn"`
	LastModifiedBy      string            `json:"lastModifiedBy"`
	LastModifiedOn      int64             `json:"lastModifiedOn"`
	Rules               []json.RawMessage `json:"rules"`
}

type AddDataClassRequest struct {
	Name                string   `json:"name"`
	Description         string   `json:"description,omitempty"`
	Status              string   `json:"status,omitempty"`
	ColumnNameFilters   []string `json:"columnNameFilters,omitempty"`
	ColumnTypeFilters   []string `json:"columnTypeFilters,omitempty"`
	AllowNullValues     *bool    `json:"allowNullValues,omitempty"`
	AllowEmptyValues    *bool    `json:"allowEmptyValues,omitempty"`
	ConfidenceThreshold *int     `json:"confidenceThreshold,omitempty"`
	Examples            []string `json:"examples,omitempty"`
}

func SearchDataClasses(ctx context.Context, collibraHttpClient *http.Client, params DataClassQueryParams) ([]DataClass, int, error) {
	endpoint, err := dataClassesEndpoint(params)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to build endpoint: %w", err)
	}
	body, _, err := fetchJSON("GET", endpoint, ctx, collibraHttpClient)
	if err != nil {
		return nil, 0, err
	}
	return parseDataClasses(body)
}

func fetchJSON(method string, endpoint string, ctx context.Context, httpClient *http.Client) ([]byte, int, error) {
	req, err := http.NewRequestWithContext(ctx, method, endpoint, nil)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to make request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, fmt.Errorf("failed to read response body: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, resp.StatusCode, fmt.Errorf("http %d: %s", resp.StatusCode, string(body))
	}
	return body, resp.StatusCode, nil
}

func dataClassesEndpoint(params DataClassQueryParams) (string, error) {
	endpoint := "/rest/classification/v1/dataClasses"

	values, err := query.Values(params)
	if err != nil {
		return "", fmt.Errorf("failed to encode query params: %w", err)
	}
	if encoded := values.Encode(); encoded != "" {
		endpoint += "?" + encoded
	}
	return endpoint, nil
}

func parseDataClasses(body []byte) ([]DataClass, int, error) {
	var dcResp DataClassesResponse
	if err := json.Unmarshal(body, &dcResp); err != nil {
		return nil, 0, fmt.Errorf("failed to parse data classes response: %w", err)
	}
	return dcResp.Results, dcResp.Total, nil
}
