package tools

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/collibra/chip/pkg/chip"
	"github.com/collibra/chip/pkg/clients"
	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type AssetDetailsInput struct {
	AssetID                 string `json:"assetId" jsonschema:"the UUID of the asset to retrieve details for"`
	OutgoingRelationsCursor string `json:"outgoingRelationsCursor,omitempty" jsonschema:"Optional. Cursor (asset ID) to fetch the next page of outgoing relations. Use the last relation's target ID from the previous response."`
	IncomingRelationsCursor string `json:"incomingRelationsCursor,omitempty" jsonschema:"Optional. Cursor (asset ID) to fetch the next page of incoming relations. Use the last relation's source ID from the previous response."`
}

type AssetDetailsOutput struct {
	Asset *clients.Asset `json:"asset,omitempty" jsonschema:"the detailed asset information if found"`
	Link  string         `json:"link,omitempty" jsonschema:"the link you can navigate to in Collibra to view the asset"`
	Error string         `json:"error,omitempty" jsonschema:"error message if asset not found or other error occurred"`
	Found bool           `json:"found" jsonschema:"whether the asset was found"`
}

func NewAssetDetailsTool() *chip.Tool[AssetDetailsInput, AssetDetailsOutput] {
	return &chip.Tool[AssetDetailsInput, AssetDetailsOutput]{
		Tool: &mcp.Tool{
			Name:        "asset_details_get",
			Description: "Get detailed information about a specific asset by its UUID, including attributes, relations, and metadata. Returns up to 100 attributes per type and supports cursor-based pagination for relations (50 per page).",
		},
		ToolHandler: handleAssetDetails,
	}
}

func handleAssetDetails(ctx context.Context, collibraHttpClient *http.Client, input AssetDetailsInput) (AssetDetailsOutput, error) {
	assetUUID, err := uuid.Parse(input.AssetID)
	if err != nil {
		return AssetDetailsOutput{
			Error: fmt.Sprintf("Invalid asset ID format: %s", err.Error()),
			Found: false,
		}, nil
	}

	assets, err := clients.GetAssetSummary(
		ctx,
		collibraHttpClient,
		assetUUID,
		input.OutgoingRelationsCursor,
		input.IncomingRelationsCursor,
	)
	if err != nil {
		return AssetDetailsOutput{
			Error: fmt.Sprintf("Failed to retrieve asset details: %s", err.Error()),
			Found: false,
		}, nil
	}

	if len(assets) == 0 {
		return AssetDetailsOutput{
			Error: "Asset not found",
			Found: false,
		}, nil
	}

	collibraUrl, err := getCollibraUrl(ctx)
	if err != nil {
		slog.WarnContext(ctx, "Collibra instance URL unknown, links will be rendered without host")
	}

	return AssetDetailsOutput{
		Asset: &assets[0],
		Found: true,
		Link:  fmt.Sprintf("%s/asset/%s", collibraUrl, assetUUID),
	}, nil
}

func getCollibraUrl(ctx context.Context) (string, error) {
	toolRequest, err := chip.GetCallToolRequest(ctx)
	if err != nil {
		return "", err
	}
	if toolRequest.GetExtra() != nil {
		if url := toolRequest.Extra.Header.Get("collibraUrl"); url != "" {
			return strings.TrimSuffix(url, "/"), nil
		}
	}
	config, err := chip.GetToolConfig(ctx)
	if err != nil {
		return "", err
	}
	return strings.TrimSuffix(config.CollibraUrl, "/"), nil
}
