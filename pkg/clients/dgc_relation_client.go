package clients

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
)

// Well-known Collibra UUIDs for relation and attribute types.
const (
	DefinitionAttributeTypeID                = "00000000-0000-0000-0000-000000000202"
	MeasureIsCalculatedUsingDataElementRelID = "00000000-0000-0000-0000-000000007200"
	BusinessAssetRepresentsDataAssetRelID    = "00000000-0000-0000-0000-000000007038"
	ColumnIsPartOfTableRelID                 = "00000000-0000-0000-0000-000000007042"
	DataAttributeRepresentsColumnRelID       = "00000000-0000-0000-0000-000000007094"
	ColumnIsSourceForDataAttributeRelID      = "00000000-0000-0000-0000-120000000011"
)

type RelationsQueryParams struct {
	SourceID       string `url:"sourceId,omitempty"`
	TargetID       string `url:"targetId,omitempty"`
	RelationTypeID string `url:"relationTypeId,omitempty"`
	Limit          int    `url:"limit"`
}

type RelationsResponse struct {
	Total   int        `json:"total"`
	Offset  int        `json:"offset"`
	Limit   int        `json:"limit"`
	Results []Relation `json:"results"`
}

type Relation struct {
	ID     string        `json:"id"`
	Source RelationAsset `json:"source"`
	Target RelationAsset `json:"target"`
}

type RelationAsset struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	TypeName string `json:"typeName"`
}

type AttributesQueryParams struct {
	AssetID         string `url:"assetId,omitempty"`
	AttributeTypeID string `url:"attributeTypeId,omitempty"`
}

type AttributesResponse struct {
	Total   int               `json:"total"`
	Offset  int               `json:"offset"`
	Limit   int               `json:"limit"`
	Results []AttributeResult `json:"results"`
}

type AttributeResult struct {
	ID    string `json:"id"`
	Value string `json:"value"`
}

type ConnectedAsset struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	AssetType string `json:"assetType"`
}

// GetRelations queries the Collibra relations API.
func GetRelations(ctx context.Context, client *http.Client, params RelationsQueryParams) (*RelationsResponse, error) {
	endpoint, err := buildUrl("/rest/2.0/relations", params)
	if err != nil {
		return nil, fmt.Errorf("failed to build relations endpoint: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "GET", endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create relations request: %w", err)
	}

	body, err := executeRequest(client, req)
	if err != nil {
		return nil, err
	}

	var response RelationsResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("failed to parse relations response: %w", err)
	}
	return &response, nil
}

// GetAssetAttributes queries the Collibra attributes API for a specific asset and attribute type.
func GetAssetAttributes(ctx context.Context, client *http.Client, assetID string, attrTypeID string) (*AttributesResponse, error) {
	params := AttributesQueryParams{
		AssetID:         assetID,
		AttributeTypeID: attrTypeID,
	}

	endpoint, err := buildUrl("/rest/2.0/attributes", params)
	if err != nil {
		return nil, fmt.Errorf("failed to build attributes endpoint: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "GET", endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create attributes request: %w", err)
	}

	body, err := executeRequest(client, req)
	if err != nil {
		return nil, err
	}

	var response AttributesResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("failed to parse attributes response: %w", err)
	}
	return &response, nil
}

// FindConnectedAssets finds assets connected to assetID via relationTypeID, querying both directions.
func FindConnectedAssets(ctx context.Context, client *http.Client, assetID string, relationTypeID string) ([]ConnectedAsset, error) {
	sourceResp, err := GetRelations(ctx, client, RelationsQueryParams{
		SourceID:       assetID,
		RelationTypeID: relationTypeID,
		Limit:          0,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get relations as source: %w", err)
	}

	targetResp, err := GetRelations(ctx, client, RelationsQueryParams{
		TargetID:       assetID,
		RelationTypeID: relationTypeID,
		Limit:          0,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get relations as target: %w", err)
	}

	allRelations := append(sourceResp.Results, targetResp.Results...)
	seen := make(map[string]struct{})
	result := make([]ConnectedAsset, 0)

	for _, rel := range allRelations {
		opposite := oppositeAsset(rel, assetID)
		if opposite.ID == "" {
			continue
		}
		if _, exists := seen[opposite.ID]; exists {
			continue
		}
		seen[opposite.ID] = struct{}{}
		result = append(result, opposite)
	}

	return result, nil
}

// FindColumnsForDataAttribute finds assets connected via both data attribute relation types.
func FindColumnsForDataAttribute(ctx context.Context, client *http.Client, dataAttributeID string) ([]ConnectedAsset, error) {
	seen := make(map[string]struct{})
	result := make([]ConnectedAsset, 0)

	for _, relID := range []string{DataAttributeRepresentsColumnRelID, ColumnIsSourceForDataAttributeRelID} {
		assets, err := FindConnectedAssets(ctx, client, dataAttributeID, relID)
		if err != nil {
			return nil, err
		}
		for _, asset := range assets {
			if _, exists := seen[asset.ID]; exists {
				continue
			}
			seen[asset.ID] = struct{}{}
			result = append(result, asset)
		}
	}

	return result, nil
}

// FetchDescription retrieves the definition/description attribute for an asset.
func FetchDescription(ctx context.Context, client *http.Client, assetID string) string {
	resp, err := GetAssetAttributes(ctx, client, assetID, DefinitionAttributeTypeID)
	if err != nil {
		slog.InfoContext(ctx, fmt.Sprintf("Failed to fetch description for asset %s: %v", assetID, err))
		return "No description available."
	}
	if len(resp.Results) > 0 && resp.Results[0].Value != "" {
		return resp.Results[0].Value
	}
	return "No description available."
}

func oppositeAsset(rel Relation, assetID string) ConnectedAsset {
	if rel.Source.ID == assetID {
		return ConnectedAsset{
			ID:        rel.Target.ID,
			Name:      rel.Target.Name,
			AssetType: rel.Target.TypeName,
		}
	}
	if rel.Target.ID == assetID {
		return ConnectedAsset{
			ID:        rel.Source.ID,
			Name:      rel.Source.Name,
			AssetType: rel.Source.TypeName,
		}
	}
	return ConnectedAsset{}
}
