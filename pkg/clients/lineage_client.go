package clients

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

type LineageEntity struct {
	Id        string   `json:"id"`
	Name      string   `json:"name"`
	Type      string   `json:"type"`
	SourceIds []string `json:"sourceIds,omitempty"`
	DgcId     string   `json:"dgcId,omitempty"`
	ParentId  string   `json:"parentId,omitempty"`
}

// UnmarshalJSON handles both plain string values and JsonNullable-wrapped objects
// for the DgcId and ParentId fields. The server may serialize JsonNullable<T> as
// {"present": false, "undefined": true} when JsonNullableModule is not on the classpath.
func (e *LineageEntity) UnmarshalJSON(data []byte) error {
	type lineageEntityAlias LineageEntity
	var raw struct {
		lineageEntityAlias
		DgcId    json.RawMessage `json:"dgcId"`
		ParentId json.RawMessage `json:"parentId"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	*e = LineageEntity(raw.lineageEntityAlias)
	e.DgcId = extractJsonNullableString(raw.DgcId)
	e.ParentId = extractJsonNullableString(raw.ParentId)
	return nil
}

// extractJsonNullableString extracts a string from either a plain JSON string
// or a JsonNullable object. Returns empty string for null, undefined, or objects
// where the value is not recoverable.
func extractJsonNullableString(data json.RawMessage) string {
	if len(data) == 0 || string(data) == "null" {
		return ""
	}
	var s string
	if err := json.Unmarshal(data, &s); err == nil {
		return s
	}
	// JsonNullable object format — actual value is not serialized without the module
	return ""
}

type LineageRelation struct {
	SourceEntityId    string   `json:"sourceEntityId"`
	TargetEntityId    string   `json:"targetEntityId"`
	TransformationIds []string `json:"transformationIds"`
}

type LineagePagination struct {
	NextCursor string `json:"nextCursor,omitempty"`
}

type LineageResponseWarning struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type LineageTransformation struct {
	Id                  string `json:"id"`
	Name                string `json:"name"`
	Description         string `json:"description,omitempty"`
	TransformationLogic string `json:"transformationLogic,omitempty"`
}

type TransformationSummary struct {
	Id          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

// --- API response types ---

type lineageUpstreamDownstreamResponse struct {
	Relations  []LineageRelation        `json:"relations"`
	NextCursor string                   `json:"nextCursor,omitempty"`
	Warnings   []LineageResponseWarning `json:"warnings,omitempty"`
}

type lineageEntitiesResponse struct {
	Results    []LineageEntity          `json:"results"`
	NextCursor string                   `json:"nextCursor,omitempty"`
	Warnings   []LineageResponseWarning `json:"warnings,omitempty"`
}

type lineageTransformationsResponse struct {
	Results    []TransformationSummary  `json:"results"`
	NextCursor string                   `json:"nextCursor,omitempty"`
	Warnings   []LineageResponseWarning `json:"warnings,omitempty"`
}

// --- Output types ---

type GetLineageEntityOutput struct {
	Entity *LineageEntity `json:"entity,omitempty"`
	Error  string         `json:"error,omitempty"`
	Found  bool           `json:"found"`
}

type GetLineageDirectionalOutput struct {
	EntityId   string                   `json:"entityId"`
	Direction  LineageDirection         `json:"direction"`
	Relations  []LineageRelation        `json:"relations"`
	Pagination *LineagePagination       `json:"pagination,omitempty"`
	Warnings   []LineageResponseWarning `json:"warnings,omitempty"`
	Error      string                   `json:"error,omitempty"`
}

type SearchLineageEntitiesOutput struct {
	Results    []LineageEntity          `json:"results"`
	Pagination *LineagePagination       `json:"pagination,omitempty"`
	Warnings   []LineageResponseWarning `json:"warnings,omitempty"`
}

type GetLineageTransformationOutput struct {
	Transformation *LineageTransformation `json:"transformation,omitempty"`
	Error          string                 `json:"error,omitempty"`
	Found          bool                   `json:"found"`
}

type SearchLineageTransformationsOutput struct {
	Results    []TransformationSummary  `json:"results"`
	Pagination *LineagePagination       `json:"pagination,omitempty"`
	Warnings   []LineageResponseWarning `json:"warnings,omitempty"`
}

type LineageDirection string

const (
	LineageDirectionUpstream   LineageDirection = "upstream"
	LineageDirectionDownstream LineageDirection = "downstream"

	// lineageDGCProxyPath is the path prefix targeting the lineage proxy on DGC
	lineageDGCProxyPath = "/technical_lineage_resource"

	// lineageReadAPIPath is the API prefix for the lineage read API (LineageRead.yaml).
	lineageReadAPIPath = "/rest/lineageGraphRead/v1"

	lineageAPIBasePath = lineageDGCProxyPath + lineageReadAPIPath
)

// --- Query param structs ---

type lineageDirectionalParams struct {
	EntityType string `url:"entityType,omitempty"`
	Limit      int    `url:"limit,omitempty"`
	Cursor     string `url:"cursor,omitempty"`
}

type lineageSearchEntitiesParams struct {
	NameContains string `url:"nameContains,omitempty"`
	Type         string `url:"type,omitempty"`
	DgcId        string `url:"dgcId,omitempty"`
	Limit        int    `url:"limit,omitempty"`
	Cursor       string `url:"cursor,omitempty"`
}

type lineageSearchTransformationsParams struct {
	NameContains string `url:"nameContains,omitempty"`
	Limit        int    `url:"limit,omitempty"`
	Cursor       string `url:"cursor,omitempty"`
}

// --- Client functions ---

func GetLineageEntity(ctx context.Context, collibraHttpClient *http.Client, entityId string) (*GetLineageEntityOutput, error) {
	endpoint := fmt.Sprintf("%s/entities/%s", lineageAPIBasePath, entityId)

	req, err := http.NewRequestWithContext(ctx, "GET", endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	body, err := executeRequest(collibraHttpClient, req)
	if err != nil {
		return &GetLineageEntityOutput{Found: false, Error: err.Error()}, nil
	}

	var entity LineageEntity
	if err := json.Unmarshal(body, &entity); err != nil {
		return nil, fmt.Errorf("failed to parse entity response: %w", err)
	}

	return &GetLineageEntityOutput{Entity: &entity, Found: true}, nil
}

func GetLineageUpstream(ctx context.Context, collibraHttpClient *http.Client, entityId string, entityType string, limit int, cursor string) (*GetLineageDirectionalOutput, error) {
	return getLineageDirectional(ctx, collibraHttpClient, entityId, LineageDirectionUpstream, entityType, limit, cursor)
}

func GetLineageDownstream(ctx context.Context, collibraHttpClient *http.Client, entityId string, entityType string, limit int, cursor string) (*GetLineageDirectionalOutput, error) {
	return getLineageDirectional(ctx, collibraHttpClient, entityId, LineageDirectionDownstream, entityType, limit, cursor)
}

func getLineageDirectional(ctx context.Context, collibraHttpClient *http.Client, entityId string, direction LineageDirection, entityType string, limit int, cursor string) (*GetLineageDirectionalOutput, error) {
	basePath := fmt.Sprintf("%s/entities/%s/%s", lineageAPIBasePath, entityId, direction)

	params := lineageDirectionalParams{
		EntityType: entityType,
		Limit:      limit,
		Cursor:     cursor,
	}

	endpoint, err := buildUrl(basePath, params)
	if err != nil {
		return nil, fmt.Errorf("failed to build endpoint: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "GET", endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	body, err := executeRequest(collibraHttpClient, req)
	if err != nil {
		return &GetLineageDirectionalOutput{EntityId: entityId, Direction: direction, Relations: []LineageRelation{}, Error: err.Error()}, nil
	}

	var resp lineageUpstreamDownstreamResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse %s response: %w", direction, err)
	}

	out := &GetLineageDirectionalOutput{
		EntityId:  entityId,
		Direction: direction,
		Relations: resp.Relations,
		Warnings:  resp.Warnings,
	}
	if resp.NextCursor != "" {
		out.Pagination = &LineagePagination{NextCursor: resp.NextCursor}
	}
	return out, nil
}

func SearchLineageEntities(ctx context.Context, collibraHttpClient *http.Client, nameContains string, entityType string, dgcId string, limit int, cursor string) (*SearchLineageEntitiesOutput, error) {
	params := lineageSearchEntitiesParams{
		NameContains: nameContains,
		Type:         entityType,
		DgcId:        dgcId,
		Limit:        limit,
		Cursor:       cursor,
	}

	endpoint, err := buildUrl(lineageAPIBasePath+"/entities", params)
	if err != nil {
		return nil, fmt.Errorf("failed to build endpoint: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "GET", endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	body, err := executeRequest(collibraHttpClient, req)
	if err != nil {
		return nil, err
	}

	var resp lineageEntitiesResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse entities response: %w", err)
	}

	out := &SearchLineageEntitiesOutput{
		Results:  resp.Results,
		Warnings: resp.Warnings,
	}
	if resp.NextCursor != "" {
		out.Pagination = &LineagePagination{NextCursor: resp.NextCursor}
	}
	return out, nil
}

func GetLineageTransformation(ctx context.Context, collibraHttpClient *http.Client, transformationId string) (*GetLineageTransformationOutput, error) {
	endpoint := fmt.Sprintf("%s/transformations/%s", lineageAPIBasePath, transformationId)

	req, err := http.NewRequestWithContext(ctx, "GET", endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	body, err := executeRequest(collibraHttpClient, req)
	if err != nil {
		return &GetLineageTransformationOutput{Found: false, Error: err.Error()}, nil
	}

	var t LineageTransformation
	if err := json.Unmarshal(body, &t); err != nil {
		return nil, fmt.Errorf("failed to parse transformation response: %w", err)
	}

	return &GetLineageTransformationOutput{Transformation: &t, Found: true}, nil
}

func SearchLineageTransformations(ctx context.Context, collibraHttpClient *http.Client, nameContains string, limit int, cursor string) (*SearchLineageTransformationsOutput, error) {
	params := lineageSearchTransformationsParams{
		NameContains: nameContains,
		Limit:        limit,
		Cursor:       cursor,
	}

	endpoint, err := buildUrl(lineageAPIBasePath+"/transformations", params)
	if err != nil {
		return nil, fmt.Errorf("failed to build endpoint: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "GET", endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	body, err := executeRequest(collibraHttpClient, req)
	if err != nil {
		return nil, err
	}

	var resp lineageTransformationsResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse transformations response: %w", err)
	}

	out := &SearchLineageTransformationsOutput{
		Results:  resp.Results,
		Warnings: resp.Warnings,
	}
	if resp.NextCursor != "" {
		out.Pagination = &LineagePagination{NextCursor: resp.NextCursor}
	}
	return out, nil
}
