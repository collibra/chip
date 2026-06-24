package clients

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
)

type generateContextRequest struct {
	AssetId string `json:"assetId"`
}

// GeneratedContextMetadata is the JSON envelope returned when includeMetadata=true.
type GeneratedContextMetadata struct {
	AssetId                  string               `json:"assetId"`
	ContextSpecificationId   string               `json:"contextSpecificationId"`
	ContextSpecificationName string               `json:"contextSpecificationName"`
	AssetType                ContextSpecAssetType `json:"assetType"`
	Content                  string               `json:"content"`
	GeneratedOn              string               `json:"generatedOn"`
}

// GenerateContextResult carries the output of a context generation call.
// When includeMetadata is false, only Content is populated (raw YAML).
// When includeMetadata is true, Content is inside Metadata.Content and the
// full Metadata struct is populated.
type GenerateContextResult struct {
	Content  string
	Metadata *GeneratedContextMetadata
}

// GenerateContext calls POST /rest/semanticBlueprint/v1/contextSpecifications/{id}/generate.
// When includeMetadata is false the raw YAML is returned in Result.Content.
// When includeMetadata is true the JSON envelope is returned in Result.Metadata.
func GenerateContext(
	ctx context.Context,
	collibraHttpClient *http.Client,
	assetId, contextSpecificationId string,
	includeMetadata bool,
) (*GenerateContextResult, error) {
	slog.InfoContext(ctx, fmt.Sprintf("Generating context for asset %s with spec %s, includeMetadata: %v", assetId, contextSpecificationId, includeMetadata))

	reqBody := generateContextRequest{AssetId: assetId}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	endpoint := fmt.Sprintf("/rest/semanticBlueprint/v1/contextSpecifications/%s/generate", contextSpecificationId)
	req, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	if includeMetadata {
		req.Header.Set("Accept", "application/json")
	} else {
		req.Header.Set("Accept", "application/yaml")
	}

	body, err := executeCollibraRequest(collibraHttpClient, req)
	if err != nil {
		return nil, err
	}

	if includeMetadata {
		var metadata GeneratedContextMetadata
		if err := json.Unmarshal(body, &metadata); err != nil {
			return nil, fmt.Errorf("failed to parse response: %w", err)
		}
		return &GenerateContextResult{
			Content:  metadata.Content,
			Metadata: &metadata,
		}, nil
	}

	return &GenerateContextResult{Content: string(body)}, nil
}
