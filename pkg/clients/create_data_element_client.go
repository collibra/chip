package clients

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// DataElementAssetTypeID is the Collibra asset type UUID for Data Element.
const DataElementAssetTypeID = "00000000-0000-0000-0000-000000031302"

// CreateDataElementParams holds the parameters for creating a Data Element asset.
type CreateDataElementParams struct {
	Name        string `json:"name"`
	DomainID    string `json:"domainId"`
	DisplayName string `json:"displayName,omitempty"`
	StatusID    string `json:"statusId,omitempty"`
}

// createDataElementRequest is the full request body sent to the Collibra API.
type createDataElementRequest struct {
	Name        string `json:"name"`
	DomainID    string `json:"domainId"`
	TypeID      string `json:"typeId"`
	DisplayName string `json:"displayName,omitempty"`
	StatusID    string `json:"statusId,omitempty"`
}

// CreateDataElementResponse holds the response from creating a Data Element asset.
type CreateDataElementResponse struct {
	ID           string `json:"id"`
	ResourceType string `json:"resourceType"`
}

// CreateDataElement creates a new Data Element asset in Collibra.
func CreateDataElement(ctx context.Context, client *http.Client, params CreateDataElementParams) (*CreateDataElementResponse, error) {
	reqBody := createDataElementRequest{
		Name:        params.Name,
		DomainID:    params.DomainID,
		TypeID:      DataElementAssetTypeID,
		DisplayName: params.DisplayName,
		StatusID:    params.StatusID,
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshalling create data element request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "/rest/2.0/assets", bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("creating create data element request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("executing create data element request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading create data element response: %w", err)
	}

	if resp.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf("create data element failed with status %d: %s", resp.StatusCode, string(respBody))
	}

	var result CreateDataElementResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("unmarshalling create data element response: %w", err)
	}

	return &result, nil
}
