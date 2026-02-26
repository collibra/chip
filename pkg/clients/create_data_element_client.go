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
	DisplayName  string                     `json:"displayName"`
	ResourceType string                     `json:"resourceType"`
	Type         CreateDataElementTypeRef   `json:"type"`
	Domain       CreateDataElementDomainRef `json:"domain"`
	Status       CreateDataElementStatusRef `json:"status"`
}

// CreateDataElementTypeRef represents the asset type reference in the response.
type CreateDataElementTypeRef struct {
	Name string `json:"name"`
	ID   string `json:"id"`
}

// CreateDataElementDomainRef represents the domain reference in the response.
type CreateDataElementDomainRef struct {
	Name string `json:"name"`
	ID   string `json:"id"`
}

// CreateDataElementStatusRef represents the status reference in the response.
type CreateDataElementStatusRef struct {
	Name string `json:"name"`
	ID   string `json:"id"`
}

// CreateDataElement creates a new Data Element asset in Collibra via the REST API.
func CreateDataElement(ctx context.Context, client *http.Client, params CreateDataElementRequest) (*CreateDataElementResponse, error) {
	body, err := json.Marshal(params)
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
		var errBody map[string]any
		_ = json.NewDecoder(resp.Body).Decode(&errBody)
		if msg, ok := errBody["message"].(string); ok {
			return nil, fmt.Errorf("API error (status %d): %s", resp.StatusCode, msg)
		}
		return nil, fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}

	var result CreateDataElementResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}
	return &result, nil
}
