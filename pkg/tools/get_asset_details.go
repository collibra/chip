package tools

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/collibra/chip/pkg/chip"
	clients2 "github.com/collibra/chip/pkg/clients"
	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type AssetDetailsInput struct {
	AssetID                 string `json:"assetId" jsonschema:"the UUID of the asset to retrieve details for"`
	OutgoingRelationsCursor string `json:"outgoingRelationsCursor,omitempty" jsonschema:"Optional. Cursor (asset ID) to fetch the next page of outgoing relations. Use the last relation's target ID from the previous response."`
	IncomingRelationsCursor string `json:"incomingRelationsCursor,omitempty" jsonschema:"Optional. Cursor (asset ID) to fetch the next page of incoming relations. Use the last relation's source ID from the previous response."`
}

type AssetDetailsOutput struct {
	Asset *clients2.Asset `json:"asset,omitempty" jsonschema:"the detailed asset information if found"`
	Link  string          `json:"link,omitempty" jsonschema:"the link you can navigate to in Collibra to view the asset"`
	Error string          `json:"error,omitempty" jsonschema:"error message if asset not found or other error occurred"`
	Found bool            `json:"found" jsonschema:"whether the asset was found"`
}

func NewAssetDetailsTool() *chip.CollibraTool[AssetDetailsInput, AssetDetailsOutput] {
	return &chip.CollibraTool[AssetDetailsInput, AssetDetailsOutput]{
		Tool: &mcp.Tool{
			Name:        "getAssetDetails",
			Description: "Get detailed information about a specific asset by its UUID, including a link you can navigate to in Collibra, attributes, relations, and metadata. Returns up to 100 attributes per type (string, numeric, boolean, date). Supports cursor-based pagination for relations (50 per page). Use the last relation's target/source ID as cursor for the next page.",
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

	assets, err := clients2.GetAssetSummary(
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

	collibraUrl, err := chip.GetCollibraUrl(ctx)
	if err != nil {
		slog.Warn("url of Collibra instance unknown, links will be rendered only relative")
	}

	return AssetDetailsOutput{
		Asset: &assets[0],
		Found: true,
		Link:  fmt.Sprintf("%s/asset/%s", collibraUrl, assetUUID),
	}, nil
}
