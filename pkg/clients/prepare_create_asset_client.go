package clients

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// PrepareCreateAssetStatus represents the status of asset creation readiness.
type PrepareCreateAssetStatus string

const (
	StatusReady              PrepareCreateAssetStatus = "ready"
	StatusIncomplete         PrepareCreateAssetStatus = "incomplete"
	StatusNeedsClarification PrepareCreateAssetStatus = "needs_clarification"
	StatusDuplicateFound     PrepareCreateAssetStatus = "duplicate_found"
)

// PrepareCreateAssetType represents an asset type from the API.
type PrepareCreateAssetType struct {
	ID       string `json:"id"`
	PublicID string `json:"publicId"`
	Name     string `json:"name"`
}

// PrepareCreateAssetTypeListResponse is the response from listing asset types.
type PrepareCreateAssetTypeListResponse struct {
	Results []PrepareCreateAssetType `json:"results"`
	Total   int                      `json:"total"`
}

// PrepareCreateDomain represents a domain from the API.
type PrepareCreateDomain struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// PrepareCreateDomainListResponse is the response from listing domains.
type PrepareCreateDomainListResponse struct {
	Results []PrepareCreateDomain `json:"results"`
	Total   int                   `json:"total"`
}


// PrepareCreateAttributeType represents an attribute type with full schema.
type PrepareCreateAttributeType struct {
	ID             string                          `json:"id"`
	Name           string                          `json:"name"`
	Kind           string                          `json:"kind"`
	Required       bool                            `json:"required"`
	Constraints    *PrepareCreateConstraints       `json:"constraints,omitempty"`
	AllowedValues  []string                        `json:"allowedValues,omitempty"`
	Direction      string                          `json:"direction,omitempty"`
	TargetAssetType *PrepareCreateAssetType        `json:"targetAssetType,omitempty"`
}

// PrepareCreateConstraints represents attribute validation constraints.
type PrepareCreateConstraints struct {
	MinLength *int     `json:"minLength,omitempty"`
	MaxLength *int     `json:"maxLength,omitempty"`
	Min       *float64 `json:"min,omitempty"`
	Max       *float64 `json:"max,omitempty"`
}

// PrepareCreateAssetResult represents an existing asset found during duplicate check.
type PrepareCreateAssetResult struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// PrepareCreateAssetSearchResponse is the response from searching assets.
type PrepareCreateAssetSearchResponse struct {
	Results []PrepareCreateAssetResult `json:"results"`
	Total   int                        `json:"total"`
}

// ListAssetTypesForPrepare lists asset types, limited to the given count.
func ListAssetTypesForPrepare(ctx context.Context, client *http.Client, limit int) ([]PrepareCreateAssetType, int, error) {
	reqURL := fmt.Sprintf("/rest/2.0/assetTypes?limit=%d&offset=0", limit)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, 0, fmt.Errorf("creating list asset types request: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("listing asset types: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, 0, fmt.Errorf("listing asset types: status %d: %s", resp.StatusCode, string(body))
	}

	var result PrepareCreateAssetTypeListResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, 0, fmt.Errorf("decoding asset types response: %w", err)
	}
	return result.Results, result.Total, nil
}

// GetAssetTypeByPublicID resolves an asset type by its publicId.
func GetAssetTypeByPublicID(ctx context.Context, client *http.Client, publicID string) (*PrepareCreateAssetType, error) {
	reqURL := fmt.Sprintf("/rest/2.0/assetTypes/publicId/%s", url.PathEscape(publicID))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("creating get asset type request: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("getting asset type: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("asset type with publicId %q not found", publicID)
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("getting asset type: status %d: %s", resp.StatusCode, string(body))
	}

	var result PrepareCreateAssetType
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decoding asset type response: %w", err)
	}
	return &result, nil
}

// ListDomainsForPrepare lists domains, limited to the given count.
func ListDomainsForPrepare(ctx context.Context, client *http.Client, limit int) ([]PrepareCreateDomain, int, error) {
	reqURL := fmt.Sprintf("/rest/2.0/domains?limit=%d&offset=0", limit)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, 0, fmt.Errorf("creating list domains request: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("listing domains: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, 0, fmt.Errorf("listing domains: status %d: %s", resp.StatusCode, string(body))
	}

	var result PrepareCreateDomainListResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, 0, fmt.Errorf("decoding domains response: %w", err)
	}
	return result.Results, result.Total, nil
}

// GetDomainByID gets a specific domain by its ID.
func GetDomainByID(ctx context.Context, client *http.Client, domainID string) (*PrepareCreateDomain, error) {
	reqURL := fmt.Sprintf("/rest/2.0/domains/%s", url.PathEscape(domainID))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("creating get domain request: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("getting domain: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("domain with id %q not found", domainID)
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("getting domain: status %d: %s", resp.StatusCode, string(body))
	}

	var result PrepareCreateDomain
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decoding domain response: %w", err)
	}
	return &result, nil
}

// GetAvailableAssetTypesForDomain returns the asset types allowed in a given domain.
func GetAvailableAssetTypesForDomain(ctx context.Context, client *http.Client, domainID string) ([]PrepareCreateAssetType, error) {
	reqURL := fmt.Sprintf("/rest/2.0/assignments/domain/%s/assetTypes", url.PathEscape(domainID))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("creating get available asset types request: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("getting available asset types for domain: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("getting available asset types for domain: status %d: %s", resp.StatusCode, string(body))
	}

	var result []PrepareCreateAssetType
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decoding available asset types response: %w", err)
	}
	return result, nil
}

// GetAttributeTypeByID gets the full attribute type schema by ID.
func GetAttributeTypeByID(ctx context.Context, client *http.Client, attrTypeID string) (*PrepareCreateAttributeType, error) {
	reqURL := fmt.Sprintf("/rest/2.0/attributeTypes/%s", url.PathEscape(attrTypeID))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("creating get attribute type request: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("getting attribute type: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("getting attribute type: status %d: %s", resp.StatusCode, string(body))
	}

	var result PrepareCreateAttributeType
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decoding attribute type response: %w", err)
	}
	return &result, nil
}

// prepareCreateResourceRef is a resource reference with a discriminator.
type prepareCreateResourceRef struct {
	ID                    string `json:"id"`
	Name                  string `json:"name"`
	ResourceDiscriminator string `json:"resourceDiscriminator"`
}

// prepareCreateCharacteristicTypeRef represents a characteristic type reference from the API.
type prepareCreateCharacteristicTypeRef struct {
	ID                        string                   `json:"id"`
	AssignedResourceReference prepareCreateResourceRef `json:"assignedResourceReference"`
	MinimumOccurrences        int                      `json:"minimumOccurrences"`
	MaximumOccurrences        *int                     `json:"maximumOccurrences"`
}

// prepareCreateRawAssignment represents the raw API response for an assignment.
type prepareCreateRawAssignment struct {
	ID                                   string                               `json:"id"`
	AssignedCharacteristicTypeReferences []prepareCreateCharacteristicTypeRef `json:"assignedCharacteristicTypeReferences"`
}

// PrepareCreateAssignment represents a flattened attribute assignment for an asset type.
type PrepareCreateAssignment struct {
	AttributeTypeID string
	Name            string
	Min             int
}

// GetScopedAssignments fetches attribute assignments for an asset type and flattens them.
func GetScopedAssignments(ctx context.Context, client *http.Client, assetTypeID string) ([]PrepareCreateAssignment, error) {
	reqURL := fmt.Sprintf("/rest/2.0/assignments/assetType/%s", url.PathEscape(assetTypeID))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("creating get assignments request: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("getting assignments for asset type %s: %w", assetTypeID, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("getting assignments for asset type %s: status %d: %s", assetTypeID, resp.StatusCode, string(body))
	}

	var rawAssignments []prepareCreateRawAssignment
	if err := json.NewDecoder(resp.Body).Decode(&rawAssignments); err != nil {
		return nil, fmt.Errorf("decoding assignments response: %w", err)
	}

	var assignments []PrepareCreateAssignment
	for _, raw := range rawAssignments {
		for _, ref := range raw.AssignedCharacteristicTypeReferences {
			disc := ref.AssignedResourceReference.ResourceDiscriminator
			if disc != "" && !strings.HasSuffix(disc, "AttributeType") {
				continue
			}
			assignments = append(assignments, PrepareCreateAssignment{
				AttributeTypeID: ref.AssignedResourceReference.ID,
				Name:            ref.AssignedResourceReference.Name,
				Min:             ref.MinimumOccurrences,
			})
		}
	}

	return assignments, nil
}

// SearchAssetsForDuplicate searches for existing assets by name, type, and domain.
func SearchAssetsForDuplicate(ctx context.Context, client *http.Client, name string, assetTypeID string, domainID string) ([]PrepareCreateAssetResult, error) {
	params := url.Values{}
	params.Set("name", name)
	params.Set("typeId", assetTypeID)
	params.Set("domainId", domainID)
	params.Set("limit", "1")

	reqURL := fmt.Sprintf("/rest/2.0/assets?%s", params.Encode())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("creating search assets request: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("searching assets: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("searching assets: status %d: %s", resp.StatusCode, string(body))
	}

	var result PrepareCreateAssetSearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decoding asset search response: %w", err)
	}
	return result.Results, nil
}
