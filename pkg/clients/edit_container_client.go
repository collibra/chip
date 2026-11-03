package clients

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
)

// ContainerRef is a reference to a community or domain (id + name).
type ContainerRef struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// DomainDetails is the slim view of a domain used by edit_domain and the name resolver.
type DomainDetails struct {
	ID          string        `json:"id"`
	Name        string        `json:"name"`
	Description string        `json:"description,omitempty"`
	Community   *ContainerRef `json:"community,omitempty"`
}

// CommunityDetails is the slim view of a community used by edit_community and the
// name resolver. Parent is nil for a top-level community.
type CommunityDetails struct {
	ID          string        `json:"id"`
	Name        string        `json:"name"`
	Description string        `json:"description,omitempty"`
	Parent      *ContainerRef `json:"parent,omitempty"`
}

// DomainPatch is the body for PATCH /rest/2.0/domains/{id}. Nil fields are
// left unchanged by Collibra.
type DomainPatch struct {
	Name        *string `json:"name,omitempty"`
	Description *string `json:"description,omitempty"`
	CommunityID *string `json:"communityId,omitempty"`
}

// CommunityPatch is the body for PATCH /rest/2.0/communities/{id}. Nil fields
// are left unchanged by Collibra. ClearParent sends an explicit
// "parentId": null, which the API is supposed to treat as "make top-level".
type CommunityPatch struct {
	Name        *string
	Description *string
	ParentID    *string
	ClearParent bool
}

// MarshalJSON emits only the fields that are set; ClearParent forces an
// explicit null parentId, which omitempty on a pointer could not express.
func (p CommunityPatch) MarshalJSON() ([]byte, error) {
	body := map[string]any{}
	if p.Name != nil {
		body["name"] = *p.Name
	}
	if p.Description != nil {
		body["description"] = *p.Description
	}
	if p.ParentID != nil {
		body["parentId"] = *p.ParentID
	} else if p.ClearParent {
		body["parentId"] = nil
	}
	return json.Marshal(body)
}

type domainList struct {
	Results []DomainDetails `json:"results"`
}

type communityList struct {
	Results []CommunityDetails `json:"results"`
}

// GetDomain fetches a domain via GET /rest/2.0/domains/{id}.
func GetDomain(ctx context.Context, client *http.Client, domainID string) (*DomainDetails, error) {
	var out DomainDetails
	if err := getJSON(ctx, client, "/rest/2.0/domains/"+url.PathEscape(domainID), &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// SearchDomains returns domains whose name contains the given text (the API's
// substring match); callers narrow to an exact match.
func SearchDomains(ctx context.Context, client *http.Client, name string) ([]DomainDetails, error) {
	var out domainList
	if err := getJSON(ctx, client, "/rest/2.0/domains?limit=50&name="+url.QueryEscape(name), &out); err != nil {
		return nil, err
	}
	return out.Results, nil
}

// GetCommunity fetches a community via GET /rest/2.0/communities/{id}.
func GetCommunity(ctx context.Context, client *http.Client, communityID string) (*CommunityDetails, error) {
	var out CommunityDetails
	if err := getJSON(ctx, client, "/rest/2.0/communities/"+url.PathEscape(communityID), &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// SearchCommunities returns communities whose name contains the given text
// (the API's substring match); callers narrow to an exact match.
func SearchCommunities(ctx context.Context, client *http.Client, name string) ([]CommunityDetails, error) {
	var out communityList
	if err := getJSON(ctx, client, "/rest/2.0/communities?limit=50&name="+url.QueryEscape(name), &out); err != nil {
		return nil, err
	}
	return out.Results, nil
}

// PatchDomain applies a partial update via PATCH /rest/2.0/domains/{id} and
// returns the domain as Collibra reports it afterwards.
func PatchDomain(ctx context.Context, client *http.Client, domainID string, patch DomainPatch) (*DomainDetails, error) {
	var out DomainDetails
	if err := patchJSON(ctx, client, "/rest/2.0/domains/"+url.PathEscape(domainID), patch, &out); err != nil {
		return nil, fmt.Errorf("patch domain: %w", err)
	}
	return &out, nil
}

// PatchCommunity applies a partial update via PATCH /rest/2.0/communities/{id}
// and returns the community as Collibra reports it afterwards.
func PatchCommunity(ctx context.Context, client *http.Client, communityID string, patch CommunityPatch) (*CommunityDetails, error) {
	var out CommunityDetails
	if err := patchJSON(ctx, client, "/rest/2.0/communities/"+url.PathEscape(communityID), patch, &out); err != nil {
		return nil, fmt.Errorf("patch community: %w", err)
	}
	return &out, nil
}

func patchJSON(ctx context.Context, client *http.Client, reqURL string, payload, out any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshaling request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPatch, reqURL, bytes.NewReader(body))
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
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("status %d: %s", resp.StatusCode, string(respBody))
	}
	if err := json.Unmarshal(respBody, out); err != nil {
		return fmt.Errorf("decoding response: %w", err)
	}
	return nil
}
