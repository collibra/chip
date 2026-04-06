package clients

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// CreateAssetRequest is the request body for POST /rest/2.0/assets.
type CreateAssetRequest struct {
	Name                         string `json:"name"`
	TypeID                       string `json:"typeId"`
	DomainID                     string `json:"domainId"`
	DisplayName                  string `json:"displayName,omitempty"`
	ExcludeFromAutoHyperlinking  bool   `json:"excludeFromAutoHyperlinking,omitempty"`
}

// CreateAssetResponse is the response from POST /rest/2.0/assets.
type CreateAssetResponse struct {
	ID             string              `json:"id"`
	Name           string              `json:"name"`
	DisplayName    string              `json:"displayName"`
	Type           CreateAssetTypeRef  `json:"type"`
	Domain         CreateAssetDomainRef `json:"domain"`
	CreatedBy      string              `json:"createdBy"`
	CreatedOn      int64               `json:"createdOn"`
	LastModifiedBy string              `json:"lastModifiedBy"`
	LastModifiedOn int64               `json:"lastModifiedOn"`
}

// CreateAssetTypeRef is a reference to an asset type in a create asset response.
type CreateAssetTypeRef struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// CreateAssetDomainRef is a reference to a domain in a create asset response.
type CreateAssetDomainRef struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// CreateAttributeRequest is the request body for POST /rest/2.0/attributes.
type CreateAttributeRequest struct {
	AssetID string `json:"assetId"`
	TypeID  string `json:"typeId"`
	Value   string `json:"value"`
}

// CreateAttributeResponse is the response from POST /rest/2.0/attributes.
type CreateAttributeResponse struct {
	ID    string                    `json:"id"`
	Type  CreateAttributeTypeRef    `json:"type"`
	Asset CreateAttributeAssetRef   `json:"asset"`
	Value string                    `json:"value"`
}

// CreateAttributeTypeRef is a reference to an attribute type.
type CreateAttributeTypeRef struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// CreateAttributeAssetRef is a reference to an asset in an attribute response.
type CreateAttributeAssetRef struct {
	ID string `json:"id"`
}

// CreateAsset creates a new asset via POST /rest/2.0/assets.
func CreateAsset(ctx context.Context, client *http.Client, request CreateAssetRequest) (*CreateAssetResponse, error) {
	body, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("creating asset: marshaling request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "/rest/2.0/assets", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("creating asset: building request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("creating asset: sending request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("creating asset: reading response: %w", err)
	}

	if resp.StatusCode != http.StatusCreated {
		switch resp.StatusCode {
		case http.StatusBadRequest:
			return nil, fmt.Errorf("creating asset: bad request (invalid parameters or duplicate name): %s", string(respBody))
		case http.StatusForbidden:
			return nil, fmt.Errorf("creating asset: asset type not allowed in domain: %s", string(respBody))
		case http.StatusNotFound:
			return nil, fmt.Errorf("creating asset: invalid assetTypeId or domainId: %s", string(respBody))
		default:
			return nil, fmt.Errorf("creating asset: unexpected status %d: %s", resp.StatusCode, string(respBody))
		}
	}

	var result CreateAssetResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("creating asset: decoding response: %w", err)
	}

	return &result, nil
}

// CreateAttribute creates a new attribute on an asset via POST /rest/2.0/attributes.
func CreateAttribute(ctx context.Context, client *http.Client, request CreateAttributeRequest) (*CreateAttributeResponse, error) {
	body, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("creating attribute: marshaling request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "/rest/2.0/attributes", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("creating attribute: building request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("creating attribute: sending request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("creating attribute: reading response: %w", err)
	}

	if resp.StatusCode != http.StatusCreated {
		switch resp.StatusCode {
		case http.StatusBadRequest:
			return nil, fmt.Errorf("creating attribute: bad request (invalid parameters): %s", string(respBody))
		case http.StatusNotFound:
			return nil, fmt.Errorf("creating attribute: asset or attribute type not found: %s", string(respBody))
		default:
			return nil, fmt.Errorf("creating attribute: unexpected status %d: %s", resp.StatusCode, string(respBody))
		}
	}

	var result CreateAttributeResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("creating attribute: decoding response: %w", err)
	}

	return &result, nil
}
