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
	DomainID    string `json:"domainId"`
	TypeID      string `json:"typeId"`
	DisplayName string `json:"displayName,omitempty"`
	StatusID    string `json:"statusId,omitempty"`
}

// CreateDataElementResponse is the response from creating a Data Element asset.
type CreateDataElementResponse struct {
	ID           string                      `json:"id"`
	ResourceType string                      `json:"resourceType"`
	Name         string                      `json:"name"`
	DisplayName  string                      `json:"displayName,omitempty"`
	Domain       *CreateDataElementReference `json:"domain,omitempty"`
	Type         *CreateDataElementReference `json:"type,omitempty"`
	Status       *CreateDataElementReference `json:"status,omitempty"`
	CreatedBy    string                      `json:"createdBy,omitempty"`
	CreatedOn    int64                       `json:"createdOn,omitempty"`
}

// CreateDataElementReference is a reference object returned in the asset response.
type CreateDataElementReference struct {
	ID   string `json:"id"`
	Name string `json:"name,omitempty"`
}

// CreateDataElement creates a new Data Element asset in Collibra via the REST API.
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

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		responseBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(responseBody))
	}

	var result CreateDataElementResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}
	return &result, nil
}
