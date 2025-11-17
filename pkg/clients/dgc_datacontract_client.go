package clients

import (
	"bytes"
	"encoding/json"
	"fmt"
	"mime/multipart"
)

// DataContractListPaginated represents the paginated response from the data contracts API
type DataContractListPaginated struct {
	Items      []DataContract `json:"items"`
	Limit      int            `json:"limit"`
	NextCursor string         `json:"nextCursor,omitempty"`
	Total      int            `json:"total,omitempty"`
}

// DataContract represents metadata attributes of a data contract
type DataContract struct {
	ID         string `json:"id"`
	DomainID   string `json:"domainId"`
	ManifestID string `json:"manifestId"`
}

type DataContractsQueryParams struct {
	ManifestID   string `url:"manifestId,omitempty"`
	IncludeTotal bool   `url:"includeTotal,omitempty"`
	Cursor       string `url:"cursor,omitempty"`
	Limit        int    `url:"limit,omitempty"`
}

// PushDataContractManifestRequest represents the request parameters for pushing a data contract manifest
type PushDataContractManifestRequest struct {
	Manifest   string
	ManifestID string
	Version    string
	Force      bool
	Active     bool
}

// PushDataContractManifestResponse represents the response from pushing a data contract manifest
type PushDataContractManifestResponse struct {
	ID         string `json:"id"`
	DomainID   string `json:"domainId"`
	ManifestID string `json:"manifestId"`
}

func ParseDataContractsResponse(jsonData []byte) (*DataContractListPaginated, error) {
	var response DataContractListPaginated
	if err := json.Unmarshal(jsonData, &response); err != nil {
		return nil, fmt.Errorf("failed to parse data contracts response: %w", err)
	}

	return &response, nil
}

func CreateAddFromManifestRequest(req PushDataContractManifestRequest) (*bytes.Buffer, string, error) {
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	// Add the manifest file with Content-Disposition header
	part, err := writer.CreateFormFile("manifest", "contract.yaml")
	if err != nil {
		return nil, "", fmt.Errorf("failed to create form file: %w", err)
	}
	if _, err := part.Write([]byte(req.Manifest)); err != nil {
		return nil, "", fmt.Errorf("failed to write manifest content: %w", err)
	}

	// Add optional fields
	if req.ManifestID != "" {
		if err := writer.WriteField("manifestId", req.ManifestID); err != nil {
			return nil, "", fmt.Errorf("failed to write manifestId field: %w", err)
		}
	}

	if req.Version != "" {
		if err := writer.WriteField("version", req.Version); err != nil {
			return nil, "", fmt.Errorf("failed to write version field: %w", err)
		}
	}

	if req.Force {
		if err := writer.WriteField("force", "true"); err != nil {
			return nil, "", fmt.Errorf("failed to write force field: %w", err)
		}
	}

	if req.Active {
		if err := writer.WriteField("active", "true"); err != nil {
			return nil, "", fmt.Errorf("failed to write active field: %w", err)
		}
	}

	if err := writer.Close(); err != nil {
		return nil, "", fmt.Errorf("failed to close multipart writer: %w", err)
	}

	return body, writer.FormDataContentType(), nil
}

func ParseAddFromManifestResponse(jsonData []byte) (*PushDataContractManifestResponse, error) {
	var response PushDataContractManifestResponse
	if err := json.Unmarshal(jsonData, &response); err != nil {
		return nil, fmt.Errorf("failed to parse add from manifest response: %w", err)
	}

	return &response, nil
}
