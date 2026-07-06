package clients

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// CommunityDetails is the response shape returned by POST /rest/2.0/communities.
type CommunityDetails struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

// CreateCommunityRequest is the request body for POST /rest/2.0/communities.
type CreateCommunityRequest struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

// CreateCommunity creates a new community via POST /rest/2.0/communities.
func CreateCommunity(ctx context.Context, client *http.Client, request CreateCommunityRequest) (*CommunityDetails, error) {
	var community CommunityDetails
	if err := postJSON(ctx, client, "/rest/2.0/communities", request, http.StatusCreated, &community); err != nil {
		return nil, fmt.Errorf("creating community: %w", err)
	}
	return &community, nil
}

// DomainDetails is the response shape returned by POST /rest/2.0/domains.
type DomainDetails struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	CommunityID string `json:"communityId"`
	TypeID      string `json:"typeId"`
}

// CreateDomainRequest is the request body for POST /rest/2.0/domains.
type CreateDomainRequest struct {
	Name                         string `json:"name"`
	Description                  string `json:"description,omitempty"`
	CommunityID                  string `json:"communityId"`
	TypeID                       string `json:"typeId"`
	ExcludedFromAutoHyperlinking bool   `json:"excludedFromAutoHyperlinking,omitempty"`
}

// CreateDomain creates a new domain via POST /rest/2.0/domains.
func CreateDomain(ctx context.Context, client *http.Client, request CreateDomainRequest) (*DomainDetails, error) {
	var domain DomainDetails
	if err := postJSON(ctx, client, "/rest/2.0/domains", request, http.StatusCreated, &domain); err != nil {
		return nil, fmt.Errorf("creating domain: %w", err)
	}
	return &domain, nil
}

// postJSON is a small shared helper for simple "POST body, decode JSON response"
// calls against DGC's rest/2.0 API family.
func postJSON(ctx context.Context, client *http.Client, endpoint string, request any, expectedStatus int, out any) error {
	body, err := json.Marshal(request)
	if err != nil {
		return fmt.Errorf("marshaling request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("building request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("sending request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("reading response: %w", err)
	}

	if resp.StatusCode != expectedStatus {
		return fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(respBody))
	}

	if err := json.Unmarshal(respBody, out); err != nil {
		return fmt.Errorf("decoding response: %w", err)
	}

	return nil
}
