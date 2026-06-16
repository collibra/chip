package clients

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
)

// Community is a minimal community reference {id, name}, enough to resolve a
// community name typed by a user back to the UUID that search filters expect.
type Community struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type communityListResponse struct {
	Results []Community `json:"results"`
	Total   int         `json:"total"`
}

// SearchCommunitiesByName queries /communities?name=… and returns the matches
// up to the given limit. Collibra performs a case-insensitive substring match
// server-side, so callers wanting an exact hit should still verify name
// equality on the result (see search_asset_keyword's resolver).
func SearchCommunitiesByName(ctx context.Context, client *http.Client, name string, limit int) ([]Community, error) {
	params := url.Values{}
	params.Set("name", name)
	params.Set("limit", fmt.Sprintf("%d", limit))
	params.Set("offset", "0")

	reqURL := "/rest/2.0/communities?" + params.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("building search communities request: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("searching communities by name: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("searching communities by name: status %d: %s", resp.StatusCode, string(body))
	}

	var result communityListResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decoding communities search response: %w", err)
	}
	return result.Results, nil
}

// DomainType is a minimal domain-type reference {id, name}. Domain types are a
// small, enumerable set, so callers list them all and match in memory rather
// than hitting a name-filtered search per value.
type DomainType struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type domainTypeListResponse struct {
	Results []DomainType `json:"results"`
	Total   int          `json:"total"`
}

// ListDomainTypes fetches every domain type defined in the instance. The set is
// small (tens of entries in OOTB Collibra), so a single large page suffices.
func ListDomainTypes(ctx context.Context, client *http.Client) ([]DomainType, error) {
	reqURL := "/rest/2.0/domainTypes?limit=1000&offset=0"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("building list domain types request: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("listing domain types: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("listing domain types: status %d: %s", resp.StatusCode, string(body))
	}

	var result domainTypeListResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decoding domain types response: %w", err)
	}
	return result.Results, nil
}
