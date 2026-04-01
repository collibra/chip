package clients

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
)

// BusinessTermAssetTypePublicID is the well-known public ID for the Business Term asset type.
const BusinessTermAssetTypePublicID = "BusinessTerm"

// --- Response types for prepare_add_business_term ---

// PrepareAddBusinessTermResourceRef represents a named resource reference (NamedResourceReferenceImpl).
type PrepareAddBusinessTermResourceRef struct {
	ID                    string `json:"id"`
	Name                  string `json:"name"`
	ResourceDiscriminator string `json:"resourceDiscriminator,omitempty"`
}

// PrepareAddBusinessTermDomainResponse represents a domain from GET /rest/2.0/domains/{domainId}.
type PrepareAddBusinessTermDomainResponse struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

// PrepareAddBusinessTermDomainPagedResponse represents the paginated response from GET /rest/2.0/domains.
type PrepareAddBusinessTermDomainPagedResponse struct {
	Results []PrepareAddBusinessTermDomainResponse `json:"results"`
	Total   int                                    `json:"total"`
	Limit   int                                    `json:"limit"`
	Offset  int                                    `json:"offset"`
}

// PrepareAddBusinessTermAssetTypeResponse represents an asset type from GET /rest/2.0/assetTypes/publicId/{publicId}.
type PrepareAddBusinessTermAssetTypeResponse struct {
	ID          string `json:"id"`
	PublicID    string `json:"publicId"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

// PrepareAddBusinessTermCharacteristicTypeRef represents an assigned characteristic type reference.
type PrepareAddBusinessTermCharacteristicTypeRef struct {
	ID                        string                                    `json:"id"`
	AssignedResourceReference PrepareAddBusinessTermResourceRef         `json:"assignedResourceReference"`
	MinimumOccurrences        int                                      `json:"minimumOccurrences"`
	MaximumOccurrences        *int                                     `json:"maximumOccurrences,omitempty"`
}

// PrepareAddBusinessTermAssignmentResponse represents an assignment from GET /rest/2.0/assignments/assetType/{assetTypeId}.
type PrepareAddBusinessTermAssignmentResponse struct {
	ID                                    string                                                `json:"id"`
	AssignedCharacteristicTypeReferences  []PrepareAddBusinessTermCharacteristicTypeRef          `json:"assignedCharacteristicTypeReferences"`
}

// PrepareAddBusinessTermConstraintsResponse represents attribute constraints.
type PrepareAddBusinessTermConstraintsResponse struct {
	MinLength     int      `json:"minLength,omitempty"`
	MaxLength     int      `json:"maxLength,omitempty"`
	Pattern       string   `json:"pattern,omitempty"`
	AllowedValues []string `json:"allowedValues,omitempty"`
}

// PrepareAddBusinessTermRelationTypeResponse represents a relation type within an attribute type.
type PrepareAddBusinessTermRelationTypeResponse struct {
	ID                string `json:"id"`
	Name              string `json:"name"`
	Direction         string `json:"direction"`
	TargetAssetTypeID string `json:"targetAssetTypeId"`
}

// PrepareAddBusinessTermAttributeTypeResponse represents an attribute type from GET /rest/2.0/attributeTypes/{id}.
type PrepareAddBusinessTermAttributeTypeResponse struct {
	ID            string                                      `json:"id"`
	Name          string                                      `json:"name"`
	Kind          string                                      `json:"kind"`
	Description   string                                      `json:"description"`
	Constraints   *PrepareAddBusinessTermConstraintsResponse   `json:"constraints,omitempty"`
	RelationTypes []PrepareAddBusinessTermRelationTypeResponse `json:"relationTypes,omitempty"`
}

// PrepareAddBusinessTermAssetResponse represents an individual asset in search results.
type PrepareAddBusinessTermAssetResponse struct {
	ID          string                                    `json:"id"`
	Name        string                                    `json:"name"`
	Domain      PrepareAddBusinessTermResourceRef         `json:"domain"`
	Type        PrepareAddBusinessTermResourceRef         `json:"type"`
	DisplayName string                                    `json:"displayName"`
	Description string                                    `json:"description"`
}

// PrepareAddBusinessTermSearchAssetsResponse represents the response from GET /rest/2.0/assets.
type PrepareAddBusinessTermSearchAssetsResponse struct {
	Results []PrepareAddBusinessTermAssetResponse `json:"results"`
	Total   int                                   `json:"total"`
	Limit   int                                   `json:"limit"`
	Offset  int                                   `json:"offset"`
}

// PrepareAddBusinessTermExtractedAssignment is the simplified assignment info extracted from the API response.
type PrepareAddBusinessTermExtractedAssignment struct {
	AttributeTypeID string
	Required        bool
}

// --- Client functions ---

// PrepareAddBusinessTermListDomains lists all available domains.
func PrepareAddBusinessTermListDomains(ctx context.Context, client *http.Client) ([]PrepareAddBusinessTermDomainResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "/rest/2.0/domains?limit=100&excludeMeta=true", nil)
	if err != nil {
		return nil, fmt.Errorf("creating list domains request: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("listing domains: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("listing domains: status %d: %s", resp.StatusCode, string(body))
	}

	var paged PrepareAddBusinessTermDomainPagedResponse
	if err := json.NewDecoder(resp.Body).Decode(&paged); err != nil {
		return nil, fmt.Errorf("decoding domains response: %w", err)
	}
	return paged.Results, nil
}

// PrepareAddBusinessTermGetDomain gets a specific domain by ID.
func PrepareAddBusinessTermGetDomain(ctx context.Context, client *http.Client, domainID string) (*PrepareAddBusinessTermDomainResponse, error) {
	reqURL := fmt.Sprintf("/rest/2.0/domains/%s", url.PathEscape(domainID))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("creating get domain request: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("getting domain %s: %w", domainID, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("getting domain %s: status %d: %s", domainID, resp.StatusCode, string(body))
	}

	var domain PrepareAddBusinessTermDomainResponse
	if err := json.NewDecoder(resp.Body).Decode(&domain); err != nil {
		return nil, fmt.Errorf("decoding domain response: %w", err)
	}
	return &domain, nil
}

// PrepareAddBusinessTermGetAssetType gets an asset type by its public ID.
func PrepareAddBusinessTermGetAssetType(ctx context.Context, client *http.Client, publicID string) (*PrepareAddBusinessTermAssetTypeResponse, error) {
	reqURL := fmt.Sprintf("/rest/2.0/assetTypes/publicId/%s", url.PathEscape(publicID))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("creating get asset type request: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("getting asset type %s: %w", publicID, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("getting asset type %s: status %d: %s", publicID, resp.StatusCode, string(body))
	}

	var assetType PrepareAddBusinessTermAssetTypeResponse
	if err := json.NewDecoder(resp.Body).Decode(&assetType); err != nil {
		return nil, fmt.Errorf("decoding asset type response: %w", err)
	}
	return &assetType, nil
}

// PrepareAddBusinessTermGetAssignments gets attribute assignments for an asset type and extracts
// the attribute type IDs and required status from the characteristic type references.
func PrepareAddBusinessTermGetAssignments(ctx context.Context, client *http.Client, assetTypeID string) ([]PrepareAddBusinessTermExtractedAssignment, error) {
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
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("getting assignments for asset type %s: status %d: %s", assetTypeID, resp.StatusCode, string(body))
	}

	var assignments []PrepareAddBusinessTermAssignmentResponse
	if err := json.NewDecoder(resp.Body).Decode(&assignments); err != nil {
		return nil, fmt.Errorf("decoding assignments response: %w", err)
	}

	// Deduplicate attribute type IDs across all assignments using a map.
	// Only include characteristic types that are attribute types (not relation types).
	attributeTypeDiscriminators := map[string]bool{
		"AttributeType":              true,
		"StringAttributeType":        true,
		"ScriptAttributeType":        true,
		"BooleanAttributeType":       true,
		"DateAttributeType":          true,
		"NumericAttributeType":       true,
		"SingleValueListAttributeType": true,
		"MultiValueListAttributeType":  true,
	}

	seen := make(map[string]bool)
	var extracted []PrepareAddBusinessTermExtractedAssignment
	for _, assignment := range assignments {
		for _, ref := range assignment.AssignedCharacteristicTypeReferences {
			disc := ref.AssignedResourceReference.ResourceDiscriminator
			if !attributeTypeDiscriminators[disc] {
				continue
			}
			attrTypeID := ref.AssignedResourceReference.ID
			if attrTypeID == "" || seen[attrTypeID] {
				continue
			}
			seen[attrTypeID] = true
			extracted = append(extracted, PrepareAddBusinessTermExtractedAssignment{
				AttributeTypeID: attrTypeID,
				Required:        ref.MinimumOccurrences > 0,
			})
		}
	}
	return extracted, nil
}

// PrepareAddBusinessTermGetAttributeType gets the full attribute type schema by ID.
func PrepareAddBusinessTermGetAttributeType(ctx context.Context, client *http.Client, attributeTypeID string) (*PrepareAddBusinessTermAttributeTypeResponse, error) {
	reqURL := fmt.Sprintf("/rest/2.0/attributeTypes/%s", url.PathEscape(attributeTypeID))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("creating get attribute type request: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("getting attribute type %s: %w", attributeTypeID, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("getting attribute type %s: status %d: %s", attributeTypeID, resp.StatusCode, string(body))
	}

	var attrType PrepareAddBusinessTermAttributeTypeResponse
	if err := json.NewDecoder(resp.Body).Decode(&attrType); err != nil {
		return nil, fmt.Errorf("decoding attribute type response: %w", err)
	}
	return &attrType, nil
}

// PrepareAddBusinessTermSearchAssets searches for assets matching the given criteria.
func PrepareAddBusinessTermSearchAssets(ctx context.Context, client *http.Client, name string, assetTypeID string, domainID string) (*PrepareAddBusinessTermSearchAssetsResponse, error) {
	params := url.Values{}
	if name != "" {
		params.Set("name", name)
		params.Set("nameMatchMode", "EXACT")
	}
	if assetTypeID != "" {
		params.Set("typeIds", assetTypeID)
	}
	if domainID != "" {
		params.Set("domainId", domainID)
	}
	params.Set("limit", "10")
	params.Set("offset", "0")

	reqURL := "/rest/2.0/assets?" + params.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("creating search assets request: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("searching assets: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("searching assets: status %d: %s", resp.StatusCode, string(body))
	}

	var result PrepareAddBusinessTermSearchAssetsResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decoding search assets response: %w", err)
	}
	return &result, nil
}
