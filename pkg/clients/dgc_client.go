package clients

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"

	"github.com/google/go-querystring/query"
	"github.com/google/uuid"
)

func SearchKeyword(ctx context.Context, collibraHttpClient *http.Client, question string, resourceTypes []string, filters []SearchFilter, limit int, offset int) (*SearchResponse, error) {
	slog.InfoContext(ctx, fmt.Sprintf("Keyword search query: '%s'", question))
	searchUrl := "/rest/2.0/search"

	searchRequest := CreateSearchRequest(question, resourceTypes, filters, limit, offset)

	jsonData, err := json.Marshal(searchRequest)
	slog.InfoContext(ctx, fmt.Sprintf("Search request: %s", string(jsonData)))
	if err != nil {
		return nil, fmt.Errorf("failed to marshal search request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", searchUrl, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	body, err := executeRequest(collibraHttpClient, req)
	if err != nil {
		return nil, err
	}

	return ParseSearchResponse(body)
}

func GetAssetSummary(
	ctx context.Context,
	collibraHttpClient *http.Client,
	uuid uuid.UUID,
	outgoingRelationsCursor string,
	incomingRelationsCursor string,
) ([]Asset, error) {
	gqlUrl := "/graphql/knowledgeGraph/v1"
	gqlRequest := CreateAssetDetailsGraphQLQuery(
		[]string{uuid.String()},
		outgoingRelationsCursor,
		incomingRelationsCursor,
	)

	jsonData, err := json.Marshal(gqlRequest)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal GraphQL request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", gqlUrl, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	body, err := executeRequest(collibraHttpClient, req)
	if err != nil {
		return nil, err
	}

	return ParseAssetDetailsGraphQLResponse(body)
}

func ListAssetTypes(ctx context.Context, collibraHttpClient *http.Client, limit int, offset int) (*AssetTypePagedResponse, error) {
	slog.InfoContext(ctx, fmt.Sprintf("Listing asset types with limit: %d, offset: %d", limit, offset))

	params := AssetTypesQueryParams{
		ExcludeMeta: true,
		Limit:       limit,
		Offset:      offset,
	}

	endpoint, err := buildUrl("/rest/2.0/assetTypes", params)
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

	return ParseAssetTypesResponse(body)
}

func ListDataContracts(ctx context.Context, collibraHttpClient *http.Client, cursor string, limit int, manifestID string) (*DataContractListPaginated, error) {
	slog.InfoContext(ctx, fmt.Sprintf("Listing data contracts with limit: %d, cursor: %s", limit, cursor))

	params := DataContractsQueryParams{
		Cursor:       cursor,
		Limit:        limit,
		IncludeTotal: true,
		ManifestID:   manifestID,
	}

	endpoint, err := buildUrl("/rest/dataProduct/v1/dataContracts", params)
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

	return ParseDataContractsResponse(body)
}

func PullActiveDataContractManifest(ctx context.Context, collibraHttpClient *http.Client, dataContractID string) ([]byte, error) {
	slog.InfoContext(ctx, fmt.Sprintf("Pulling active manifest for data contract ID: %s", dataContractID))

	endpoint := fmt.Sprintf("/rest/dataProduct/v1/dataContracts/%s/activeVersion/manifest", dataContractID)

	req, err := http.NewRequestWithContext(ctx, "GET", endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	manifest, err := executeRequest(collibraHttpClient, req)
	if err != nil {
		return nil, err
	}

	return manifest, nil
}

func PushDataContractManifest(ctx context.Context, collibraHttpClient *http.Client, reqParams PushDataContractManifestRequest) (*PushDataContractManifestResponse, error) {
	slog.InfoContext(ctx, fmt.Sprintf("Pushing data contract manifest for manifest ID: %s", reqParams.ManifestID))

	endpoint := "/rest/dataProduct/v1/dataContracts/addFromManifest"

	body, contentType, err := CreateAddFromManifestRequest(reqParams)
	if err != nil {
		return nil, err
	}

	slog.InfoContext(ctx, fmt.Sprintf("content type: %s", contentType))

	req, err := http.NewRequestWithContext(ctx, "POST", endpoint, body)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", contentType)

	responseBody, err := executeRequest(collibraHttpClient, req)
	if err != nil {
		return nil, err
	}

	return ParseAddFromManifestResponse(responseBody)
}

func buildUrl(basePath string, params interface{}) (string, error) {
	values, err := query.Values(params)
	if err != nil {
		return "", fmt.Errorf("failed to encode query params: %w", err)
	}
	if encoded := values.Encode(); encoded != "" {
		return basePath + "?" + encoded, nil
	}
	return basePath, nil
}

func executeRequest(client *http.Client, req *http.Request) ([]byte, error) {
	response, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to make request: %w", err)
	}
	defer func(Body io.ReadCloser) {
		_ = Body.Close()
	}(response.Body)

	responseBody, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("HTTP %d: %s", response.StatusCode, string(responseBody))
	}

	return responseBody, nil
}
