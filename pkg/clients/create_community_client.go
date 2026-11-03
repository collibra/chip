package clients

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

// CreateCommunityRequest is the request body for POST /rest/2.0/communities.
type CreateCommunityRequest struct {
	Name        string `json:"name"`
	ParentID    string `json:"parentId,omitempty"`
	Description string `json:"description,omitempty"`
}

// CreateCommunityResponse is the response from POST /rest/2.0/communities.
type CreateCommunityResponse struct {
	ID          string      `json:"id"`
	Name        string      `json:"name"`
	Description string      `json:"description,omitempty"`
	Parent      *NamedIDRef `json:"parent,omitempty"`
}

// NamedIDRef is a {id, name} reference to another resource, as nested in
// create responses.
type NamedIDRef struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// CreateCommunity creates a new community via POST /rest/2.0/communities.
func CreateCommunity(ctx context.Context, client *http.Client, request CreateCommunityRequest) (*CreateCommunityResponse, error) {
	var result CreateCommunityResponse
	if err := postJSON(ctx, client, "/rest/2.0/communities", request, &result); err != nil {
		return nil, fmt.Errorf("creating community: %w", err)
	}
	return &result, nil
}

// postJSON POSTs body as JSON and decodes the response into out. Non-2xx
// responses surface Collibra's error envelope via executeCollibraRequest.
func postJSON(ctx context.Context, client *http.Client, path string, body, out any) error {
	payload, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshaling request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, path, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("building request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	respBody, err := executeCollibraRequest(client, req)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(respBody, out); err != nil {
		return fmt.Errorf("decoding response: %w", err)
	}
	return nil
}
