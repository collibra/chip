package get_asset_details

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/collibra/chip/pkg/chip"
	"github.com/collibra/chip/pkg/clients"
	"github.com/google/uuid"
)

type Input struct {
	AssetID                 string `json:"assetId" jsonschema:"the UUID of the asset to retrieve details for"`
	OutgoingRelationsCursor string `json:"outgoingRelationsCursor,omitempty" jsonschema:"Optional. Cursor (asset ID) to fetch the next page of outgoing relations. Use the last relation's target ID from the previous response."`
	IncomingRelationsCursor string `json:"incomingRelationsCursor,omitempty" jsonschema:"Optional. Cursor (asset ID) to fetch the next page of incoming relations. Use the last relation's source ID from the previous response."`
}

type Output struct {
	Asset *clients.Asset `json:"asset,omitempty" jsonschema:"the detailed asset information if found"`
	Link  string         `json:"link,omitempty" jsonschema:"the link you can navigate to in Collibra to view the asset"`
	Error string         `json:"error,omitempty" jsonschema:"error message if asset not found or other error occurred"`
	Found bool           `json:"found" jsonschema:"whether the asset was found"`
}

func NewTool(collibraClient *http.Client) *chip.Tool[Input, Output] {
	return &chip.Tool[Input, Output]{
		Name:        "asset_details_get",
		Description: "Get detailed information about a specific asset by its UUID, including attributes, relations, and metadata. Returns up to 100 attributes per type and supports cursor-based pagination for relations (50 per page).",
		Handler:     handler(collibraClient),
		Permissions: []string{},
	}
}

func handler(collibraClient *http.Client) chip.ToolHandlerFunc[Input, Output] {
	return func(ctx context.Context, input Input) (Output, error) {
		assetUUID, err := uuid.Parse(input.AssetID)
		if err != nil {
			return Output{
				Error: fmt.Sprintf("Invalid asset ID format: %s", err.Error()),
				Found: false,
			}, nil
		}

		assets, err := clients.GetAssetSummary(
			ctx,
			collibraClient,
			assetUUID,
			input.OutgoingRelationsCursor,
			input.IncomingRelationsCursor,
		)
		if err != nil {
			return Output{
				Error: fmt.Sprintf("Failed to retrieve asset details: %s", err.Error()),
				Found: false,
			}, nil
		}

		if len(assets) == 0 {
			return Output{
				Error: "Asset not found",
				Found: false,
			}, nil
		}

		collibraHost, ok := chip.GetCollibraHost(ctx)
		if !ok {
			slog.WarnContext(ctx, "Collibra instance URL unknown, links will be rendered without host")
		}

		return Output{
			Asset: &assets[0],
			Found: true,
			Link:  fmt.Sprintf("%s/asset/%s", strings.TrimSuffix(collibraHost, "/"), assetUUID),
		}, nil
	}
}
