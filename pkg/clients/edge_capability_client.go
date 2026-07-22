package clients

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

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
func CreateCapability(ctx context.Context, client *http.Client, request CapabilityRequest) (*EdgeCapability, error) {
	return doCapabilityRequest(ctx, client, http.MethodPost, "/edge/api/rest/v2/capabilities", request)
}

// CreateOrUpdateCapability creates or updates an Edge capability with a known id via
// PUT /edge/api/rest/v2/capabilities/{id}.
func CreateOrUpdateCapability(ctx context.Context, client *http.Client, capabilityID string, request CapabilityRequest) (*EdgeCapability, error) {
	endpoint := "/edge/api/rest/v2/capabilities/" + capabilityID
	return doCapabilityRequest(ctx, client, http.MethodPut, endpoint, request)
}

func doCapabilityRequest(ctx context.Context, client *http.Client, method, endpoint string, request CapabilityRequest) (*EdgeCapability, error) {
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

	var result EdgeCapability
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("saving capability: decoding response: %w", err)
	}

	return &result, nil
}

// EdgeCapability is the full capability resource returned by the list/get/find
// endpoints. It is distinct from Capability (the create/update request-response
// shape) because these read endpoints expose the capability type as a structured
// object whose id is needed to filter integrations by platform.
type EdgeCapability struct {
	Id           string              `json:"id,omitempty"`
	Name         string              `json:"name,omitempty"`
	Description  string              `json:"description,omitempty"`
	EdgeSiteId   string              `json:"edgeSiteId,omitempty"`
	EdgeSiteName string              `json:"edgeSiteName,omitempty"`
	Type         *EdgeCapabilityType `json:"type,omitempty"`
	Parameters   map[string]any      `json:"parameters,omitempty"`
}

type EdgeCapabilityType struct {
	Id string `json:"id,omitempty"`
}

type CapabilityFindRequest struct {
	EdgeSiteId string            `json:"edgeSiteId,omitempty"`
	Labels     map[string]string `json:"labels,omitempty"`
	Parameters map[string]string `json:"parameters,omitempty"`
}

type CapabilityRunRequest struct {
	JobId           string         `json:"jobId,omitempty"`
	InFastNamespace bool           `json:"inFastNamespace,omitempty"`
	Parameters      map[string]any `json:"parameters,omitempty"`
	WorkflowName    string         `json:"workflowName,omitempty"`
}

// ListCapabilities lists all Edge capabilities via GET /edge/api/rest/v2/capabilities.
func ListCapabilities(ctx context.Context, client *http.Client) ([]EdgeCapability, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", "/edge/api/rest/v2/capabilities", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	body, err := executeRequest(client, req)
	if err != nil {
		return nil, err
	}
	var capabilities []EdgeCapability
	if err := json.Unmarshal(body, &capabilities); err != nil {
		return nil, fmt.Errorf("failed to parse capabilities response: %w", err)
	}
	return capabilities, nil
}

// FindCapabilities searches Edge capabilities via POST /edge/api/rest/v2/capabilities/find.
func FindCapabilities(ctx context.Context, client *http.Client, reqBody CapabilityFindRequest) ([]EdgeCapability, error) {
	data, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, "POST", "/edge/api/rest/v2/capabilities/find", bytes.NewBuffer(data))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	body, err := executeRequest(client, req)
	if err != nil {
		return nil, err
	}
	var caps []EdgeCapability
	if err := json.Unmarshal(body, &caps); err != nil {
		return nil, fmt.Errorf("failed to parse capabilities response: %w", err)
	}
	return caps, nil
}

// GetCapability fetches a single Edge capability via GET /edge/api/rest/v2/capabilities/{id}.
func GetCapability(ctx context.Context, client *http.Client, id string) (*EdgeCapability, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", "/edge/api/rest/v2/capabilities/"+id, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	body, err := executeRequest(client, req)
	if err != nil {
		return nil, err
	}
	var cap EdgeCapability
	if err := json.Unmarshal(body, &cap); err != nil {
		return nil, fmt.Errorf("failed to parse capability response: %w", err)
	}
	return &cap, nil
}

// DeleteCapability deletes an Edge capability via DELETE /edge/api/rest/v2/capabilities/{id}.
func DeleteCapability(ctx context.Context, client *http.Client, id string) error {
	req, err := http.NewRequestWithContext(ctx, "DELETE", "/edge/api/rest/v2/capabilities/"+id, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	_, err = executeRequest(client, req)
	return err
}

// RunCapability triggers an Edge capability run via POST /edge/api/rest/v2/capabilities/{id}/run
// and returns the resulting job id.
func RunCapability(ctx context.Context, client *http.Client, id string, reqBody CapabilityRunRequest) (string, error) {
	data, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, "POST", "/edge/api/rest/v2/capabilities/"+id+"/run", bytes.NewBuffer(data))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}
	body, err := executeRequest(client, req)
	if err != nil {
		return "", err
	}
	var jobId string
	if err := json.Unmarshal(body, &jobId); err != nil {
		return "", fmt.Errorf("failed to parse run response: %w", err)
	}
	return jobId, nil
}
