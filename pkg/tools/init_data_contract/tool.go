package init_data_contract

import (
	"context"
	"fmt"
	"net/http"

	"github.com/collibra/chip/pkg/chip"
	"github.com/collibra/chip/pkg/clients"
	"github.com/collibra/chip/pkg/tools/validation"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type Input struct {
	GovernedAssetID string `json:"governedAssetId" jsonschema:"The UUID of the Data Product Port asset to be governed by the data contract. This is the only required field. If an uninitialized contract already governs this port, that contract is initialized rather than duplicated."`
	Manifest        string `json:"manifest,omitempty" jsonschema:"Optional. The content of the data contract manifest file to upload. If omitted, the manifest is auto-generated from the governed port's existing Collibra metadata."`
	ManifestID      string `json:"manifestId,omitempty" jsonschema:"Optional. The unique identifier of the data contract as specified in the manifest. If omitted and a manifest that adheres to the Open Data Contract Standard is provided, the manifestID will be parsed automatically. If omitted and a manifest cannot be parsed, it defaults to the UUID of the data contract asset. Maximum length: 200 characters."`
	Version         string `json:"version,omitempty" jsonschema:"Optional. The version value for the initial data contract manifest. If omitted and a manifest that adheres to the Open Data Contract Standard is provided, the version is parsed automatically. If omitted and a manifest cannot be parsed, it defaults to '0.0.1'. Maximum length: 100 characters."`
	Name            string `json:"name,omitempty" jsonschema:"Optional. A custom, human-readable name for the data contract. If omitted and a manifest that adheres to the Open Data Contract Standard is provided, the name is parsed automatically. If omitted and a manifest cannot be parsed, it inherits the name of the governed asset. Maximum length: 200 characters."`
	DomainID        string `json:"domainId,omitempty" jsonschema:"Optional. The UUID of the domain where the data contract asset will be created. The specified domain must support the data contract asset type. If omitted, it defaults to the domain of the governed asset."`
}

type Output struct {
	ID            string `json:"id,omitempty" jsonschema:"The UUID of the data contract asset that was created or initialized"`
	Name          string `json:"name,omitempty" jsonschema:"The name of the data contract asset"`
	ManifestID    string `json:"manifestId,omitempty" jsonschema:"The unique identifier of the data contract manifest"`
	DomainName    string `json:"domainName,omitempty" jsonschema:"The name of the domain where the data contract asset is located"`
	DomainID      string `json:"domainId,omitempty" jsonschema:"The UUID of the domain where the data contract asset is located"`
	ActiveVersion string `json:"activeVersion,omitempty" jsonschema:"The version value of the currently active data contract manifest"`
	Format        string `json:"format,omitempty" jsonschema:"The format type of the active data contract manifest version. Possible values: ODCS, DCS, CUSTOM."`
	Error         string `json:"error,omitempty" jsonschema:"Error message if the data contract could not be initialized"`
	Success       bool   `json:"success" jsonschema:"Whether the data contract was successfully initialized"`
}

func NewTool(collibraClient *http.Client) *chip.Tool[Input, Output] {
	return &chip.Tool[Input, Output]{
		Name:        "init_data_contract",
		Title:       "Initialize Data Contract",
		Description: "Initialize a data contract and link it to its initial manifest. This is the first step in creating a data contract. Idempotent by governed port. Provide a manifest to upload, or omit it to auto-generate the manifest from the governed port's existing Collibra metadata. After initialization, use push_data_contract_manifest to add further manifest versions.",
		Handler:     handler(collibraClient),
		Permissions: []string{"dgc.data-contract"},
		Annotations: &mcp.ToolAnnotations{DestructiveHint: chip.Ptr(true)},
	}
}

func handler(collibraClient *http.Client) chip.ToolHandlerFunc[Input, Output] {
	return func(ctx context.Context, input Input) (Output, error) {
		if err := validation.UUID("governedAssetId", input.GovernedAssetID); err != nil {
			return Output{}, err
		}
		if err := validation.UUIDOptional("domainId", input.DomainID); err != nil {
			return Output{}, err
		}

		req := clients.InitDataContractRequest{
			GovernedAssetID: input.GovernedAssetID,
			Manifest:        input.Manifest,
			ManifestID:      input.ManifestID,
			Version:         input.Version,
			Name:            input.Name,
			DomainID:        input.DomainID,
		}

		response, err := clients.InitDataContract(ctx, collibraClient, req)
		if err != nil {
			return Output{
				Error:   fmt.Sprintf("Failed to initialize data contract: %s", err.Error()),
				Success: false,
			}, nil
		}

		return Output{
			ID:            response.ID,
			Name:          response.Name,
			ManifestID:    response.ManifestID,
			DomainName:    response.DomainName,
			DomainID:      response.DomainID,
			ActiveVersion: response.ActiveVersion,
			Format:        response.ManifestVersion.Format,
			Success:       true,
		}, nil
	}
}
