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
	ID           string                     `json:"id"`
	Name         string                     `json:"name"`
	DisplayName  string                     `json:"displayName,omitempty"`
	ResourceType string                     `json:"resourceType"`
	Domain       CreateDataElementRef       `json:"domain,omitempty"`
	Type         CreateDataElementRef       `json:"type,omitempty"`
	Status       *CreateDataElementRef      `json:"status,omitempty"`
	CreatedOn    int64                      `json:"createdOn,omitempty"`
	CreatedBy    string                     `json:"createdBy,omitempty"`
}

// CreateDataElementRef is a reference to a related resource (domain, type, status).
type CreateDataElementRef struct {
	ID   string `json:"id"`
	Name string `json:"name"`
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

	if resp.StatusCode == http.StatusConflict || resp.StatusCode == http.StatusBadRequest {
		return nil, fmt.Errorf("asset creation failed (status %d): a Data Element with this name may already exist in the domain", resp.StatusCode)
	}

	if resp.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}

	var result CreateDataElementResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	return &result, nil
}
