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

// EditAssetCore is the slim view of an asset returned by GET /rest/2.0/assets/{id}
// that the edit_asset tool needs for validation and dispatch.
type EditAssetCore struct {
	ID          string              `json:"id"`
	Name        string              `json:"name"`
	DisplayName string              `json:"displayName,omitempty"`
	Type        EditAssetTypeRef    `json:"type"`
	Domain      EditAssetDomainRef  `json:"domain"`
	Status      *EditAssetStatusRef `json:"status,omitempty"`
}

// EditAssetTypeRef is a reference to an asset type.
type EditAssetTypeRef struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// EditAssetDomainRef is a reference to a domain on an asset.
type EditAssetDomainRef struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// EditAssetStatusRef is a reference to the asset's status.
type EditAssetStatusRef struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// EditAssetDomainDetails is the view of a domain that exposes its domain type,
// returned by GET /rest/2.0/domains/{id}. Needed to scope the assignment.
type EditAssetDomainDetails struct {
	ID   string                  `json:"id"`
	Name string                  `json:"name"`
	Type *EditAssetDomainTypeRef `json:"type,omitempty"`
}

// EditAssetDomainTypeRef is a reference to a domain type.
type EditAssetDomainTypeRef struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// EditAssetAttributeInstance is a single attribute value on an asset,
// returned by GET /rest/2.0/attributes?assetId=....
type EditAssetAttributeInstance struct {
	ID    string                     `json:"id"`
	Type  EditAssetAttributeTypeRef  `json:"type"`
	Asset EditAssetAttributeAssetRef `json:"asset"`
	Value string                     `json:"value"`
}

// EditAssetAttributeTypeRef is a reference to an attribute type on an instance.
type EditAssetAttributeTypeRef struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// EditAssetAttributeAssetRef is a reference to the owning asset.
type EditAssetAttributeAssetRef struct {
	ID string `json:"id"`
}

// editAssetAttributesList is the paginated wrapper returned by
// GET /rest/2.0/attributes.
type editAssetAttributesList struct {
	Total   int                          `json:"total"`
	Offset  int                          `json:"offset"`
	Limit   int                          `json:"limit"`
	Results []EditAssetAttributeInstance `json:"results"`
}

// EditAssetAssignment is the scoped assignment for a (asset type, domain type)
// pair — lists which attribute and relation types are valid for assets of
// this shape. This is the public shape the edit_asset tool consumes; it is
// built up from Collibra's raw assignment response by GetAssignmentForAssetType.
type EditAssetAssignment struct {
	AssetType      EditAssetTypeRef                   `json:"assetType"`
	DomainType     *EditAssetDomainTypeRef            `json:"domainType,omitempty"`
	AttributeTypes []EditAssetAssignmentAttributeType `json:"attributeTypes"`
	RelationTypes  []EditAssetAssignmentRelationType  `json:"relationTypes,omitempty"`
}

// EditAssetAssignmentRelationType is a relation type allowed by a scoped
// assignment, in the direction where the edited asset is the source (head).
// Role is the forward name (e.g. "synonym"); CoRole is the reverse name.
type EditAssetAssignmentRelationType struct {
	ID         string            `json:"id"`
	Role       string            `json:"role"`
	CoRole     string            `json:"coRole,omitempty"`
	SourceType *EditAssetTypeRef `json:"sourceType,omitempty"`
	TargetType *EditAssetTypeRef `json:"targetType,omitempty"`
}

// --- Raw assignment response shape (Collibra's wire format) ----------------

type rawAssignmentResponse struct {
	ID                  string                        `json:"id"`
	AssetType           EditAssetTypeRef              `json:"assetType"`
	DomainTypes         []EditAssetDomainTypeRef      `json:"domainTypes"`
	CharacteristicTypes []rawAssignmentCharacteristic `json:"characteristicTypes"`
}

type rawAssignmentCharacteristic struct {
	ID                                      string                      `json:"id"`
	MinimumOccurrences                      int                         `json:"minimumOccurrences"`
	RoleDirection                           string                      `json:"roleDirection,omitempty"`
	AttributeType                           *rawAssignmentAttributeType `json:"attributeType,omitempty"`
	RelationType                            *rawAssignmentRelationType  `json:"relationType,omitempty"`
	AssignedCharacteristicTypeDiscriminator string                      `json:"assignedCharacteristicTypeDiscriminator"`
}

type rawAssignmentAttributeType struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	PublicID string `json:"publicId,omitempty"`
	Kind     string `json:"resourceType,omitempty"`
}

type rawAssignmentRelationType struct {
	ID         string            `json:"id"`
	Role       string            `json:"role"`
	CoRole     string            `json:"coRole,omitempty"`
	SourceType *EditAssetTypeRef `json:"sourceType,omitempty"`
	TargetType *EditAssetTypeRef `json:"targetType,omitempty"`
}

// EditAssetAssignmentAttributeType is an attribute type allowed by a scoped
// assignment, with its full name and (optional) constraints.
type EditAssetAssignmentAttributeType struct {
	ID          string                          `json:"id"`
	Name        string                          `json:"name"`
	Kind        string                          `json:"kind,omitempty"`
	Required    bool                            `json:"required,omitempty"`
	Constraints *EditAssetAssignmentConstraints `json:"constraints,omitempty"`
}

// EditAssetAssignmentConstraints captures attribute type constraints used to
// validate operation values before any writes.
type EditAssetAssignmentConstraints struct {
	MinLength *int     `json:"minLength,omitempty"`
	MaxLength *int     `json:"maxLength,omitempty"`
	Min       *float64 `json:"min,omitempty"`
	Max       *float64 `json:"max,omitempty"`
}

// EditAssetPatchRequest is the body for PATCH /rest/2.0/assets/{id} — only the
// fields allowed by update_property (name, displayName, statusId).
type EditAssetPatchRequest struct {
	Name        *string `json:"name,omitempty"`
	DisplayName *string `json:"displayName,omitempty"`
	StatusID    *string `json:"statusId,omitempty"`
}

// EditAssetPatchAttributeRequest is the body for PATCH /rest/2.0/attributes/{id}.
type EditAssetPatchAttributeRequest struct {
	Value string `json:"value"`
}

// GetAssetCore fetches the core asset shape needed by the edit_asset tool.
func GetAssetCore(ctx context.Context, client *http.Client, assetID string) (*EditAssetCore, error) {
	reqURL := fmt.Sprintf("/rest/2.0/assets/%s", url.PathEscape(assetID))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("get asset: building request: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("get asset: sending request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("asset %q not found", assetID)
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("get asset: status %d: %s", resp.StatusCode, string(body))
	}

	var result EditAssetCore
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("get asset: decoding response: %w", err)
	}
	return &result, nil
}

// GetDomainDetails fetches a domain including its domain type reference, used
// to scope the assignment lookup.
func GetDomainDetails(ctx context.Context, client *http.Client, domainID string) (*EditAssetDomainDetails, error) {
	reqURL := fmt.Sprintf("/rest/2.0/domains/%s", url.PathEscape(domainID))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("get domain: building request: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("get domain: sending request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("domain %q not found", domainID)
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("get domain: status %d: %s", resp.StatusCode, string(body))
	}

	var result EditAssetDomainDetails
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("get domain: decoding response: %w", err)
	}
	return &result, nil
}

// ListAttributesForAsset fetches all attribute instances on an asset.
// Pages are followed transparently so the caller gets the full list.
func ListAttributesForAsset(ctx context.Context, client *http.Client, assetID string) ([]EditAssetAttributeInstance, error) {
	const pageSize = 100
	var all []EditAssetAttributeInstance
	offset := 0
	for {
		params := url.Values{}
		params.Set("assetId", assetID)
		params.Set("limit", fmt.Sprintf("%d", pageSize))
		params.Set("offset", fmt.Sprintf("%d", offset))

		reqURL := fmt.Sprintf("/rest/2.0/attributes?%s", params.Encode())
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
		if err != nil {
			return nil, fmt.Errorf("list attributes: building request: %w", err)
		}
		req.Header.Set("Accept", "application/json")

		resp, err := client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("list attributes: sending request: %w", err)
		}
		body, readErr := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if readErr != nil {
			return nil, fmt.Errorf("list attributes: reading response: %w", readErr)
		}
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("list attributes: status %d: %s", resp.StatusCode, string(body))
		}
		var page editAssetAttributesList
		if err := json.Unmarshal(body, &page); err != nil {
			return nil, fmt.Errorf("list attributes: decoding response: %w", err)
		}
		all = append(all, page.Results...)
		if len(page.Results) < pageSize || len(all) >= page.Total {
			break
		}
		offset += pageSize
	}
	return all, nil
}

// GetAssignmentForAssetType returns the scoped assignment for an (asset type,
// domain type) pair, listing valid attribute and relation types. Collibra's
// endpoint returns an array — typically one entry per matching scope. We
// merge attribute and relation types across the returned entries; if a
// domainTypeID was supplied, we filter to entries that match it (when present
// on the entry), otherwise we use everything returned.
func GetAssignmentForAssetType(ctx context.Context, client *http.Client, assetTypeID, domainTypeID string) (*EditAssetAssignment, error) {
	params := url.Values{}
	if domainTypeID != "" {
		params.Set("domainTypeId", domainTypeID)
	}
	reqURL := fmt.Sprintf("/rest/2.0/assignments/assetType/%s", url.PathEscape(assetTypeID))
	if q := params.Encode(); q != "" {
		reqURL += "?" + q
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("get assignment: building request: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("get assignment: sending request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("no assignment for asset type %q (domain type %q)", assetTypeID, domainTypeID)
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("get assignment: status %d: %s", resp.StatusCode, string(body))
	}

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("get assignment: reading response: %w", err)
	}

	// Collibra returns assignments as a top-level array; tolerate a single
	// object too just in case.
	var list []rawAssignmentResponse
	if err := json.Unmarshal(respBody, &list); err != nil {
		var single rawAssignmentResponse
		if err2 := json.Unmarshal(respBody, &single); err2 != nil {
			return nil, fmt.Errorf("get assignment: decoding response: %w", err)
		}
		list = []rawAssignmentResponse{single}
	}

	merged := EditAssetAssignment{
		AssetType: EditAssetTypeRef{ID: assetTypeID},
	}
	for _, a := range list {
		// If the caller passed a domainTypeID, skip assignments whose
		// domainTypes don't include it.
		if domainTypeID != "" && len(a.DomainTypes) > 0 {
			matched := false
			for _, dt := range a.DomainTypes {
				if dt.ID == domainTypeID {
					matched = true
					break
				}
			}
			if !matched {
				continue
			}
		}
		if merged.DomainType == nil && len(a.DomainTypes) > 0 {
			dt := a.DomainTypes[0]
			merged.DomainType = &dt
		}
		for _, ct := range a.CharacteristicTypes {
			switch ct.AssignedCharacteristicTypeDiscriminator {
			case "AttributeType":
				if ct.AttributeType == nil {
					continue
				}
				merged.AttributeTypes = append(merged.AttributeTypes, EditAssetAssignmentAttributeType{
					ID:       ct.AttributeType.ID,
					Name:     ct.AttributeType.Name,
					Kind:     ct.AttributeType.Kind,
					Required: ct.MinimumOccurrences >= 1,
				})
			case "RelationType":
				if ct.RelationType == nil {
					continue
				}
				// Only register the direction where the edited asset is the
				// source (head). Collibra emits both directions as separate
				// characteristic entries; TO_TARGET = forward (asset->target).
				if ct.RoleDirection != "" && ct.RoleDirection != "TO_TARGET" {
					continue
				}
				merged.RelationTypes = append(merged.RelationTypes, EditAssetAssignmentRelationType{
					ID:         ct.RelationType.ID,
					Role:       ct.RelationType.Role,
					CoRole:     ct.RelationType.CoRole,
					SourceType: ct.RelationType.SourceType,
					TargetType: ct.RelationType.TargetType,
				})
			}
		}
	}
	return &merged, nil
}

// PatchAsset updates the whitelisted core fields (name, displayName, statusId)
// on an asset via PATCH /rest/2.0/assets/{id}.
func PatchAsset(ctx context.Context, client *http.Client, assetID string, payload EditAssetPatchRequest) (*EditAssetCore, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("patch asset: marshaling request: %w", err)
	}
	reqURL := fmt.Sprintf("/rest/2.0/assets/%s", url.PathEscape(assetID))
	req, err := http.NewRequestWithContext(ctx, http.MethodPatch, reqURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("patch asset: building request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("patch asset: sending request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("patch asset: reading response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("patch asset: status %d: %s", resp.StatusCode, string(respBody))
	}

	var result EditAssetCore
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("patch asset: decoding response: %w", err)
	}
	return &result, nil
}

// PatchAttributeValue updates a single attribute instance's value via
// PATCH /rest/2.0/attributes/{id}.
func PatchAttributeValue(ctx context.Context, client *http.Client, attributeID, value string) (*EditAssetAttributeInstance, error) {
	body, err := json.Marshal(EditAssetPatchAttributeRequest{Value: value})
	if err != nil {
		return nil, fmt.Errorf("patch attribute: marshaling request: %w", err)
	}
	reqURL := fmt.Sprintf("/rest/2.0/attributes/%s", url.PathEscape(attributeID))
	req, err := http.NewRequestWithContext(ctx, http.MethodPatch, reqURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("patch attribute: building request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("patch attribute: sending request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("patch attribute: reading response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("patch attribute: status %d: %s", resp.StatusCode, string(respBody))
	}

	var result EditAssetAttributeInstance
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("patch attribute: decoding response: %w", err)
	}
	return &result, nil
}

// CreateAttributeOnAsset adds a single attribute instance via POST /rest/2.0/attributes.
// It mirrors CreateAttribute in create_asset_client.go but returns the richer
// EditAssetAttributeInstance shape for diff tracking.
func CreateAttributeOnAsset(ctx context.Context, client *http.Client, assetID, attrTypeID, value string) (*EditAssetAttributeInstance, error) {
	body, err := json.Marshal(CreateAttributeRequest{
		AssetID: assetID,
		TypeID:  attrTypeID,
		Value:   value,
	})
	if err != nil {
		return nil, fmt.Errorf("create attribute: marshaling request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "/rest/2.0/attributes", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create attribute: building request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("create attribute: sending request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("create attribute: reading response: %w", err)
	}
	if resp.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf("create attribute: status %d: %s", resp.StatusCode, string(respBody))
	}

	var result EditAssetAttributeInstance
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("create attribute: decoding response: %w", err)
	}
	return &result, nil
}

// DeleteAttribute removes a single attribute instance via DELETE /rest/2.0/attributes/{id}.
func DeleteAttribute(ctx context.Context, client *http.Client, attributeID string) error {
	reqURL := fmt.Sprintf("/rest/2.0/attributes/%s", url.PathEscape(attributeID))
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, reqURL, nil)
	if err != nil {
		return fmt.Errorf("delete attribute: building request: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("delete attribute: sending request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("delete attribute: status %d: %s", resp.StatusCode, string(body))
	}
	return nil
}

// EditAssetCreateRelationRequest is the body for POST /rest/2.0/relations.
type EditAssetCreateRelationRequest struct {
	SourceID string `json:"sourceId"`
	TargetID string `json:"targetId"`
	TypeID   string `json:"typeId"`
}

// EditAssetRelation is a relation instance between two assets.
type EditAssetRelation struct {
	ID     string                     `json:"id"`
	Type   EditAssetTypeRef           `json:"type"`
	Source EditAssetAttributeAssetRef `json:"source"`
	Target EditAssetAttributeAssetRef `json:"target"`
}

// CreateRelation posts a new relation via POST /rest/2.0/relations. The source
// asset is the head; target is the tail.
func CreateRelation(ctx context.Context, client *http.Client, payload EditAssetCreateRelationRequest) (*EditAssetRelation, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("create relation: marshaling request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "/rest/2.0/relations", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create relation: building request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("create relation: sending request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("create relation: reading response: %w", err)
	}
	if resp.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf("create relation: status %d: %s", resp.StatusCode, string(respBody))
	}

	var result EditAssetRelation
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("create relation: decoding response: %w", err)
	}
	return &result, nil
}

// EditAssetAddTagsRequest is the body for POST /rest/2.0/assets/{id}/tags.
// Collibra expects the field to be named "tagNames" — sending "tags" is
// silently ignored by the API and yields a "tagNames may not be null" 400.
type EditAssetAddTagsRequest struct {
	TagNames []string `json:"tagNames"`
}

// AddTagsToAsset appends one or more tags to an asset without replacing
// existing tags (incremental, matching the "prefer incremental" AC).
func AddTagsToAsset(ctx context.Context, client *http.Client, assetID string, tags []string) error {
	body, err := json.Marshal(EditAssetAddTagsRequest{TagNames: tags})
	if err != nil {
		return fmt.Errorf("add tags: marshaling request: %w", err)
	}
	reqURL := fmt.Sprintf("/rest/2.0/assets/%s/tags", url.PathEscape(assetID))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, reqURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("add tags: building request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("add tags: sending request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusNoContent {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("add tags: status %d: %s", resp.StatusCode, string(respBody))
	}
	return nil
}

// EditAssetRole is a resource role (e.g. Steward, Owner) that can be assigned
// to an asset via responsibilities.
type EditAssetRole struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// EditAssetStatus is a status (e.g. Candidate, Accepted, Obsolete) that can be
// assigned to an asset via update_property statusId.
type EditAssetStatus struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// editAssetStatusesList is the paginated wrapper returned by GET /rest/2.0/statuses.
type editAssetStatusesList struct {
	Total   int               `json:"total"`
	Offset  int               `json:"offset"`
	Limit   int               `json:"limit"`
	Results []EditAssetStatus `json:"results"`
}

// ListStatuses returns all asset statuses defined in Collibra. Used to resolve
// a status name (e.g. "Candidate") to its UUID before patching an asset.
func ListStatuses(ctx context.Context, client *http.Client) ([]EditAssetStatus, error) {
	const pageSize = 1000
	var all []EditAssetStatus
	offset := 0
	for {
		params := url.Values{}
		params.Set("limit", fmt.Sprintf("%d", pageSize))
		params.Set("offset", fmt.Sprintf("%d", offset))

		reqURL := "/rest/2.0/statuses?" + params.Encode()
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
		if err != nil {
			return nil, fmt.Errorf("list statuses: building request: %w", err)
		}
		req.Header.Set("Accept", "application/json")

		resp, err := client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("list statuses: sending request: %w", err)
		}
		body, readErr := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if readErr != nil {
			return nil, fmt.Errorf("list statuses: reading response: %w", readErr)
		}
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("list statuses: status %d: %s", resp.StatusCode, string(body))
		}
		var page editAssetStatusesList
		if err := json.Unmarshal(body, &page); err != nil {
			return nil, fmt.Errorf("list statuses: decoding response: %w", err)
		}
		all = append(all, page.Results...)
		if len(page.Results) < pageSize || len(all) >= page.Total {
			break
		}
		offset += pageSize
	}
	return all, nil
}

// editAssetRolesList is the paginated wrapper returned by GET /rest/2.0/roles.
type editAssetRolesList struct {
	Total   int             `json:"total"`
	Offset  int             `json:"offset"`
	Limit   int             `json:"limit"`
	Results []EditAssetRole `json:"results"`
}

// ListRoles returns all resource roles defined in Collibra. Callers use this
// to resolve a role name (e.g. "Steward") to its UUID before creating a
// responsibility. The full list is typically small and fits in a single page.
func ListRoles(ctx context.Context, client *http.Client) ([]EditAssetRole, error) {
	const pageSize = 1000
	var all []EditAssetRole
	offset := 0
	for {
		params := url.Values{}
		params.Set("limit", fmt.Sprintf("%d", pageSize))
		params.Set("offset", fmt.Sprintf("%d", offset))

		reqURL := "/rest/2.0/roles?" + params.Encode()
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
		if err != nil {
			return nil, fmt.Errorf("list roles: building request: %w", err)
		}
		req.Header.Set("Accept", "application/json")

		resp, err := client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("list roles: sending request: %w", err)
		}
		body, readErr := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if readErr != nil {
			return nil, fmt.Errorf("list roles: reading response: %w", readErr)
		}
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("list roles: status %d: %s", resp.StatusCode, string(body))
		}
		var page editAssetRolesList
		if err := json.Unmarshal(body, &page); err != nil {
			return nil, fmt.Errorf("list roles: decoding response: %w", err)
		}
		all = append(all, page.Results...)
		if len(page.Results) < pageSize || len(all) >= page.Total {
			break
		}
		offset += pageSize
	}
	return all, nil
}

// EditAssetUser is a Collibra user, used to resolve a username or email
// to the user's UUID before assigning responsibilities.
type EditAssetUser struct {
	ID           string `json:"id"`
	UserName     string `json:"userName,omitempty"`
	EmailAddress string `json:"emailAddress,omitempty"`
	FirstName    string `json:"firstName,omitempty"`
	LastName     string `json:"lastName,omitempty"`
}

// editAssetUsersList is the paginated wrapper returned by GET /rest/2.0/users.
type editAssetUsersList struct {
	Total   int             `json:"total"`
	Offset  int             `json:"offset"`
	Limit   int             `json:"limit"`
	Results []EditAssetUser `json:"results"`
}

// FindUserByUsername returns the first user matching a username, or nil if
// none exists. Used by set_responsibility to resolve "jane.smith" to a UUID.
func FindUserByUsername(ctx context.Context, client *http.Client, username string) (*EditAssetUser, error) {
	params := url.Values{}
	params.Set("name", username)
	params.Set("limit", "1")
	return findUserBy(ctx, client, params)
}

// FindUserByEmail returns the first user matching an email address, or nil
// if none exists.
func FindUserByEmail(ctx context.Context, client *http.Client, email string) (*EditAssetUser, error) {
	params := url.Values{}
	params.Set("emailAddress", email)
	params.Set("limit", "1")
	return findUserBy(ctx, client, params)
}

func findUserBy(ctx context.Context, client *http.Client, params url.Values) (*EditAssetUser, error) {
	reqURL := "/rest/2.0/users?" + params.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("find user: building request: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("find user: sending request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("find user: reading response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("find user: status %d: %s", resp.StatusCode, string(body))
	}
	var page editAssetUsersList
	if err := json.Unmarshal(body, &page); err != nil {
		return nil, fmt.Errorf("find user: decoding response: %w", err)
	}
	if len(page.Results) == 0 {
		return nil, nil
	}
	user := page.Results[0]
	return &user, nil
}

// EditAssetCreateResponsibilityRequest is the body for POST /rest/2.0/responsibilities.
type EditAssetCreateResponsibilityRequest struct {
	RoleID     string `json:"roleId"`
	OwnerID    string `json:"ownerId"`
	ResourceID string `json:"resourceId"`
}

// EditAssetResponsibility is a responsibility instance linking a role, an
// owner (user or group), and an asset.
type EditAssetResponsibility struct {
	ID         string `json:"id"`
	RoleID     string `json:"roleId,omitempty"`
	OwnerID    string `json:"ownerId,omitempty"`
	ResourceID string `json:"resourceId,omitempty"`
}

// CreateResponsibility assigns a role to an owner for an asset via
// POST /rest/2.0/responsibilities. This is incremental — it doesn't replace
// other responsibilities on the asset.
func CreateResponsibility(ctx context.Context, client *http.Client, payload EditAssetCreateResponsibilityRequest) (*EditAssetResponsibility, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("create responsibility: marshaling request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "/rest/2.0/responsibilities", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create responsibility: building request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("create responsibility: sending request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("create responsibility: reading response: %w", err)
	}
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("create responsibility: status %d: %s", resp.StatusCode, string(respBody))
	}

	var result EditAssetResponsibility
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("create responsibility: decoding response: %w", err)
	}
	return &result, nil
}

// EditAssetBulkPatchAttributeItem is one row of PATCH /rest/2.0/attributes/bulk.
type EditAssetBulkPatchAttributeItem struct {
	ID    string `json:"id"`
	Value string `json:"value"`
}

// BulkCreateAttributes creates multiple attribute instances in one round trip
// via POST /rest/2.0/attributes/bulk. Treated as all-or-nothing: if the whole
// batch fails, the caller marks every affected op as failed with the batch
// error. Partial-row failures from Collibra aren't parsed individually.
func BulkCreateAttributes(ctx context.Context, client *http.Client, items []CreateAttributeRequest) ([]EditAssetAttributeInstance, error) {
	body, err := json.Marshal(items)
	if err != nil {
		return nil, fmt.Errorf("bulk create attributes: marshaling request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "/rest/2.0/attributes/bulk", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("bulk create attributes: building request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("bulk create attributes: sending request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("bulk create attributes: reading response: %w", err)
	}
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("bulk create attributes: status %d: %s", resp.StatusCode, string(respBody))
	}
	var result []EditAssetAttributeInstance
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("bulk create attributes: decoding response: %w", err)
	}
	return result, nil
}

// BulkPatchAttributes updates multiple attribute instances in one round trip
// via PATCH /rest/2.0/attributes/bulk. All-or-nothing, same rationale as
// BulkCreateAttributes.
func BulkPatchAttributes(ctx context.Context, client *http.Client, items []EditAssetBulkPatchAttributeItem) ([]EditAssetAttributeInstance, error) {
	body, err := json.Marshal(items)
	if err != nil {
		return nil, fmt.Errorf("bulk patch attributes: marshaling request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPatch, "/rest/2.0/attributes/bulk", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("bulk patch attributes: building request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("bulk patch attributes: sending request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("bulk patch attributes: reading response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("bulk patch attributes: status %d: %s", resp.StatusCode, string(respBody))
	}
	var result []EditAssetAttributeInstance
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("bulk patch attributes: decoding response: %w", err)
	}
	return result, nil
}

// BulkCreateRelations creates multiple relations in one round trip via
// POST /rest/2.0/relations/bulk.
func BulkCreateRelations(ctx context.Context, client *http.Client, items []EditAssetCreateRelationRequest) ([]EditAssetRelation, error) {
	body, err := json.Marshal(items)
	if err != nil {
		return nil, fmt.Errorf("bulk create relations: marshaling request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "/rest/2.0/relations/bulk", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("bulk create relations: building request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("bulk create relations: sending request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("bulk create relations: reading response: %w", err)
	}
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("bulk create relations: status %d: %s", resp.StatusCode, string(respBody))
	}
	var result []EditAssetRelation
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("bulk create relations: decoding response: %w", err)
	}
	return result, nil
}

// DeleteRelation removes a relation via DELETE /rest/2.0/relations/{id}.
func DeleteRelation(ctx context.Context, client *http.Client, relationID string) error {
	reqURL := fmt.Sprintf("/rest/2.0/relations/%s", url.PathEscape(relationID))
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, reqURL, nil)
	if err != nil {
		return fmt.Errorf("delete relation: building request: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("delete relation: sending request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusNotFound {
		return fmt.Errorf("relation %q not found", relationID)
	}
	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("delete relation: status %d: %s", resp.StatusCode, string(body))
	}
	return nil
}
