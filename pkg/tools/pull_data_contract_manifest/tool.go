package pull_data_contract_manifest

import (
	"context"
	"fmt"
	"net/http"

	"github.com/collibra/chip/pkg/chip"
	"github.com/collibra/chip/pkg/clients"
	"github.com/google/uuid"
)

type Input struct {
	DataContractID string `json:"dataContractId" jsonschema:"The UUID of the data contract asset (which is an asset type with ID 00000000-0000-0000-0000-000000050003) for which to download the active manifest version"`
}

type Output struct {
	Manifest string `json:"manifest,omitempty" jsonschema:"The content of the active data contract manifest file"`
	Error    string `json:"error,omitempty" jsonschema:"Error message if the manifest could not be retrieved"`
	Found    bool   `json:"found" jsonschema:"Whether the manifest was found"`
}

func NewTool(collibraClient *http.Client) *chip.Tool[Input, Output] {
	return &chip.Tool[Input, Output]{
		Name:        "pull_data_contract_manifest",
		Description: "Download the manifest file for the currently active version of a specific data contract. Returns the manifest content as a string.",
		Handler:     handler(collibraClient),
		Permissions: []string{},
	}
}

func handler(collibraClient *http.Client) chip.ToolHandlerFunc[Input, Output] {
	return func(ctx context.Context, input Input) (Output, error) {
		dataContractUUID, err := uuid.Parse(input.DataContractID)
		if err != nil {
			return Output{
				Error: fmt.Sprintf("Invalid data contract ID format: %s", err.Error()),
				Found: false,
			}, nil
		}

		manifest, err := clients.PullActiveDataContractManifest(ctx, collibraClient, dataContractUUID.String())
		if err != nil {
			return Output{
				Error: fmt.Sprintf("Failed to download manifest: %s", err.Error()),
				Found: false,
			}, nil
		}

		return Output{
			Manifest: string(manifest),
			Found:    true,
		}, nil
	}
}
