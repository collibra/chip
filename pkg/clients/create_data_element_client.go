package clients

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// CreateDataElementRequest is the request body for creating a Data Element asset.
type CreateDataElementRequest struct {
	Name        string `json:"name"`
	DomainId    string `json:"domainId"`
	TypeId      string `json:"typeId"`
	DisplayName string `json:"displayName,omitempty"`
	StatusId    string `json:"statusId,omitempty"`
}

// CreateDataElementResponse is the response from creating a Data Element asset.
type CreateDataElementResponse struct {
	Id           string `json:"id"`
	ResourceType string `json:"resourceType"`
	Name         string `json:"name"`
	DisplayName  string `json:"displayName"`
	CreatedOn    int64  `json:"createdOn"`
	CreatedBy    string `json:"createdBy"`
}

// CreateDataElement creates a new Data Element asset via the Collibra REST API.
func CreateDataElement(ctx context.Context, client *http.Client, reqBody CreateDataElementRequest) (*CreateDataElementResponse, error) {
	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshaling request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "/rest/2.0/assets", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("executing request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response body: %w", err)
	}

	if resp.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(respBody))
	}

	var result CreateDataElementResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}
	return &result, nil
}
