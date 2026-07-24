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
// Term) and is the key for the assignment walk-up: when an asset type has
// no assignment of its own, resolution walks up to the first ancestor that
// has one (see selectScopedAssignment).
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
	RelationTypeID string
	Role           string
	CoRole         string
	// Direction is "SOURCE_TO_TARGET" or "TARGET_TO_SOURCE" — describing
	// which side of the relation the asset being created sits on.
	Direction string
	// TargetType is the asset type on the other side of the relation.
	TargetType *PrepareCreateAssetType
}

// PrepareCreateScopedAssignment is the single effective assignment selected
// for a given (assetType, domain) pair — the one assignment the platform
// would apply, never a merge across scopes or hierarchy levels. Its attribute
// and relation slots are exactly that assignment's characteristics.
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
//
// The two inheritance fields carry characteristics the asset type gains through
// Traits (orthogonal to the scope selection above — they enrich whichever
// assignment was selected, they are not a cross-assignment union). Both are
// merged into the resolved assignment; see emitAssignmentCharacteristics.
type rawScopedAssignment struct {
	ID                                   string                                    `json:"id"`
	DomainTypes                          []rawAssignmentResourceRef                `json:"domainTypes"`
	AssignedCharacteristicTypeReferences []rawAssignedCharacteristicTypeReference  `json:"assignedCharacteristicTypeReferences"`
	CharacteristicTypes                  []rawAssignmentCharacteristicTypeMetadata `json:"characteristicTypes"`
	Scope                                *rawAssignmentScope                       `json:"scope"`
	// TraitAssignmentInheritances are Traits applied DIRECTLY to this asset
	// type; each entry carries its own characteristics exactly like the
	// top-level assignment does.
	TraitAssignmentInheritances []rawTraitAssignmentInheritance `json:"traitAssignmentInheritances"`
	// AssignmentInheritances are Traits applied to an ANCESTOR asset type; each
	// entry carries no characteristics directly — they sit one level deeper
	// under the entry's nested TraitAssignmentInheritances.
	AssignmentInheritances []rawAssignmentInheritance `json:"assignmentInheritances"`
}

// rawTraitAssignmentInheritance is one Trait's contribution of characteristics:
// its own assignedCharacteristicTypeReferences (membership + min/max) and its
// own characteristicTypes (relation role / co-role / direction / target), read
// and joined per entry exactly as the top-level assignment's are.
type rawTraitAssignmentInheritance struct {
	AssignedCharacteristicTypeReferences []rawAssignedCharacteristicTypeReference  `json:"assignedCharacteristicTypeReferences"`
	CharacteristicTypes                  []rawAssignmentCharacteristicTypeMetadata `json:"characteristicTypes"`
}

// rawAssignmentInheritance is a Trait applied to an ancestor asset type. It
// holds no characteristics of its own — its contribution sits under the nested
// traitAssignmentInheritances list (one entry per Trait on that ancestor).
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
	ResourceType          string `json:"resourceType"`
	ResourceDiscriminator string `json:"resourceDiscriminator"`
}

type rawAssignedCharacteristicTypeReference struct {
	ID                        string                   `json:"id"`
	AssignedResourceReference rawAssignmentResourceRef `json:"assignedResourceReference"`
	AssignedResourcePublicID  string                   `json:"assignedResourcePublicId"`
	MinimumOccurrences        int                      `json:"minimumOccurrences"`
	MaximumOccurrences        *int                     `json:"maximumOccurrences"`
	// RoleDirection ("TO_TARGET" / "TO_SOURCE") distinguishes the two sides of a
	// relation type assigned in both directions. It is part of the relation
	// dedup key so both directions survive as distinct characteristics; it is
	// empty for attributes.
	RoleDirection string `json:"roleDirection,omitempty"`
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

// maxAncestorDepth caps how many ancestor levels either create-path walk
// traverses — both the asset-type parent walk-up (fetchAssignmentLevels) and
// the domain community-ancestry walk (fetchDomainCommunityAncestors) share
// this single bound. The asset-type hierarchy and the community tree are
// acyclic by platform contract, but direct DB manipulation could violate that
// and spin us forever, so the bound stays as an explicit safeguard. 50 is far
// above the deepest real or QA-generated tree, so a legitimate hierarchy is
// never truncated.
const maxAncestorDepth = 50

// GetScopedAssignment returns the single effective assignment for an
// (assetType, domain) pair, resolved the way the Collibra platform resolves
// it: locate one hierarchy level (the asset type's own assignment set, else
// the first ancestor that has any assignment), then select exactly one
// assignment from that level by scope tier — domain-direct > community >
// global — never a merge. See selectScopedAssignment.
//
// Scope coverage decides which scoped assignments are in play: a scoped
// assignment (scope != null) governs only the domains its scope covers,
// tagged by tier so selection can honour the priority. See resolveCoveredScopes.
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

// assignmentLevel is one level of the asset type's parent hierarchy — the
// type itself plus its assignment set. Splitting the fetch (impure) from the
// selection (pure) lets us unit-test the selection logic without an HTTP server.
type assignmentLevel struct {
	assetType *PrepareCreateAssetType
	raws      []rawScopedAssignment
}

// fetchAssignmentLevels walks the asset type up its parent hierarchy —
// type → parent → grandparent → … — fetching each level's /assetTypes/{id}
// (for parent info) and /assignments/assetType/{id}. Every level fetched is
// recorded, including those with no assignment, so the pure selection step can
// locate the first level that actually carries one. Stops at the root (no
// parent) or at maxAncestorDepth.
func fetchAssignmentLevels(ctx context.Context, client *http.Client, assetTypeID string) ([]assignmentLevel, error) {
	var levels []assignmentLevel
	currentID := assetTypeID
	seen := make(map[string]struct{})
	for depth := 0; depth < maxAncestorDepth; depth++ {
		if _, looped := seen[currentID]; looped {
			break
		}
		seen[currentID] = struct{}{}

		at, err := GetAssetTypeByID(ctx, client, currentID)
		if err != nil {
			// Tolerate parent-fetch errors: if we can't get the parent's
			// info we still return what we have so far. Discovery falls
			// back to "no compatible domains" only if the entire walk
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
		levels = append(levels, assignmentLevel{assetType: at, raws: raws})

		if at.Parent == nil || at.Parent.ID == "" {
			break
		}
		currentID = at.Parent.ID
	}
	return levels, nil
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

// scopeTier records how a scope covers the target domain. The platform's
// selection priority is domain-direct > community > global; carrying the tier
// (rather than a bare "covered" flag) is what lets a later change teach the
// selection step that ordering. Today's selection still treats any covered
// scope equally — only the result shape carries the tier so far.
type scopeTier int

const (
	// scopeTierDomainDirect: the scope lists the target domain directly.
	scopeTierDomainDirect scopeTier = iota
	// scopeTierCommunity: the scope lists the domain's own community or any
	// ancestor community of it.
	scopeTierCommunity
)

// resolveCoveredScopes returns, for each scope — among the levels' scoped
// assignments — that covers the target domain, the tier by which it covers:
// domain-direct (the scope lists the domain directly) or community (the scope
// lists the domain's own community or any ancestor). selectScopedAssignment
// consumes the tier to apply the domain-direct > community priority. The
// assignment listing embeds each scope's membership; community ancestry is
// fetched lazily, so the common global-only case costs no extra calls.
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

// fetchDomainCommunityAncestors returns the IDs of every community the
// domain sits under: its own community plus each ancestor up to the root.
// A scope that lists any of these communities covers the domain.
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

// getJSON is the shared GET-and-decode plumbing for the small lookups
// above that need no custom status handling.
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

// selectScopedAssignment reproduces the platform's create-time resolution:
// locate a single hierarchy level, then select exactly one assignment from it —
// never a union across scopes or levels.
//
//   - Locate one level: use the asset type's own assignment set (level 0); if
//     it has none, walk up to the first ancestor level that carries any
//     assignment. Exactly one level is consumed; there is no cross-level union.
//     If no level carries any assignment, none is located and the asset type is
//     creatable nowhere.
//   - Select one assignment from the located level: the level carries exactly
//     one global (scope == null) assignment and zero or more scoped ones (a
//     platform invariant chip relies on). Prefer, in strict order, a
//     domain-direct covering scoped assignment, a community covering scoped
//     assignment, then the always-present global one. Tiers are never combined;
//     a same-tier tie resolves first-found. The target domain type plays no
//     part in selection.
//
// The selected assignment's characteristics are emitted whole. The domain type
// is used only as a post-selection creatability gate: the type is creatable in
// the domain only if the selected assignment lists domainTypeID, otherwise this
// returns a "not allowed" error.
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
		// A located level always carries the global (unscoped) assignment by
		// platform invariant, so the tier fallback can't come up empty; guard
		// defensively rather than trust the invariant blindly.
		return nil, fmt.Errorf("no assignments found")
	}

	// Creatability gate — the only use of the domain type, applied to the
	// already-selected assignment (never to selection itself).
	if !containsResourceRef(selected.DomainTypes, domainTypeID) {
		return nil, fmt.Errorf("no scoped assignment found for asset type in this domain type %q", domainTypeID)
	}

	return emitAssignmentCharacteristics(selected), nil
}

// selectByTier picks the single governing assignment from one level's set,
// applying the platform priority domain-direct > community > global. Within a
// tier the first match wins. Returns false only when the level is empty (the
// caller guarantees it is not).
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

// characteristicSource is one contributor of characteristics to the resolved
// assignment: its assigned-characteristic references paired with the
// characteristicTypes metadata sidecar that carries each relation's role /
// co-role / direction / target. The selected assignment's own characteristics
// and each Trait inheritance are one source apiece.
type characteristicSource struct {
	refs []rawAssignedCharacteristicTypeReference
	meta []rawAssignmentCharacteristicTypeMetadata
}

// characteristicKey is the closest-wins shadowing key across merge sources: the
// resource id for an attribute, and the resource id + role direction for a
// relation (roleDirection empty for attributes). The two directions of a
// bidirectional relation type are therefore distinct keys and both survive.
type characteristicKey struct {
	resourceID    string
	roleDirection string
}

// characteristicSourcesFrom ranks an assignment's characteristic contributors in
// closest-wins order, so a duplicate found in a later source is shadowed by the
// earlier one (see emitAssignmentCharacteristics on the create path and
// mergeEditAssignments on the edit path — both share this ranking, differing only
// in the response type they unpack the four arguments from):
//
//  1. the assignment's own characteristics (own refs + meta);
//  2. Traits applied directly to this asset type (direct = traitAssignmentInheritances),
//     each carrying its own references + metadata;
//  3. Traits applied to an ancestor asset type (ancestor = assignmentInheritances),
//     whose characteristics sit one level deeper under each entry's nested
//     traitAssignmentInheritances.
func characteristicSourcesFrom(
	refs []rawAssignedCharacteristicTypeReference,
	meta []rawAssignmentCharacteristicTypeMetadata,
	direct []rawTraitAssignmentInheritance,
	ancestor []rawAssignmentInheritance,
) []characteristicSource {
	sources := []characteristicSource{{refs: refs, meta: meta}}
	for _, ti := range direct {
		sources = append(sources, characteristicSource{refs: ti.AssignedCharacteristicTypeReferences, meta: ti.CharacteristicTypes})
	}
	for _, ai := range ancestor {
		for _, ti := range ai.TraitAssignmentInheritances {
			sources = append(sources, characteristicSource{refs: ti.AssignedCharacteristicTypeReferences, meta: ti.CharacteristicTypes})
		}
	}
	return sources
}

// emitAssignmentCharacteristics decodes the selected assignment's attribute and
// relation characteristics into the resolved shape, merging its own
// characteristics with those inherited through Traits (directly and from
// ancestor types).
//
// Precedence is closest-wins: the sources are walked own > direct-trait >
// ancestor-trait, and the first occurrence of a characteristic's key supplies
// the whole characteristic — later duplicates are dropped entire, with no
// field-level blending. The dedup key is the resource id for an attribute and
// the resource id + roleDirection for a relation, so a relation type assigned in
// both directions keeps both entries (each direction is a distinct key).
func emitAssignmentCharacteristics(a rawScopedAssignment) *PrepareCreateScopedAssignment {
	out := &PrepareCreateScopedAssignment{AssignmentID: a.ID}
	seen := make(map[characteristicKey]struct{})
	for _, src := range characteristicSourcesFrom(a.AssignedCharacteristicTypeReferences, a.CharacteristicTypes, a.TraitAssignmentInheritances, a.AssignmentInheritances) {
		// Key the relation-metadata sidecar by its top-level LINE id, which the
		// reference's own top-level id joins against (see the join note below).
		metaByID := make(map[string]rawAssignmentCharacteristicTypeMetadata, len(src.meta))
		for _, m := range src.meta {
			metaByID[m.ID] = m
		}
		for _, ref := range src.refs {
			disc := ref.AssignedResourceReference.ResourceDiscriminator
			rt := ref.AssignedResourceReference.ResourceType
			// Derived relation types (DRTs) are computed/transitive relations the
			// user cannot create — never offer them. Guard explicitly on the
			// reference discriminator (the reliable signal). The empty-disc branch
			// covers the same case for the fallback path in isRelationTypeDiscriminator
			// (HasSuffix(rt, "RelationType")), which would otherwise match
			// "DerivedRelationType" and silently reintroduce them. Applies at every
			// source, including trait-inherited relations.
			if disc == "DerivedRelationType" || (disc == "" && rt == "DerivedRelationType") {
				continue
			}
			switch {
			case isAttributeTypeDiscriminator(disc, rt):
				// Attributes dedup on the resource id alone (no direction).
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
			case isRelationTypeDiscriminator(disc, rt):
				// Relations dedup on resource id + direction, so a relation type
				// assigned in both directions keeps both entries.
				key := characteristicKey{
					resourceID:    ref.AssignedResourceReference.ID,
					roleDirection: ref.RoleDirection,
				}
				if _, dup := seen[key]; dup {
					continue
				}
				seen[key] = struct{}{}
				// Join on the reference's own top-level LINE id (ref.ID ↔
				// characteristicTypes[].id) — the two lists correlate 1:1 on that
				// line id. AssignedResourceReference.ID is the relation-type resource
				// id (a different value): right for the emitted RelationTypeID, wrong
				// as the join key.
				meta := metaByID[ref.ID]
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
func isAttributeTypeDiscriminator(disc, resourceType string) bool {
	if disc == "" {
		return strings.HasSuffix(resourceType, "AttributeType")
	}
	return strings.HasSuffix(disc, "AttributeType")
}

// normalizeAttributeKind corrects a single platform bug in the attribute-kind
// discriminator: "DateTimeAttributeType" is a mistake — an attribute type that
// shouldn't exist, lingering from a years-old error. The frozen deprecated
// resourceType enum already collapses it to "DateAttributeType", so we surface
// that instead of leaking the bug to the model. This is the ONLY kind
// normalization — do not generalise it.
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
func isRelationTypeDiscriminator(disc, resourceType string) bool {
	if disc == "" {
		return strings.HasSuffix(resourceType, "RelationType")
	}
	return disc == "RelationType" || disc == "ComplexRelationType"
}

// PrepareCreateAllowedDomainType is one domain type an asset type can
// be created in. The set is deduped across the single located assignment
// level's assignments (an asset type may be allowed in multiple domain types).
type PrepareCreateAllowedDomainType struct {
	ID   string
	Name string
}

// ListAllowedDomainTypesForAssetType returns the deduped domain type IDs the
// asset type can be created in, read from the single located assignment level:
// the asset type's own assignment set, or — when it has none — the first
// ancestor level that carries any assignment (the same one-level walk-up
// selectScopedAssignment performs). It stops at that first level with any
// assignment whether or not that level lists domain types; ancestors beyond it
// are never merged in.
//
// An empty result means the asset type is creatable nowhere: either no level
// carried any assignment, or the located level's assignments all have empty
// domainTypes. An assignment with empty domainTypes admits NO domain type — it
// is not an "inherit from parent" signal, so an all-empty located level does
// not fall through to a parent. Global is defined by scope == null, not by
// empty domainTypes. Callers use empty-vs-non-empty as the creatability boolean
// (see NotAllowedMessage).
func ListAllowedDomainTypesForAssetType(ctx context.Context, client *http.Client, assetTypeID string) ([]PrepareCreateAllowedDomainType, error) {
	levels, err := fetchAssignmentLevels(ctx, client, assetTypeID)
	if err != nil {
		return nil, err
	}

	seen := make(map[string]struct{})
	out := make([]PrepareCreateAllowedDomainType, 0)
	for _, level := range levels {
		if len(level.raws) == 0 {
			continue // no assignment at this level — keep walking up
		}
		// First level with any assignment is authoritative — stop here. If
		// every assignment on it has empty domainTypes the result stays empty,
		// which is exactly the "creatable nowhere" verdict.
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

// NotAllowedMessage renders the user-facing "not allowed" message for a create
// that the creatability gate refused (GetScopedAssignment returned an error).
// It is the single raise site for that message so create_asset and
// prepare_create_asset stay at parity. Two branches, chosen by the creatability
// boolean "does the located assignment level list at least one domain type?":
//
//   - nowhere — the asset type can't be used to create an asset in ANY domain on
//     this instance. Two sub-cases collapse into this one verdict and message:
//     no assignment was located anywhere in the type's hierarchy, or a level was
//     located but every assignment on it has an empty domainTypes list. The
//     distinction is invisible in the UI and only an admin could explain it, so
//     it is irrelevant to the end user.
//   - not here — the type is creatable somewhere (the located level lists domain
//     types) but not in this domain: the assignment governing this domain
//     doesn't list the domain's type.
//
// The message deliberately never lists the allowed domain types. That set is
// scope-conditioned — one list per assignment at the located level (global plus
// each covering scope), each scope further expanded into the communities and
// domains it covers — so there is no single coherent list to put in front of a
// user (or model).
//
// The creatability boolean comes from ListAllowedDomainTypesForAssetType
// (non-empty ⇒ creatable somewhere). When that lookup itself errors we cannot
// prove "nowhere", so we fall back to the less absolute "not here" message.
//
// Stale-assignment edge case: this verdict can legitimately contradict what the
// user observes. A domain may already hold assets of this asset type because an
// admin removed the domain type from the governing assignment AFTER those assets
// were created — the existing assets are valid history, while the block on NEW
// creates is correct. The wording stays deliberately generic rather than
// guessing at that cause (only an admin could confirm it), so the terse message
// is a known choice, not an oversight.
func NotAllowedMessage(ctx context.Context, client *http.Client, assetTypeID, assetTypeName, domainName, domainTypeName string) string {
	creatableSomewhere := true // can't prove "nowhere" if the lookup fails
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
