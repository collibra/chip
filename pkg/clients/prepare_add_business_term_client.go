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

// --- Domain types ---

// PrepareAddBusinessTermDomain represents a Collibra domain.
type PrepareAddBusinessTermDomain struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// PrepareAddBusinessTermDomainsResponse is the paged response for listing domains.
type PrepareAddBusinessTermDomainsResponse struct {
	Total   int                              `json:"total"`
	Results []PrepareAddBusinessTermDomain   `json:"results"`
}

// --- Asset type ---

// PrepareAddBusinessTermAssetType represents a Collibra asset type.
type PrepareAddBusinessTermAssetType struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// --- Assignments ---

// PrepareAddBusinessTermAssignmentTypeRef is a reference to an attribute type within an assignment.
type PrepareAddBusinessTermAssignmentTypeRef struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// PrepareAddBusinessTermAssignment represents a flattened attribute assignment for an asset type.
// This is derived from the raw API response which nests characteristic type references.
type PrepareAddBusinessTermAssignment struct {
	ID            string                                  `json:"id"`
	AttributeType PrepareAddBusinessTermAssignmentTypeRef `json:"attributeType"`
	Min           int                                     `json:"min"`
	Max           int                                     `json:"max"`
}

// prepareAddBusinessTermResourceRef is a resource reference with a discriminator.
type prepareAddBusinessTermResourceRef struct {
	ID                    string `json:"id"`
	Name                  string `json:"name"`
	ResourceDiscriminator string `json:"resourceDiscriminator"`
}

// prepareAddBusinessTermCharacteristicTypeRef represents a characteristic type reference from the API.
type prepareAddBusinessTermCharacteristicTypeRef struct {
	ID                        string                                    `json:"id"`
	AssignedResourceReference prepareAddBusinessTermResourceRef         `json:"assignedResourceReference"`
	MinimumOccurrences        int                                       `json:"minimumOccurrences"`
	MaximumOccurrences        *int                                      `json:"maximumOccurrences"`
}

// prepareAddBusinessTermRawAssignment represents the raw API response for an assignment.
type prepareAddBusinessTermRawAssignment struct {
	ID                                  string                                                `json:"id"`
	AssignedCharacteristicTypeReferences []prepareAddBusinessTermCharacteristicTypeRef         `json:"assignedCharacteristicTypeReferences"`
}

// --- Attribute type ---

// PrepareAddBusinessTermRelationType represents relation type information within an attribute type.
type PrepareAddBusinessTermRelationType struct {
	ID         string                                  `json:"id"`
	Role       string                                  `json:"role"`
	CoRole     string                                  `json:"coRole"`
	Direction  string                                  `json:"direction"`
	TargetType PrepareAddBusinessTermAssignmentTypeRef `json:"targetType"`
	SourceType PrepareAddBusinessTermAssignmentTypeRef `json:"sourceType"`
}

// PrepareAddBusinessTermConstraints represents validation constraints for an attribute type.
type PrepareAddBusinessTermConstraints struct {
	MinLength *int `json:"minLength,omitempty"`
	MaxLength *int `json:"maxLength,omitempty"`
}

// PrepareAddBusinessTermAttributeType represents a full attribute type with schema details.
// Fields are mapped from the Collibra API response where the "kind" comes from
// attributeTypeDiscriminator and structural fields like constraints/relationType
// are not part of the standard attribute type API response.
type PrepareAddBusinessTermAttributeType struct {
	ID            string                                  `json:"id"`
	Name          string                                  `json:"name"`
	Kind          string                                  `json:"kind"`
	Required      bool                                    `json:"required"`
	AllowedValues []string                                `json:"allowedValues"`
	Constraints   *PrepareAddBusinessTermConstraints      `json:"constraints,omitempty"`
	Description   string                                  `json:"description"`
	RelationType  *PrepareAddBusinessTermRelationType     `json:"relationType,omitempty"`
}

// prepareAddBusinessTermRawAttributeType represents the raw API response for an attribute type.
type prepareAddBusinessTermRawAttributeType struct {
	ID                         string `json:"id"`
	Name                       string `json:"name"`
	Description                string `json:"description"`
	AttributeTypeDiscriminator string `json:"attributeTypeDiscriminator"`
	StringType                 string `json:"stringType,omitempty"`
	AllowedValues              []string `json:"allowedValues,omitempty"`
}

// --- Assets for duplicate check ---

// PrepareAddBusinessTermAsset represents an asset returned from a search.
type PrepareAddBusinessTermAsset struct {
	ID     string                       `json:"id"`
	Name   string                       `json:"name"`
	Domain PrepareAddBusinessTermDomain `json:"domain"`
}

// PrepareAddBusinessTermAssetsResponse is the paged response for searching assets.
type PrepareAddBusinessTermAssetsResponse struct {
	Total   int                             `json:"total"`
	Results []PrepareAddBusinessTermAsset   `json:"results"`
}

// --- Client Functions ---

// PrepareAddBusinessTermListDomains lists all available domains.
func PrepareAddBusinessTermListDomains(ctx context.Context, client *http.Client) ([]PrepareAddBusinessTermDomain, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "/rest/2.0/domains", nil)
	if err != nil {
		return nil, fmt.Errorf("creating list domains request: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("listing domains: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("listing domains: status %d: %s", resp.StatusCode, string(body))
	}

	var result PrepareAddBusinessTermDomainsResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decoding domains response: %w", err)
	}

	return result.Results, nil
}

// PrepareAddBusinessTermGetDomain gets a specific domain by ID.
func PrepareAddBusinessTermGetDomain(ctx context.Context, client *http.Client, domainID string) (*PrepareAddBusinessTermDomain, error) {
	path := fmt.Sprintf("/rest/2.0/domains/%s", url.PathEscape(domainID))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, fmt.Errorf("creating get domain request: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("getting domain %s: %w", domainID, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("getting domain %s: status %d: %s", domainID, resp.StatusCode, string(body))
	}

	var domain PrepareAddBusinessTermDomain
	if err := json.NewDecoder(resp.Body).Decode(&domain); err != nil {
		return nil, fmt.Errorf("decoding domain response: %w", err)
	}

	return &domain, nil
}

// PrepareAddBusinessTermGetAssetType gets an asset type by its public ID.
func PrepareAddBusinessTermGetAssetType(ctx context.Context, client *http.Client, publicID string) (*PrepareAddBusinessTermAssetType, error) {
	path := fmt.Sprintf("/rest/2.0/assetTypes/publicId/%s", url.PathEscape(publicID))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, fmt.Errorf("creating get asset type request: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("getting asset type %s: %w", publicID, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("getting asset type %s: status %d: %s", publicID, resp.StatusCode, string(body))
	}

	var assetType PrepareAddBusinessTermAssetType
	if err := json.NewDecoder(resp.Body).Decode(&assetType); err != nil {
		return nil, fmt.Errorf("decoding asset type response: %w", err)
	}

	return &assetType, nil
}

// PrepareAddBusinessTermGetAssignments gets attribute assignments for an asset type.
// The Collibra API returns a plain JSON array of assignment objects, each containing
// nested assignedCharacteristicTypeReferences. This function flattens them into
// a list of PrepareAddBusinessTermAssignment for easier consumption.
func PrepareAddBusinessTermGetAssignments(ctx context.Context, client *http.Client, assetTypeID string) ([]PrepareAddBusinessTermAssignment, error) {
	path := fmt.Sprintf("/rest/2.0/assignments/assetType/%s", url.PathEscape(assetTypeID))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, path, nil)
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

	var rawAssignments []prepareAddBusinessTermRawAssignment
	if err := json.NewDecoder(resp.Body).Decode(&rawAssignments); err != nil {
		return nil, fmt.Errorf("decoding assignments response: %w", err)
	}

	// Flatten: extract each characteristic type reference as an individual assignment.
	// Only include attribute types (discriminator ending with "AttributeType"),
	// skipping relation types and complex relation types.
	var assignments []PrepareAddBusinessTermAssignment
	for _, raw := range rawAssignments {
		for _, ref := range raw.AssignedCharacteristicTypeReferences {
			disc := ref.AssignedResourceReference.ResourceDiscriminator
			if disc != "" && !strings.HasSuffix(disc, "AttributeType") {
				continue
			}
			maxVal := 0
			if ref.MaximumOccurrences != nil {
				maxVal = *ref.MaximumOccurrences
			}
			assignments = append(assignments, PrepareAddBusinessTermAssignment{
				ID: ref.ID,
				AttributeType: PrepareAddBusinessTermAssignmentTypeRef{
					ID:   ref.AssignedResourceReference.ID,
					Name: ref.AssignedResourceReference.Name,
				},
				Min: ref.MinimumOccurrences,
				Max: maxVal,
			})
		}
	}

	return assignments, nil
}

// PrepareAddBusinessTermGetAttributeType gets the full attribute type schema by ID.
// The Collibra API returns attributeTypeDiscriminator (e.g. "StringAttributeType")
// which is mapped to the Kind field. Fields like constraints and relationType are
// not part of the standard API response and will be nil.
func PrepareAddBusinessTermGetAttributeType(ctx context.Context, client *http.Client, id string) (*PrepareAddBusinessTermAttributeType, error) {
	path := fmt.Sprintf("/rest/2.0/attributeTypes/%s", url.PathEscape(id))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, fmt.Errorf("creating get attribute type request: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("getting attribute type %s: %w", id, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("getting attribute type %s: status %d: %s", id, resp.StatusCode, string(body))
	}

	var raw prepareAddBusinessTermRawAttributeType
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("decoding attribute type response: %w", err)
	}

	return &PrepareAddBusinessTermAttributeType{
		ID:            raw.ID,
		Name:          raw.Name,
		Kind:          raw.AttributeTypeDiscriminator,
		Description:   raw.Description,
		AllowedValues: raw.AllowedValues,
	}, nil
}

// PrepareAddBusinessTermSearchAssets searches for assets by name and type ID for duplicate detection.
func PrepareAddBusinessTermSearchAssets(ctx context.Context, client *http.Client, name string, typeID string) ([]PrepareAddBusinessTermAsset, error) {
	reqURL := "/rest/2.0/assets"
	params := url.Values{}
	if name != "" {
		params.Set("name", name)
	}
	if typeID != "" {
		params.Set("typeId", typeID)
	}
	if len(params) > 0 {
		reqURL += "?" + params.Encode()
	}

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

	var result PrepareAddBusinessTermAssetsResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decoding assets response: %w", err)
	}

	return result.Results, nil
}
