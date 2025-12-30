package tools

import (
	"context"
	"fmt"
	"net/http"

	"github.com/collibra/chip/pkg/chip"
	"github.com/collibra/chip/pkg/clients"
)

type PushDataContractManifestInput struct {
	ManifestID string `json:"manifestId,omitempty" jsonschema:"The unique identifier of the data contract as specified in the manifest. If omitted and a manifest that adheres to the Open Data Contract Standard is provided, the manifestID will be parsed automatically. Maximum length: 200 characters."`
	Manifest   string `json:"manifest" jsonschema:"The content of the data contract manifest file"`
	Version    string `json:"version,omitempty" jsonschema:"Optional. The version of the data contract manifest being uploaded. If omitted, the version will be parsed automatically from the manifest unless it does not adhere to the Open Data Contract Standard. Maximum length: 100 characters."`
	Force      bool   `json:"force,omitempty" jsonschema:"Optional. Set to true to force the overwrite of an existing manifest version if it has the same version value. When a new manifest overwrites the active version, the 'active' parameter in the request is ignored, and the version's active state remains unchanged. Defaults to false."`
	Active     bool   `json:"active,omitempty" jsonschema:"Optional. Set to true to make this data contract manifest version the active version. This will automatically deactivate the previous active version. The active version is the one that's exposed through the data contract asset. Defaults to true."`
}

type PushDataContractManifestOutput struct {
	ID         string `json:"id,omitempty" jsonschema:"The UUID of the data contract asset"`
	DomainID   string `json:"domainId,omitempty" jsonschema:"The UUID of the domain where the data contract asset is located"`
	ManifestID string `json:"manifestId,omitempty" jsonschema:"The unique identifier of the data contract manifest"`
	Error      string `json:"error,omitempty" jsonschema:"Error message if the manifest could not be uploaded"`
	Success    bool   `json:"success" jsonschema:"Whether the manifest was successfully uploaded"`
}

func NewPushDataContractManifestTool(collibraClient *http.Client) *chip.Tool[PushDataContractManifestInput, PushDataContractManifestOutput] {
	return &chip.Tool[PushDataContractManifestInput, PushDataContractManifestOutput]{
		Name:        "data_contract_manifest_push",
		Description: "Upload a new version of a data contract manifest to Collibra. The manifestID and version are automatically parsed from the manifest content if it adheres to the Open Data Contract Standard.",
		Handler:     handlePushDataContractManifest(collibraClient),
	}
}

func handlePushDataContractManifest(collibraClient *http.Client) chip.ToolHandlerFunc[PushDataContractManifestInput, PushDataContractManifestOutput] {
	return func(ctx context.Context, input PushDataContractManifestInput) (PushDataContractManifestOutput, error) {
		if input.Manifest == "" {
			return PushDataContractManifestOutput{
				Error:   "Manifest content is required",
				Success: false,
			}, nil
		}

		req := clients.PushDataContractManifestRequest{
			Manifest:   input.Manifest,
			ManifestID: input.ManifestID,
			Version:    input.Version,
			Force:      input.Force,
			Active:     input.Active,
		}

		response, err := clients.PushDataContractManifest(ctx, collibraClient, req)
		if err != nil {
			return PushDataContractManifestOutput{
				Error:   fmt.Sprintf("Failed to upload manifest: %s", err.Error()),
				Success: false,
			}, nil
		}

		return PushDataContractManifestOutput{
			ID:         response.ID,
			DomainID:   response.DomainID,
			ManifestID: response.ManifestID,
			Success:    true,
		}, nil
	}
}
