package clients

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

// DataElementAssetTypeID is the Collibra asset type UUID for Data Element.
const DataElementAssetTypeID = "00000000-0000-0000-0000-000000031302"

// CreateDataElementRequest represents the request body for creating a data element asset.
type CreateDataElementRequest struct {
	Name        string `json:"name"`
	DomainID    string `json:"domainId"`
	TypeID      string `json:"typeId"`
	DisplayName string `json:"displayName,omitempty"`
	StatusID    string `json:"statusId,omitempty"`
}

// CreateDataElementResponse represents the response from creating a data element asset.
type CreateDataElementResponse struct {
	ID           string                       `json:"id"`
	ResourceType string                       `json:"resourceType"`
	Name         string                       `json:"name"`
	DisplayName  string                       `json:"displayName"`
	Domain       CreateDataElementDomainRef   `json:"domain"`
	Type         CreateDataElementTypeRef     `json:"type"`
	Status       CreateDataElementStatusRef   `json:"status"`
}

// CreateDataElementDomainRef is a reference to the domain of the created asset.
type CreateDataElementDomainRef struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// CreateDataElementTypeRef is a reference to the type of the created asset.
type CreateDataElementTypeRef struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// CreateDataElementStatusRef is a reference to the status of the created asset.
type CreateDataElementStatusRef struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// CreateDataElement creates a new Data Element asset in the specified Collibra domain.
func CreateDataElement(ctx context.Context, client *http.Client, reqBody CreateDataElementRequest) (*CreateDataElementResponse, error) {
	reqBody.TypeID = DataElementAssetTypeID

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
		var errBody struct {
			Message string `json:"message"`
		}
		if decErr := json.NewDecoder(resp.Body).Decode(&errBody); decErr == nil && errBody.Message != "" {
			return nil, fmt.Errorf("API error (HTTP %d): %s", resp.StatusCode, errBody.Message)
		}
		return nil, fmt.Errorf("API error: HTTP %d", resp.StatusCode)
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
