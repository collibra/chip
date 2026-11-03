package clients

import (
	"context"
	"fmt"
	"net/http"
)

// CreateDomainRequest is the request body for POST /rest/2.0/domains.
type CreateDomainRequest struct {
	Name        string `json:"name"`
	CommunityID string `json:"communityId"`
	TypeID      string `json:"typeId"`
	Description string `json:"description,omitempty"`
}

// CreateDomainResponse is the response from POST /rest/2.0/domains.
type CreateDomainResponse struct {
	ID          string      `json:"id"`
	Name        string      `json:"name"`
	Description string      `json:"description,omitempty"`
	Type        *NamedIDRef `json:"type,omitempty"`
	Community   *NamedIDRef `json:"community,omitempty"`
}

// CreateDomain creates a new domain via POST /rest/2.0/domains.
func CreateDomain(ctx context.Context, client *http.Client, request CreateDomainRequest) (*CreateDomainResponse, error) {
	var result CreateDomainResponse
	if err := postJSON(ctx, client, "/rest/2.0/domains", request, &result); err != nil {
		return nil, fmt.Errorf("creating domain: %w", err)
	}
	return &result, nil
}
