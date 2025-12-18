package tools

import (
	"context"
	"fmt"
	"net/http"

	"github.com/collibra/chip/pkg/chip"
	"github.com/collibra/chip/pkg/clients"
	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type PullDataContractManifestInput struct {
	DataContractID string `json:"dataContractId" jsonschema:"The UUID of the data contract asset (which is an asset type with ID 00000000-0000-0000-0000-000000050003) for which to download the active manifest version"`
}

type PullDataContractManifestOutput struct {
	Manifest string `json:"manifest,omitempty" jsonschema:"The content of the active data contract manifest file"`
	Error    string `json:"error,omitempty" jsonschema:"Error message if the manifest could not be retrieved"`
	Found    bool   `json:"found" jsonschema:"Whether the manifest was found"`
}

func NewPullDataContractManifestTool(collibraClient *http.Client) *chip.Tool[PullDataContractManifestInput, PullDataContractManifestOutput] {
	return &chip.Tool[PullDataContractManifestInput, PullDataContractManifestOutput]{
		Tool: &mcp.Tool{
			Name:        "data_contract_manifest_pull",
			Description: "Download the manifest file for the currently active version of a specific data contract. Returns the manifest content as a string.",
		},
		ToolHandler: handlePullDataContractManifest(collibraClient),
	}
}

func handlePullDataContractManifest(collibraClient *http.Client) chip.ToolHandlerFunc[PullDataContractManifestInput, PullDataContractManifestOutput] {
	return func(ctx context.Context, input PullDataContractManifestInput) (PullDataContractManifestOutput, error) {
		dataContractUUID, err := uuid.Parse(input.DataContractID)
		if err != nil {
			return PullDataContractManifestOutput{
				Error: fmt.Sprintf("Invalid data contract ID format: %s", err.Error()),
				Found: false,
			}, nil
		}

		manifest, err := clients.PullActiveDataContractManifest(ctx, collibraClient, dataContractUUID.String())
		if err != nil {
			return PullDataContractManifestOutput{
				Error: fmt.Sprintf("Failed to download manifest: %s", err.Error()),
				Found: false,
			}, nil
		}

		return PullDataContractManifestOutput{
			Manifest: string(manifest),
			Found:    true,
		}, nil
	}
}
