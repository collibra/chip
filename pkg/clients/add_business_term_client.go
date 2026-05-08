package clients

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// AddBusinessTermAssetRequest is the request body for creating a business term asset.
type AddBusinessTermAssetRequest struct {
	Name         string `json:"name"`
	TypePublicId string `json:"typePublicId"`
	DomainId     string `json:"domainId"`
}

// AddBusinessTermAssetResponse is the response from creating a business term asset.
type AddBusinessTermAssetResponse struct {
	Id string `json:"id"`
}

// AddBusinessTermAttributeRequest is the request body for adding an attribute to an asset.
type AddBusinessTermAttributeRequest struct {
	AssetId string `json:"assetId"`
	TypeId  string `json:"typeId"`
	Value   string `json:"value"`
}

// AddBusinessTermAttributeResponse is the response from adding an attribute to an asset.
type AddBusinessTermAttributeResponse struct {
	Id string `json:"id"`
}

// CreateBusinessTermAsset creates a new business term asset via POST /rest/2.0/assets.
func CreateBusinessTermAsset(ctx context.Context, client *http.Client, req AddBusinessTermAssetRequest) (*AddBusinessTermAssetResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshaling business term asset request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, "/rest/2.0/assets", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("creating business term asset request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json")

	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("creating business term asset: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusCreated {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("creating business term asset: unexpected status %d: %s", resp.StatusCode, string(respBody))
	}

	var result AddBusinessTermAssetResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decoding business term asset response: %w", err)
	}

	return &result, nil
}

// CreateBusinessTermAttribute adds an attribute to an asset via POST /rest/2.0/attributes.
func CreateBusinessTermAttribute(ctx context.Context, client *http.Client, req AddBusinessTermAttributeRequest) (*AddBusinessTermAttributeResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshaling business term attribute request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, "/rest/2.0/attributes", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("creating business term attribute request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json")

	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("creating business term attribute: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusCreated {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("creating business term attribute: unexpected status %d: %s", resp.StatusCode, string(respBody))
	}

	var result AddBusinessTermAttributeResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decoding business term attribute response: %w", err)
	}

	return &result, nil
}
