package clients

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

// CreateDataElementRequest is the request body for creating a Data Element asset.
type CreateDataElementRequest struct {
	Name        string `json:"name"`
	DomainID    string `json:"domainId"`
	TypeID      string `json:"typeId"`
	DisplayName string `json:"displayName,omitempty"`
	StatusID    string `json:"statusId,omitempty"`
}

// CreateDataElementResponse is the response from creating a Data Element asset.
type CreateDataElementResponse struct {
	ID           string                       `json:"id"`
	ResourceType string                       `json:"resourceType"`
	Name         string                       `json:"name"`
	DisplayName  string                       `json:"displayName"`
	Domain       CreateDataElementReference   `json:"domain"`
	Type         CreateDataElementReference   `json:"type"`
	Status       CreateDataElementReference   `json:"status"`
	CreatedBy    string                       `json:"createdBy"`
	CreatedOn    int64                        `json:"createdOn"`
}

// CreateDataElementReference is a reference object returned in the create response.
type CreateDataElementReference struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// CreateDataElementErrorResponse represents an error response from the API.
type CreateDataElementErrorResponse struct {
	ErrorCode string `json:"errorCode"`
	Message   string `json:"message"`
}

// CreateDataElement creates a new Data Element asset via the Collibra REST API.
func CreateDataElement(ctx context.Context, client *http.Client, request CreateDataElementRequest) (*CreateDataElementResponse, error) {
	body, err := json.Marshal(request)
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

	if resp.StatusCode != http.StatusCreated {
		var errResp CreateDataElementErrorResponse
		if err := json.NewDecoder(resp.Body).Decode(&errResp); err == nil && errResp.Message != "" {
			return nil, fmt.Errorf("API error (status %d): %s", resp.StatusCode, errResp.Message)
		}
		return nil, fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}

	var result CreateDataElementResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}
	return &result, nil
}
