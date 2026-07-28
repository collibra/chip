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

// PrepareCreateAssetType represents an asset type from the API. Parent is
// populated by /assetTypes/{id} and drives the walk up the type hierarchy
// when locating the level that carries assignments.
type PrepareCreateAssetType struct {
	ID       string                  `json:"id"`
	PublicID string                  `json:"publicId"`
	Name     string                  `json:"name"`
	Parent   *PrepareCreateAssetType `json:"parent,omitempty"`
}

// PrepareCreateAssetTypeListResponse is the response from listing asset types.
type PrepareCreateAssetTypeListResponse struct {
	Results []PrepareCreateAssetType `json:"results"`
	Total   int                      `json:"total"`
}

// PrepareCreateDomain represents a domain from the API. Type is populated
// by the list and detail endpoints, but not by older callers that only
// decoded {id, name}; tolerate a missing type field there.
type PrepareCreateDomain struct {
	ID   string                   `json:"id"`
	Name string                   `json:"name"`
	Type *PrepareCreateDomainType `json:"type,omitempty"`
}

// PrepareCreateDomainType is a reference to a Collibra domain type — the
// scoped-assignment lookup keys off this ID to find the effective
// assignment for an asset type in a given domain.
type PrepareCreateDomainType struct {
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
	ID              string                    `json:"id"`
	Name            string                    `json:"name"`
	Kind            string                    `json:"kind"`
	Required        bool                      `json:"required"`
	Constraints     *PrepareCreateConstraints `json:"constraints,omitempty"`
	AllowedValues   []string                  `json:"allowedValues,omitempty"`
	Direction       string                    `json:"direction,omitempty"`
	TargetAssetType *PrepareCreateAssetType   `json:"targetAssetType,omitempty"`
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

// --- Consolidated lookups (used by both prepare_create_asset and create_asset) ---

// PrepareCreateStatus is one Collibra status value (e.g. "Candidate").
type PrepareCreateStatus struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// PrepareCreateStatusListResponse is the paged response for /statuses.
type PrepareCreateStatusListResponse struct {
	Results []PrepareCreateStatus `json:"results"`
	Total   int                   `json:"total"`
}

// PrepareCreateScopedAttribute is one attribute slot in a scoped assignment:
// what attribute type it refers to, whether it's required, and how many
// instances are allowed. Kind comes from the assignment's resourceDiscriminator
// (e.g. "StringAttributeType") so it's never empty for valid responses.
type PrepareCreateScopedAttribute struct {
	AttributeTypeID       string
	AttributeTypeName     string
	AttributeTypePublicID string
	Kind                  string
	Required              bool
	Min                   int
	// Max is nil when there is no upper bound (i.e. unbounded).
	Max *int
}

// PrepareCreateScopedRelation is one relation slot in a scoped assignment.
type PrepareCreateScopedRelation struct {
	RelationTypeID       string
	RelationTypePublicID string
	Kind                 string
	Role                 string
	CoRole               string
	// Direction is "TO_TARGET" or "TO_SOURCE" — describing which side of the
	// relation the asset being created sits on (TO_TARGET means it is the
	// source leg and the relation points out to the target).
	Direction  string
	TargetType *PrepareCreateAssetType
}

type PrepareCreateScopedAssignment struct {
	AssignmentID string
	Attributes   []PrepareCreateScopedAttribute
	Relations    []PrepareCreateScopedRelation
}

// PrepareCreateAttributeTypeFull is the full /attributeTypes/{id} response —
// includes StringType ("RICH_TEXT", "PLAIN_TEXT", etc.) which write tools
// use to decide whether to convert Markdown to HTML before submission.
type PrepareCreateAttributeTypeFull struct {
	ID            string   `json:"id"`
	Name          string   `json:"name"`
	PublicID      string   `json:"publicId"`
	Kind          string   `json:"attributeTypeDiscriminator"`
	StringType    string   `json:"stringType,omitempty"`
	Description   string   `json:"description,omitempty"`
	AllowedValues []string `json:"allowedValues,omitempty"`
}

// rawScopedAssignment mirrors the on-the-wire shape of a single assignment
// returned from /assignments/assetType/{id}. Fields we don't use are omitted.
type rawScopedAssignment struct {
	ID                                   string                                   `json:"id"`
	DomainTypes                          []rawAssignmentResourceRef               `json:"domainTypes"`
	AssignedCharacteristicTypeReferences []rawAssignedCharacteristicTypeReference `json:"assignedCharacteristicTypeReferences"`
	Scope                                *rawAssignmentScope                      `json:"scope"`
	TraitAssignmentInheritances          []rawTraitAssignmentInheritance          `json:"traitAssignmentInheritances"`
	AssignmentInheritances               []rawAssignmentInheritance               `json:"assignmentInheritances"`
}

type rawTraitAssignmentInheritance struct {
	AssignedCharacteristicTypeReferences []rawAssignedCharacteristicTypeReference `json:"assignedCharacteristicTypeReferences"`
}

type rawAssignmentInheritance struct {
	TraitAssignmentInheritances []rawTraitAssignmentInheritance `json:"traitAssignmentInheritances"`
}

type rawAssignmentScope struct {
	ID          string                     `json:"id"`
	Name        string                     `json:"name"`
	Domains     []rawAssignmentResourceRef `json:"domains"`
	Communities []rawAssignmentResourceRef `json:"communities"`
}

type rawAssignmentResourceRef struct {
	ID                    string `json:"id"`
	Name                  string `json:"name"`
	ResourceDiscriminator string `json:"resourceDiscriminator"`
}

type rawAssignedCharacteristicTypeReference struct {
	ID                        string                    `json:"id"`
	AssignedResourceReference rawAssignmentResourceRef  `json:"assignedResourceReference"`
	AssignedResourcePublicID  string                    `json:"assignedResourcePublicId"`
	MinimumOccurrences        int                       `json:"minimumOccurrences"`
	MaximumOccurrences        *int                      `json:"maximumOccurrences"`
	RelationTypeDirection     string                    `json:"relationTypeDirection,omitempty"`
	RelationTypeRestriction   *rawAssignmentResourceRef `json:"relationTypeRestriction,omitempty"`
}

// GetAssetTypeByID resolves an asset type by its UUID. Used as the first
// resolution strategy in the consolidated create_asset, before falling back
// to publicId or name search.
func GetAssetTypeByID(ctx context.Context, client *http.Client, id string) (*PrepareCreateAssetType, error) {
	reqURL := fmt.Sprintf("/rest/2.0/assetTypes/%s", url.PathEscape(id))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("building get asset type request: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("getting asset type by id: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("asset type with id %q not found", id)
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("getting asset type by id: status %d: %s", resp.StatusCode, string(body))
	}

	var result PrepareCreateAssetType
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decoding asset type response: %w", err)
	}
	return &result, nil
}

// SearchAssetTypesByName queries /assetTypes?name=… and returns the matches
// up to the given limit. Collibra performs a case-insensitive substring
// match server-side, so callers should still verify exact equality if they
// only want exact matches.
func SearchAssetTypesByName(ctx context.Context, client *http.Client, name string, limit int) ([]PrepareCreateAssetType, int, error) {
	params := url.Values{}
	params.Set("name", name)
	params.Set("limit", fmt.Sprintf("%d", limit))
	params.Set("offset", "0")

	reqURL := "/rest/2.0/assetTypes?" + params.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, 0, fmt.Errorf("building search asset types request: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("searching asset types by name: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, 0, fmt.Errorf("searching asset types by name: status %d: %s", resp.StatusCode, string(body))
	}

	var result PrepareCreateAssetTypeListResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, 0, fmt.Errorf("decoding asset types search response: %w", err)
	}
	return result.Results, result.Total, nil
}

// SearchDomainsByName queries /domains?name=… and returns the matches up
// to the given limit. The list endpoint already includes the domain Type
// in each result, so callers that need to look up a scoped assignment can
// keep working from the result without an extra GET /domains/{id}.
func SearchDomainsByName(ctx context.Context, client *http.Client, name string, limit int) ([]PrepareCreateDomain, int, error) {
	params := url.Values{}
	params.Set("name", name)
	params.Set("limit", fmt.Sprintf("%d", limit))
	params.Set("offset", "0")

	reqURL := "/rest/2.0/domains?" + params.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, 0, fmt.Errorf("building search domains request: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("searching domains by name: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, 0, fmt.Errorf("searching domains by name: status %d: %s", resp.StatusCode, string(body))
	}

	var result PrepareCreateDomainListResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, 0, fmt.Errorf("decoding domains search response: %w", err)
	}
	return result.Results, result.Total, nil
}

// ListStatusesAll fetches every status value defined in the instance.
// Status counts are small (~30) and fit comfortably in a single page;
// the limit guard is just defensive.
func ListStatusesAll(ctx context.Context, client *http.Client) ([]PrepareCreateStatus, error) {
	reqURL := "/rest/2.0/statuses?limit=500&offset=0"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("building list statuses request: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("listing statuses: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("listing statuses: status %d: %s", resp.StatusCode, string(body))
	}

	var result PrepareCreateStatusListResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decoding statuses response: %w", err)
	}
	return result.Results, nil
}

const maxAncestorDepth = 50

// GetScopedAssignment resolves the single assignment that governs creating
// an asset of the given type in the given domain: walk up to the first
// asset-type level that has assignments, select one by scope tier
// (domain-direct > community > global), then gate on the domain type.
func GetScopedAssignment(ctx context.Context, client *http.Client, assetTypeID, domainTypeID, domainID string) (*PrepareCreateScopedAssignment, error) {
	levels, err := fetchAssignmentLevels(ctx, client, assetTypeID)
	if err != nil {
		return nil, err
	}
	coveredScopes, err := resolveCoveredScopes(ctx, client, levels, domainID)
	if err != nil {
		return nil, err
	}
	return selectScopedAssignment(levels, domainTypeID, coveredScopes)
}

type assignmentLevel struct {
	assetType *PrepareCreateAssetType
	raws      []rawScopedAssignment
}

func fetchAssignmentLevels(ctx context.Context, client *http.Client, assetTypeID string) ([]assignmentLevel, error) {
	var levels []assignmentLevel
	currentID := assetTypeID
	seen := make(map[string]struct{})
	for depth := 0; depth < maxAncestorDepth; depth++ {
		if _, looped := seen[currentID]; looped {
			break
		}
		seen[currentID] = struct{}{}

		// Ancestor-level fetch errors are tolerated: resolution proceeds
		// with the levels collected so far.
		at, err := GetAssetTypeByID(ctx, client, currentID)
		if err != nil {
			if depth == 0 {
				return nil, err
			}
			break
		}
		raws, err := fetchRawAssignments(ctx, client, currentID)
		if err != nil {
			if depth == 0 {
				return nil, err
			}
			break
		}
		levels = append(levels, assignmentLevel{assetType: at, raws: raws})

		if at.Parent == nil || at.Parent.ID == "" {
			break
		}
		currentID = at.Parent.ID
	}
	return levels, nil
}

func fetchRawAssignments(ctx context.Context, client *http.Client, assetTypeID string) ([]rawScopedAssignment, error) {
	reqURL := fmt.Sprintf("/rest/2.0/assignments/assetType/%s", url.PathEscape(assetTypeID))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("building get assignments request: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("getting assignments: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("getting assignments: status %d: %s", resp.StatusCode, string(body))
	}

	var raws []rawScopedAssignment
	if err := json.NewDecoder(resp.Body).Decode(&raws); err != nil {
		return nil, fmt.Errorf("decoding assignments response: %w", err)
	}
	return raws, nil
}

type scopeTier int

const (
	scopeTierDomainDirect scopeTier = iota
	scopeTierCommunity
)

func resolveCoveredScopes(ctx context.Context, client *http.Client, levels []assignmentLevel, domainID string) (map[string]scopeTier, error) {
	scopes := make(map[string]*rawAssignmentScope)
	for _, level := range levels {
		for _, a := range level.raws {
			if a.Scope != nil && a.Scope.ID != "" {
				scopes[a.Scope.ID] = a.Scope
			}
		}
	}
	covered := make(map[string]scopeTier)
	if len(scopes) == 0 {
		return covered, nil
	}

	var ancestorCommunities map[string]struct{}
	for id, scope := range scopes {
		if containsResourceRef(scope.Domains, domainID) {
			covered[id] = scopeTierDomainDirect
			continue
		}
		if len(scope.Communities) == 0 {
			continue
		}
		if ancestorCommunities == nil {
			var err error
			ancestorCommunities, err = fetchDomainCommunityAncestors(ctx, client, domainID)
			if err != nil {
				return nil, err
			}
		}
		for _, c := range scope.Communities {
			if _, ok := ancestorCommunities[c.ID]; ok {
				covered[id] = scopeTierCommunity
				break
			}
		}
	}
	return covered, nil
}

func fetchDomainCommunityAncestors(ctx context.Context, client *http.Client, domainID string) (map[string]struct{}, error) {
	var domain struct {
		Community *rawAssignmentResourceRef `json:"community"`
	}
	if err := getJSON(ctx, client, fmt.Sprintf("/rest/2.0/domains/%s", url.PathEscape(domainID)), &domain); err != nil {
		return nil, fmt.Errorf("getting domain %q for scope coverage: %w", domainID, err)
	}

	ancestors := make(map[string]struct{})
	current := domain.Community
	for depth := 0; current != nil && current.ID != "" && depth < maxAncestorDepth; depth++ {
		if _, looped := ancestors[current.ID]; looped {
			break
		}
		ancestors[current.ID] = struct{}{}
		var community struct {
			Parent *rawAssignmentResourceRef `json:"parent"`
		}
		if err := getJSON(ctx, client, fmt.Sprintf("/rest/2.0/communities/%s", url.PathEscape(current.ID)), &community); err != nil {
			return nil, fmt.Errorf("getting community %q for scope coverage: %w", current.ID, err)
		}
		current = community.Parent
	}
	return ancestors, nil
}

func getJSON(ctx context.Context, client *http.Client, reqURL string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return fmt.Errorf("building request for %s: %w", reqURL, err)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("getting %s: %w", reqURL, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("getting %s: status %d: %s", reqURL, resp.StatusCode, string(body))
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("decoding %s response: %w", reqURL, err)
	}
	return nil
}

func selectScopedAssignment(levels []assignmentLevel, domainTypeID string, coveredScopes map[string]scopeTier) (*PrepareCreateScopedAssignment, error) {
	located := -1
	for i := range levels {
		if len(levels[i].raws) > 0 {
			located = i
			break
		}
	}
	if located < 0 {
		return nil, fmt.Errorf("no assignments found")
	}

	selected, ok := selectByTier(levels[located].raws, coveredScopes)
	if !ok {
		return nil, fmt.Errorf("no assignments found")
	}

	if !containsResourceRef(selected.DomainTypes, domainTypeID) {
		return nil, fmt.Errorf("no scoped assignment found for asset type in this domain type %q", domainTypeID)
	}

	return emitAssignmentCharacteristics(selected), nil
}

func selectByTier(raws []rawScopedAssignment, coveredScopes map[string]scopeTier) (rawScopedAssignment, bool) {
	var global, domainDirect, community *rawScopedAssignment
	for i := range raws {
		a := &raws[i]
		if a.Scope == nil {
			if global == nil {
				global = a
			}
			continue
		}
		tier, ok := coveredScopes[a.Scope.ID]
		if !ok {
			continue
		}
		switch tier {
		case scopeTierDomainDirect:
			if domainDirect == nil {
				domainDirect = a
			}
		case scopeTierCommunity:
			if community == nil {
				community = a
			}
		}
	}
	switch {
	case domainDirect != nil:
		return *domainDirect, true
	case community != nil:
		return *community, true
	case global != nil:
		return *global, true
	default:
		return rawScopedAssignment{}, false
	}
}

type characteristicKey struct {
	resourceID string
	direction  string
}

// characteristicSourcesFrom flattens an assignment's own characteristic
// references with those inherited from Traits applied directly to the asset
// type and from Traits on ancestor assignments, in closest-first order.
func characteristicSourcesFrom(
	refs []rawAssignedCharacteristicTypeReference,
	direct []rawTraitAssignmentInheritance,
	ancestor []rawAssignmentInheritance,
) [][]rawAssignedCharacteristicTypeReference {
	sources := [][]rawAssignedCharacteristicTypeReference{refs}
	for _, ti := range direct {
		sources = append(sources, ti.AssignedCharacteristicTypeReferences)
	}
	for _, ai := range ancestor {
		for _, ti := range ai.TraitAssignmentInheritances {
			sources = append(sources, ti.AssignedCharacteristicTypeReferences)
		}
	}
	return sources
}

func emitAssignmentCharacteristics(a rawScopedAssignment) *PrepareCreateScopedAssignment {
	out := &PrepareCreateScopedAssignment{AssignmentID: a.ID}
	seen := make(map[characteristicKey]struct{})
	for _, refs := range characteristicSourcesFrom(a.AssignedCharacteristicTypeReferences, a.TraitAssignmentInheritances, a.AssignmentInheritances) {
		for _, ref := range refs {
			disc := ref.AssignedResourceReference.ResourceDiscriminator
			if disc == "DerivedRelationType" {
				continue
			}
			switch {
			case isAttributeTypeDiscriminator(disc):
				key := characteristicKey{resourceID: ref.AssignedResourceReference.ID}
				if _, dup := seen[key]; dup {
					continue
				}
				seen[key] = struct{}{}
				out.Attributes = append(out.Attributes, PrepareCreateScopedAttribute{
					AttributeTypeID:       ref.AssignedResourceReference.ID,
					AttributeTypeName:     ref.AssignedResourceReference.Name,
					AttributeTypePublicID: ref.AssignedResourcePublicID,
					Kind:                  normalizeAttributeKind(disc),
					Required:              ref.MinimumOccurrences > 0,
					Min:                   ref.MinimumOccurrences,
					Max:                   ref.MaximumOccurrences,
				})
			case isRelationTypeDiscriminator(disc):
				key := characteristicKey{
					resourceID: ref.AssignedResourceReference.ID,
					direction:  ref.RelationTypeDirection,
				}
				if _, dup := seen[key]; dup {
					continue
				}
				seen[key] = struct{}{}
				rel := PrepareCreateScopedRelation{
					RelationTypeID:       ref.AssignedResourceReference.ID,
					RelationTypePublicID: ref.AssignedResourcePublicID,
					Kind:                 disc,
					Direction:            ref.RelationTypeDirection,
				}
				if ref.RelationTypeRestriction != nil {
					rel.TargetType = &PrepareCreateAssetType{
						ID:   ref.RelationTypeRestriction.ID,
						Name: ref.RelationTypeRestriction.Name,
					}
				}
				out.Relations = append(out.Relations, rel)
			}
		}
	}
	return out
}

func containsResourceRef(refs []rawAssignmentResourceRef, id string) bool {
	for _, r := range refs {
		if r.ID == id {
			return true
		}
	}
	return false
}

// isAttributeTypeDiscriminator recognises the assignment-side discriminator
// for attribute-style characteristics. Collibra returns values like
// "StringAttributeType", "BooleanAttributeType", "DateAttributeType",
// "NumericAttributeType", "ScriptAttributeType", and "SingleValueListAttributeType".
func isAttributeTypeDiscriminator(disc string) bool {
	return strings.HasSuffix(disc, "AttributeType")
}

// normalizeAttributeKind maps the platform-bug discriminator
// DateTimeAttributeType to the canonical DateAttributeType.
func normalizeAttributeKind(disc string) string {
	if disc == "DateTimeAttributeType" {
		return "DateAttributeType"
	}
	return disc
}

// isRelationTypeDiscriminator recognises the assignment-side discriminator
// for relation-style characteristics. ComplexRelationType is included
// because Collibra surfaces it through the same code path even though we
// don't currently wire it through to the agent.
func isRelationTypeDiscriminator(disc string) bool {
	return disc == "RelationType" || disc == "ComplexRelationType"
}

// PrepareCreateAllowedDomainType is one domain type an asset type can be
// created in.
type PrepareCreateAllowedDomainType struct {
	ID   string
	Name string
}

// ListAllowedDomainTypesForAssetType returns the deduped domain types of the
// first hierarchy level that has any assignment — the same level
// GetScopedAssignment resolves against.
func ListAllowedDomainTypesForAssetType(ctx context.Context, client *http.Client, assetTypeID string) ([]PrepareCreateAllowedDomainType, error) {
	levels, err := fetchAssignmentLevels(ctx, client, assetTypeID)
	if err != nil {
		return nil, err
	}

	seen := make(map[string]struct{})
	out := make([]PrepareCreateAllowedDomainType, 0)
	for _, level := range levels {
		if len(level.raws) == 0 {
			continue
		}
		for _, a := range level.raws {
			for _, dt := range a.DomainTypes {
				if _, ok := seen[dt.ID]; ok {
					continue
				}
				seen[dt.ID] = struct{}{}
				out = append(out, PrepareCreateAllowedDomainType{ID: dt.ID, Name: dt.Name})
			}
		}
		break
	}
	return out, nil
}

// NotAllowedMessage explains why an asset type can't be created in a domain,
// distinguishing "creatable nowhere on this instance" from "not in this
// domain" without leaking the allowed domain types.
func NotAllowedMessage(ctx context.Context, client *http.Client, assetTypeID, assetTypeName, domainName, domainTypeName string) string {
	creatableSomewhere := true
	if allowed, err := ListAllowedDomainTypesForAssetType(ctx, client, assetTypeID); err == nil {
		creatableSomewhere = len(allowed) > 0
	}
	if !creatableSomewhere {
		return fmt.Sprintf("Asset type %q can't be created in any domain on this instance.", assetTypeName)
	}
	return fmt.Sprintf(
		"Asset type %q isn't allowed in domain %q (domain type %q). Pick a different asset type, or a different domain.",
		assetTypeName, domainName, domainTypeName)
}

// GetAttributeTypeFull fetches /attributeTypes/{id} and decodes the full
// shape including stringType — needed for create_asset / edit_asset to
// gate Markdown→HTML conversion on RICH_TEXT attributes.
func GetAttributeTypeFull(ctx context.Context, client *http.Client, id string) (*PrepareCreateAttributeTypeFull, error) {
	reqURL := fmt.Sprintf("/rest/2.0/attributeTypes/%s", url.PathEscape(id))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("building get attribute type request: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("getting attribute type details: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("getting attribute type details: status %d: %s", resp.StatusCode, string(body))
	}

	var result PrepareCreateAttributeTypeFull
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decoding attribute type details response: %w", err)
	}
	return &result, nil
}

// PrepareCreateRelationTypeFull is the subset of the /relationTypes/{id}
// response we need. role/coRole and the two leg types only exist on the
// relation type resource, not on the assignment reference, so relation
// slots are hydrated with them separately.
type PrepareCreateRelationTypeFull struct {
	ID         string                  `json:"id"`
	PublicID   string                  `json:"publicId"`
	Role       string                  `json:"role"`
	CoRole     string                  `json:"coRole"`
	SourceType *PrepareCreateAssetType `json:"sourceType"`
	TargetType *PrepareCreateAssetType `json:"targetType"`
}

// GetRelationTypeFull fetches a relation type's role, coRole, and leg types
// from /rest/2.0/relationTypes/{id}. These are not part of the assignment
// payload, so relation slots are hydrated with them per id.
func GetRelationTypeFull(ctx context.Context, client *http.Client, id string) (*PrepareCreateRelationTypeFull, error) {
	reqURL := fmt.Sprintf("/rest/2.0/relationTypes/%s", url.PathEscape(id))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("building get relation type request: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("getting relation type details: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("getting relation type details: status %d: %s", resp.StatusCode, string(body))
	}

	var result PrepareCreateRelationTypeFull
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decoding relation type details response: %w", err)
	}
	return &result, nil
}

// PrepareCreateComplexRelationTypeFull is the subset of the
// /complexRelationTypes/{id} response we need. Unlike a simple relation type,
// a complex relation type has two or more legs, each with its own role and
// asset type, so there is no single role/coRole.
type PrepareCreateComplexRelationTypeFull struct {
	ID       string
	PublicID string
	Legs     []PrepareCreateComplexRelationLeg
}

// PrepareCreateComplexRelationLeg is one leg of a complex relation type.
type PrepareCreateComplexRelationLeg struct {
	Role                 string
	CoRole               string
	RelationTypePublicID string
	AssetTypeID          string
	AssetTypeName        string
	Min                  int
	Max                  *int
}

// GetComplexRelationTypeFull fetches a complex relation type's legs from
// /rest/2.0/complexRelationTypes/{id}. Complex relation type ids are not
// resolvable via /relationTypes/{id} (that endpoint 404s for them), so
// relation slots whose Kind is "ComplexRelationType" hydrate here instead.
func GetComplexRelationTypeFull(ctx context.Context, client *http.Client, id string) (*PrepareCreateComplexRelationTypeFull, error) {
	reqURL := fmt.Sprintf("/rest/2.0/complexRelationTypes/%s", url.PathEscape(id))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("building get complex relation type request: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("getting complex relation type details: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("getting complex relation type details: status %d: %s", resp.StatusCode, string(body))
	}

	var raw struct {
		ID       string `json:"id"`
		PublicID string `json:"publicId"`
		LegTypes []struct {
			Role                 string                    `json:"role"`
			CoRole               string                    `json:"coRole"`
			RelationTypePublicID string                    `json:"relationTypePublicId"`
			MinimumOccurrences   int                       `json:"minimumOccurrences"`
			MaximumOccurrences   *int                      `json:"maximumOccurrences"`
			AssetType            *rawAssignmentResourceRef `json:"assetType"`
		} `json:"legTypes"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("decoding complex relation type details response: %w", err)
	}

	result := PrepareCreateComplexRelationTypeFull{ID: raw.ID, PublicID: raw.PublicID}
	for _, leg := range raw.LegTypes {
		entry := PrepareCreateComplexRelationLeg{
			Role:                 leg.Role,
			CoRole:               leg.CoRole,
			RelationTypePublicID: leg.RelationTypePublicID,
			Min:                  leg.MinimumOccurrences,
			Max:                  leg.MaximumOccurrences,
		}
		if leg.AssetType != nil {
			entry.AssetTypeID = leg.AssetType.ID
			entry.AssetTypeName = leg.AssetType.Name
		}
		result.Legs = append(result.Legs, entry)
	}
	return &result, nil
}
