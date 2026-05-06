// Package clients — dgc_catalogs_client.go fetches the DGC catalogs that
// the create-control flow consults to look up types by publicId / name:
//
//   - Asset types        /rest/2.0/assetTypes
//   - Relation types     /rest/2.0/relationTypes
//   - Attribute types    /rest/2.0/attributeTypes
//   - Statuses           /rest/2.0/statuses
//   - ManagedControl assignments  (already in control_tower_client.go)
//
// Each catalog has a "list-all" function (paginates the endpoint up to
// completion) backed by a 1-hour in-process cache. The list_* and find_*
// MCP tools read from the same cache, so a session warms each catalog
// at most once per hour against the target environment.

package clients

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

// catalogPageSize is the per-page limit when paginating list-all calls.
// 1000 is the documented maximum for these endpoints.
const catalogPageSize = 1000

// ===== Relation types =====

type RelationTypeDetails struct {
	ID          string                  `json:"id"`
	PublicID    string                  `json:"publicId,omitempty"`
	Description string                  `json:"description,omitempty"`
	Role        string                  `json:"role,omitempty"`
	CoRole      string                  `json:"coRole,omitempty"`
	System      bool                    `json:"system"`
	SourceType  *MetaTypeReference      `json:"sourceType,omitempty"`
	TargetType  *MetaTypeReference      `json:"targetType,omitempty"`
}

// MetaTypeReference is the projection of MetaNamedResourceReferenceImpl
// the catalogs return for relation source/target type references.
type MetaTypeReference struct {
	ID       string `json:"id"`
	Name     string `json:"name,omitempty"`
	PublicID string `json:"publicId,omitempty"`
}

type relationTypePagedResponse struct {
	Total   int64                 `json:"total"`
	Offset  int64                 `json:"offset"`
	Limit   int64                 `json:"limit"`
	Results []RelationTypeDetails `json:"results"`
}

var relationTypesCache catalogCache[[]RelationTypeDetails]

// ListAllRelationTypes returns every relation type in DGC, paginating
// through the endpoint internally. Result is cached for catalogCacheTTL.
func ListAllRelationTypes(ctx context.Context, client *http.Client) ([]RelationTypeDetails, error) {
	return relationTypesCache.get(ctx, client, fetchAllRelationTypes)
}

func fetchAllRelationTypes(ctx context.Context, client *http.Client) ([]RelationTypeDetails, error) {
	var out []RelationTypeDetails
	offset := 0
	for {
		endpoint := fmt.Sprintf("/rest/2.0/relationTypes?offset=%d&limit=%d", offset, catalogPageSize)
		req, err := http.NewRequestWithContext(ctx, "GET", endpoint, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to build relation types request: %w", err)
		}
		body, err := executeRequest(client, req)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch relation types: %w", err)
		}
		var page relationTypePagedResponse
		if err := json.Unmarshal(body, &page); err != nil {
			return nil, fmt.Errorf("failed to parse relation types page: %w", err)
		}
		out = append(out, page.Results...)
		offset += len(page.Results)
		if len(page.Results) == 0 || int64(offset) >= page.Total {
			return out, nil
		}
	}
}

// ===== Attribute types =====

type AttributeTypeDetails struct {
	ID                         string   `json:"id"`
	Name                       string   `json:"name"`
	Description                string   `json:"description,omitempty"`
	PublicID                   string   `json:"publicId,omitempty"`
	AttributeTypeDiscriminator string   `json:"attributeTypeDiscriminator,omitempty"`
	AllowedValues              []string `json:"allowedValues,omitempty"`
	System                     bool     `json:"system"`
}

type attributeTypePagedResponse struct {
	Total   int64                  `json:"total"`
	Offset  int64                  `json:"offset"`
	Limit   int64                  `json:"limit"`
	Results []AttributeTypeDetails `json:"results"`
}

var attributeTypesCache catalogCache[[]AttributeTypeDetails]

// ListAllAttributeTypes returns every attribute type in DGC, cached.
func ListAllAttributeTypes(ctx context.Context, client *http.Client) ([]AttributeTypeDetails, error) {
	return attributeTypesCache.get(ctx, client, fetchAllAttributeTypes)
}

func fetchAllAttributeTypes(ctx context.Context, client *http.Client) ([]AttributeTypeDetails, error) {
	var out []AttributeTypeDetails
	offset := 0
	for {
		endpoint := fmt.Sprintf("/rest/2.0/attributeTypes?offset=%d&limit=%d", offset, catalogPageSize)
		req, err := http.NewRequestWithContext(ctx, "GET", endpoint, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to build attribute types request: %w", err)
		}
		body, err := executeRequest(client, req)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch attribute types: %w", err)
		}
		var page attributeTypePagedResponse
		if err := json.Unmarshal(body, &page); err != nil {
			return nil, fmt.Errorf("failed to parse attribute types page: %w", err)
		}
		out = append(out, page.Results...)
		offset += len(page.Results)
		if len(page.Results) == 0 || int64(offset) >= page.Total {
			return out, nil
		}
	}
}

// ===== Statuses =====

type StatusDetails struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	PublicID    string `json:"publicId,omitempty"`
	System      bool   `json:"system"`
}

type statusPagedResponse struct {
	Total   int64           `json:"total"`
	Offset  int64           `json:"offset"`
	Limit   int64           `json:"limit"`
	Results []StatusDetails `json:"results"`
}

var statusesCache catalogCache[[]StatusDetails]

// ListAllStatuses returns every status in DGC, cached.
func ListAllStatuses(ctx context.Context, client *http.Client) ([]StatusDetails, error) {
	return statusesCache.get(ctx, client, fetchAllStatuses)
}

func fetchAllStatuses(ctx context.Context, client *http.Client) ([]StatusDetails, error) {
	var out []StatusDetails
	offset := 0
	for {
		endpoint := fmt.Sprintf("/rest/2.0/statuses?offset=%d&limit=%d", offset, catalogPageSize)
		req, err := http.NewRequestWithContext(ctx, "GET", endpoint, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to build statuses request: %w", err)
		}
		body, err := executeRequest(client, req)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch statuses: %w", err)
		}
		var page statusPagedResponse
		if err := json.Unmarshal(body, &page); err != nil {
			return nil, fmt.Errorf("failed to parse statuses page: %w", err)
		}
		out = append(out, page.Results...)
		offset += len(page.Results)
		if len(page.Results) == 0 || int64(offset) >= page.Total {
			return out, nil
		}
	}
}

// ===== Asset types (cached, full-list) =====
//
// The existing ListAssetTypes (paginated, uncached) is preserved for
// backwards compatibility with the list_asset_types tool. find_asset_type
// uses ListAllAssetTypes instead so subsequent lookups hit the cache.

var assetTypesCache catalogCache[[]AssetTypeDetails]

// ListAllAssetTypes returns every asset type in DGC, cached.
func ListAllAssetTypes(ctx context.Context, client *http.Client) ([]AssetTypeDetails, error) {
	return assetTypesCache.get(ctx, client, fetchAllAssetTypes)
}

func fetchAllAssetTypes(ctx context.Context, client *http.Client) ([]AssetTypeDetails, error) {
	var out []AssetTypeDetails
	offset := 0
	for {
		page, err := ListAssetTypes(ctx, client, catalogPageSize, offset)
		if err != nil {
			return nil, err
		}
		out = append(out, page.Results...)
		offset += len(page.Results)
		if len(page.Results) == 0 || int64(offset) >= page.Total {
			return out, nil
		}
	}
}