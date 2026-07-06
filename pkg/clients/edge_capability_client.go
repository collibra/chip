package clients

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// Capability is the response shape returned by the Edge capability management API.
type Capability struct {
	ID           string         `json:"id"`
	Type         any            `json:"type,omitempty"`
	Name         string         `json:"name"`
	Description  string         `json:"description,omitempty"`
	EdgeSiteID   string         `json:"edgeSiteId"`
	EdgeSiteName string         `json:"edgeSiteName,omitempty"`
	Parameters   map[string]any `json:"parameters"`
}

// CapabilityRequest is the request body for creating or updating a capability via
// POST /edge/api/rest/v2/capabilities or PUT /edge/api/rest/v2/capabilities/{id}.
type CapabilityRequest struct {
	TypeID      string         `json:"typeId"`
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	EdgeSiteID  string         `json:"edgeSiteId"`
	Parameters  map[string]any `json:"parameters"`
}

// CreateCapability creates a new Edge capability via POST /edge/api/rest/v2/capabilities.
// The server assigns the capability id.
func CreateCapability(ctx context.Context, client *http.Client, request CapabilityRequest) (*Capability, error) {
	return doCapabilityRequest(ctx, client, http.MethodPost, "/edge/api/rest/v2/capabilities", request)
}

// CreateOrUpdateCapability creates or updates an Edge capability with a known id via
// PUT /edge/api/rest/v2/capabilities/{id}.
func CreateOrUpdateCapability(ctx context.Context, client *http.Client, capabilityID string, request CapabilityRequest) (*Capability, error) {
	endpoint := "/edge/api/rest/v2/capabilities/" + capabilityID
	return doCapabilityRequest(ctx, client, http.MethodPut, endpoint, request)
}

func doCapabilityRequest(ctx context.Context, client *http.Client, method, endpoint string, request CapabilityRequest) (*Capability, error) {
	body, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("saving capability: marshaling request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, method, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("saving capability: building request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("saving capability: sending request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("saving capability: reading response: %w", err)
	}

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		switch resp.StatusCode {
		case http.StatusBadRequest:
			return nil, fmt.Errorf("saving capability: bad request (invalid parameters or typeId): %s", string(respBody))
		case http.StatusNotFound:
			return nil, fmt.Errorf("saving capability: edge site not found: %s", string(respBody))
		default:
			return nil, fmt.Errorf("saving capability: unexpected status %d: %s", resp.StatusCode, string(respBody))
		}
	}

	var result Capability
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("saving capability: decoding response: %w", err)
	}

	return &result, nil
}
