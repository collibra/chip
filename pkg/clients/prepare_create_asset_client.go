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

// PrepareCreateAssetType represents an asset type from the API. Parent
// is populated by /assetTypes/{id} for subtypes (e.g. Acronym → Business
// Term) and is the key for walking the inheritance chain when the
// subtype's own scoped-assignment record has empty domainTypes (which
// Collibra uses as the "inherit from parent" sentinel).
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

// --- Consolidated lookups (used by both discover_create_asset_options and create_asset) ---

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
	RelationTypeID string
	Role           string
	CoRole         string
	// Direction is "SOURCE_TO_TARGET" or "TARGET_TO_SOURCE" — describing
	// which side of the relation the asset being created sits on.
	Direction string
	// TargetType is the asset type on the other side of the relation.
	TargetType *PrepareCreateAssetType
}

// PrepareCreateScopedAssignment is the effective assignment for a given
// (assetType, domainType) pair: the union of the assignment's attribute
// slots and the relation slots that apply to assets of this type when
// created in domains of this type.
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
	ID                                   string                                    `json:"id"`
	DomainTypes                          []rawAssignmentResourceRef                `json:"domainTypes"`
	AssignedCharacteristicTypeReferences []rawAssignedCharacteristicTypeReference  `json:"assignedCharacteristicTypeReferences"`
	CharacteristicTypes                  []rawAssignmentCharacteristicTypeMetadata `json:"characteristicTypes"`
}

type rawAssignmentResourceRef struct {
	ID                    string `json:"id"`
	Name                  string `json:"name"`
	ResourceType          string `json:"resourceType"`
	ResourceDiscriminator string `json:"resourceDiscriminator"`
}

type rawAssignedCharacteristicTypeReference struct {
	ID                        string                   `json:"id"`
	AssignedResourceReference rawAssignmentResourceRef `json:"assignedResourceReference"`
	AssignedResourcePublicID  string                   `json:"assignedResourcePublicId"`
	MinimumOccurrences        int                      `json:"minimumOccurrences"`
	MaximumOccurrences        *int                     `json:"maximumOccurrences"`
}

// rawAssignmentCharacteristicTypeMetadata carries the relation-specific
// detail (role, coRole, direction, target type) that lives alongside the
// assignedCharacteristicTypeReferences list. We index it by id when
// hydrating relation slots.
type rawAssignmentCharacteristicTypeMetadata struct {
	ID         string                    `json:"id"`
	Role       string                    `json:"role,omitempty"`
	CoRole     string                    `json:"coRole,omitempty"`
	Direction  string                    `json:"direction,omitempty"`
	TargetType *rawAssignmentResourceRef `json:"targetType,omitempty"`
	SourceType *rawAssignmentResourceRef `json:"sourceType,omitempty"`
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

// maxAssignmentChainDepth caps how far we'll walk the asset type's parent
// chain when resolving inherited assignments. Defensive: in OOTB Collibra
// the chain is rarely deeper than 2-3 levels, but a misconfigured or
// cyclic instance shouldn't be able to spin us forever.
const maxAssignmentChainDepth = 5

// GetScopedAssignment returns the effective scoped assignment for an
// (assetType, domainType) pair, walking the asset type's parent chain
// when needed. Collibra's data model lets a subtype omit domainTypes on
// its own assignment (signalling "inherit from parent") and contribute
// its own characteristics on top — see Acronym → Business Term in OOTB
// glossary. The result here is the union of all chain levels'
// applicable characteristics, where applicable means the assignment
// either explicitly lists the target domainTypeID or has empty
// domainTypes (the inherit-sentinel). At least one level in the chain
// must explicitly include the target domain type, otherwise we return
// "not allowed" — empty-domainTypes-everywhere is not the same as
// "creatable everywhere".
func GetScopedAssignment(ctx context.Context, client *http.Client, assetTypeID, domainTypeID string) (*PrepareCreateScopedAssignment, error) {
	chain, err := fetchAssignmentChain(ctx, client, assetTypeID)
	if err != nil {
		return nil, err
	}
	return reduceScopedAssignmentChain(chain, domainTypeID)
}

// assignmentChainNode is one level of the asset type's parent chain — the
// type itself plus its raw assignments. Splitting the fetch (impure)
// from the reduction (pure) lets us unit-test the union/inheritance
// logic without an HTTP server.
type assignmentChainNode struct {
	assetType *PrepareCreateAssetType
	raws      []rawScopedAssignment
}

// fetchAssignmentChain walks parent → grandparent → … fetching each
// level's /assetTypes/{id} (for parent info) and /assignments/assetType/{id}.
// Stops at the root (no parent) or at maxAssignmentChainDepth.
func fetchAssignmentChain(ctx context.Context, client *http.Client, assetTypeID string) ([]assignmentChainNode, error) {
	var chain []assignmentChainNode
	currentID := assetTypeID
	seen := make(map[string]struct{})
	for depth := 0; depth < maxAssignmentChainDepth; depth++ {
		if _, looped := seen[currentID]; looped {
			break
		}
		seen[currentID] = struct{}{}

		at, err := GetAssetTypeByID(ctx, client, currentID)
		if err != nil {
			// Tolerate parent-fetch errors: if we can't get the parent's
			// info we still return what we have so far. Discovery falls
			// back to "no compatible domains" only if the entire chain
			// yielded nothing useful.
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
		chain = append(chain, assignmentChainNode{assetType: at, raws: raws})

		if at.Parent == nil || at.Parent.ID == "" {
			break
		}
		currentID = at.Parent.ID
	}
	return chain, nil
}

// fetchRawAssignments is the bare /assignments/assetType/{id} fetch,
// extracted from GetScopedAssignment so the chain walker can call it
// per level without re-implementing HTTP plumbing.
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

// reduceScopedAssignmentChain unions the applicable assignments across
// every chain level. An assignment is applicable to the target domain
// type when its domainTypes either (a) explicitly contains the target
// or (b) is empty (inherit-sentinel). At least one level must contain
// the target explicitly, or we return "not allowed". Characteristics
// are deduped by ID across the union — Collibra subtypes commonly add
// new characteristics rather than override parent ones, but we
// defensively dedupe in case of collisions.
func reduceScopedAssignmentChain(chain []assignmentChainNode, domainTypeID string) (*PrepareCreateScopedAssignment, error) {
	if len(chain) == 0 {
		return nil, fmt.Errorf("no assignments found")
	}

	explicitlyAllowed := false
	for _, node := range chain {
		for _, a := range node.raws {
			if containsDomainType(a.DomainTypes, domainTypeID) {
				explicitlyAllowed = true
				break
			}
		}
		if explicitlyAllowed {
			break
		}
	}
	if !explicitlyAllowed {
		return nil, fmt.Errorf("no scoped assignment found for asset type in this domain type %q", domainTypeID)
	}

	out := &PrepareCreateScopedAssignment{AssignmentID: chain[0].raws[0].ID}
	seenAttrIDs := make(map[string]struct{})
	seenRelIDs := make(map[string]struct{})

	for _, node := range chain {
		for _, a := range node.raws {
			applicable := len(a.DomainTypes) == 0 || containsDomainType(a.DomainTypes, domainTypeID)
			if !applicable {
				continue
			}
			metaByID := make(map[string]rawAssignmentCharacteristicTypeMetadata, len(a.CharacteristicTypes))
			for _, m := range a.CharacteristicTypes {
				metaByID[m.ID] = m
			}
			for _, ref := range a.AssignedCharacteristicTypeReferences {
				disc := ref.AssignedResourceReference.ResourceDiscriminator
				rt := ref.AssignedResourceReference.ResourceType
				switch {
				case isAttributeTypeDiscriminator(disc, rt):
					if _, dup := seenAttrIDs[ref.AssignedResourceReference.ID]; dup {
						continue
					}
					seenAttrIDs[ref.AssignedResourceReference.ID] = struct{}{}
					out.Attributes = append(out.Attributes, PrepareCreateScopedAttribute{
						AttributeTypeID:       ref.AssignedResourceReference.ID,
						AttributeTypeName:     ref.AssignedResourceReference.Name,
						AttributeTypePublicID: ref.AssignedResourcePublicID,
						Kind:                  disc,
						Required:              ref.MinimumOccurrences > 0,
						Min:                   ref.MinimumOccurrences,
						Max:                   ref.MaximumOccurrences,
					})
				case isRelationTypeDiscriminator(disc, rt):
					if _, dup := seenRelIDs[ref.AssignedResourceReference.ID]; dup {
						continue
					}
					seenRelIDs[ref.AssignedResourceReference.ID] = struct{}{}
					meta := metaByID[ref.AssignedResourceReference.ID]
					rel := PrepareCreateScopedRelation{
						RelationTypeID: ref.AssignedResourceReference.ID,
						Role:           meta.Role,
						CoRole:         meta.CoRole,
						Direction:      meta.Direction,
					}
					if meta.TargetType != nil {
						rel.TargetType = &PrepareCreateAssetType{
							ID:   meta.TargetType.ID,
							Name: meta.TargetType.Name,
						}
					}
					out.Relations = append(out.Relations, rel)
				}
			}
		}
	}
	return out, nil
}

func containsDomainType(refs []rawAssignmentResourceRef, id string) bool {
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
func isAttributeTypeDiscriminator(disc, resourceType string) bool {
	if disc == "" {
		return strings.HasSuffix(resourceType, "AttributeType")
	}
	return strings.HasSuffix(disc, "AttributeType")
}

// isRelationTypeDiscriminator recognises the assignment-side discriminator
// for relation-style characteristics. ComplexRelationType is included
// because Collibra surfaces it through the same code path even though we
// don't currently wire it through to the agent.
func isRelationTypeDiscriminator(disc, resourceType string) bool {
	if disc == "" {
		return strings.HasSuffix(resourceType, "RelationType")
	}
	return disc == "RelationType" || disc == "ComplexRelationType"
}

// PrepareCreateAllowedDomainType is one domain type an asset type can
// be created in. The set is deduped across all of an asset type's
// assignments (an asset type may be allowed in multiple domain types).
type PrepareCreateAllowedDomainType struct {
	ID   string
	Name string
}

// ListAllowedDomainTypesForAssetType returns the deduped list of domain
// type IDs the asset type can be created in, walking the parent chain
// when needed. Subtypes whose own assignments have empty domainTypes
// (inherit-sentinel) inherit their parent's allowed types — e.g.
// Acronym → Business Term → Glossary, Business Asset Domain.
func ListAllowedDomainTypesForAssetType(ctx context.Context, client *http.Client, assetTypeID string) ([]PrepareCreateAllowedDomainType, error) {
	chain, err := fetchAssignmentChain(ctx, client, assetTypeID)
	if err != nil {
		return nil, err
	}

	seen := make(map[string]struct{})
	out := make([]PrepareCreateAllowedDomainType, 0)
	for _, node := range chain {
		for _, a := range node.raws {
			for _, dt := range a.DomainTypes {
				if _, ok := seen[dt.ID]; ok {
					continue
				}
				seen[dt.ID] = struct{}{}
				out = append(out, PrepareCreateAllowedDomainType{ID: dt.ID, Name: dt.Name})
			}
		}
	}
	return out, nil
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
