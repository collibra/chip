package tools

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/collibra/chip/pkg/chip"
	"github.com/collibra/chip/pkg/clients"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type AddClassificationMatchInput struct {
	AssetID          string `json:"assetId" jsonschema:"Required. The UUID of the asset to classify (e.g., '9179b887-04ef-4ce5-ab3a-b5bbd39ea3c8')"`
	ClassificationID string `json:"classificationId" jsonschema:"Required. The UUID of the data classification/data class to apply (e.g., 'be45c001-b173-48ff-ac91-3f6e45868c8b')"`
}

type AddClassificationMatchOutput struct {
	Match   *clients.ClassificationMatch `json:"match,omitempty" jsonschema:"The created classification match with all its properties"`
	Success bool                         `json:"success" jsonschema:"Whether the classification was successfully applied to the asset"`
	Error   string                       `json:"error,omitempty" jsonschema:"Error message if the operation failed"`
}

func NewAddClassificationMatchTool() *chip.CollibraTool[AddClassificationMatchInput, AddClassificationMatchOutput] {
	return &chip.CollibraTool[AddClassificationMatchInput, AddClassificationMatchOutput]{
		Tool: &mcp.Tool{
			Name: "addClassificationMatch",
			Description: "Associate a data classification (data class) with a specific data asset in Collibra. " +
				"Use this when the user wants to 'classify an asset', 'add a data class to an asset', " +
				"'apply a classification', or 'tag an asset with a data classification'. " +
				"You need both the asset UUID and the classification UUID to create the match.",
		},
		ToolHandler: handleAddClassificationMatch,
	}
}

func handleAddClassificationMatch(ctx context.Context, collibraHttpClient *http.Client, input AddClassificationMatchInput) (AddClassificationMatchOutput, error) {
	output, isNotValid := validateClassificationMatchInput(input)
	if isNotValid {
		return output, nil
	}

	request := clients.AddClassificationMatchRequest{
		AssetID:          input.AssetID,
		ClassificationID: input.ClassificationID,
	}

	match, err := clients.AddClassificationMatch(ctx, collibraHttpClient, request)
	if err != nil {
		return AddClassificationMatchOutput{
			Success: false,
			Error:   fmt.Sprintf("Failed to add classification match: %s", err.Error()),
		}, nil
	}

	return AddClassificationMatchOutput{
		Match:   match,
		Success: true,
	}, nil
}

func validateClassificationMatchInput(input AddClassificationMatchInput) (AddClassificationMatchOutput, bool) {
	if strings.TrimSpace(input.AssetID) == "" {
		return AddClassificationMatchOutput{
			Success: false,
			Error:   "Asset ID is required",
		}, true
	}

	if strings.TrimSpace(input.ClassificationID) == "" {
		return AddClassificationMatchOutput{
			Success: false,
			Error:   "Classification ID is required",
		}, true
	}

	return AddClassificationMatchOutput{}, false
}
