package clients

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
)

// ContextSpecAssetType is the asset type associated with a Context Specification.
type ContextSpecAssetType struct {
	PublicId string `json:"publicId"`
	Name     string `json:"name,omitempty"`
}

// ContextSpecification is the full Context Specification resource as returned
// by the Semantic Blueprint API.
type ContextSpecification struct {
	ID             string               `json:"id"`
	Name           string               `json:"name"`
	Description    string               `json:"description,omitempty"`
	AssetType      ContextSpecAssetType `json:"assetType"`
	MappingYaml    string               `json:"mappingYaml"`
	CreatedBy      string               `json:"createdBy"`
	CreatedOn      string               `json:"createdOn"`
	LastModifiedBy string               `json:"lastModifiedBy"`
	LastModifiedOn string               `json:"lastModifiedOn"`
}

// ContextSpecificationPagedResponse is the paged list returned by
// GET /rest/semanticBlueprint/v1/contextSpecifications.
type ContextSpecificationPagedResponse struct {
	Total   int                    `json:"total"`
	Results []ContextSpecification `json:"results"`
}

type contextSpecQueryParams struct {
	AssetId           string `url:"assetId,omitempty"`
	AssetTypePublicId string `url:"assetTypePublicId,omitempty"`
	Offset            int    `url:"offset,omitempty"`
	Limit             int    `url:"limit,omitempty"`
}

// ListContextSpecifications calls GET /rest/semanticBlueprint/v1/contextSpecifications.
func ListContextSpecifications(
	ctx context.Context,
	collibraHttpClient *http.Client,
	assetId, assetTypePublicId string,
	offset, limit int,
) (*ContextSpecificationPagedResponse, error) {
	slog.InfoContext(ctx, fmt.Sprintf("Listing context specifications, limit: %d, offset: %d", limit, offset))

	params := contextSpecQueryParams{
		AssetId:           assetId,
		AssetTypePublicId: assetTypePublicId,
		Offset:            offset,
		Limit:             limit,
	}

	endpoint, err := buildUrl("/rest/semanticBlueprint/v1/contextSpecifications", params)
	if err != nil {
		return nil, fmt.Errorf("failed to build endpoint: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "GET", endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	body, err := executeCollibraRequest(collibraHttpClient, req)
	if err != nil {
		return nil, err
	}

	var response ContextSpecificationPagedResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}
	return &response, nil
}

// GetContextSpecification calls GET /rest/semanticBlueprint/v1/contextSpecifications/{id}.
func GetContextSpecification(
	ctx context.Context,
	collibraHttpClient *http.Client,
	contextSpecificationId string,
) (*ContextSpecification, error) {
	slog.InfoContext(ctx, fmt.Sprintf("Getting context specification: %s", contextSpecificationId))

	endpoint := fmt.Sprintf("/rest/semanticBlueprint/v1/contextSpecifications/%s", contextSpecificationId)
	req, err := http.NewRequestWithContext(ctx, "GET", endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	body, err := executeCollibraRequest(collibraHttpClient, req)
	if err != nil {
		return nil, err
	}

	var spec ContextSpecification
	if err := json.Unmarshal(body, &spec); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}
	return &spec, nil
}
