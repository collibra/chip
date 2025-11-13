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

type RemoveClassificationMatchInput struct {
	ClassificationMatchID string `json:"classificationMatchId" jsonschema:"Required. The UUID of the classification match to remove (e.g., '12345678-1234-1234-1234-123456789abc')"`
}

type RemoveClassificationMatchOutput struct {
	Success bool   `json:"success" jsonschema:"Whether the classification match was successfully removed"`
	Error   string `json:"error,omitempty" jsonschema:"Error message if the operation failed"`
}

func NewRemoveClassificationMatchTool() *chip.CollibraTool[RemoveClassificationMatchInput, RemoveClassificationMatchOutput] {
	return &chip.CollibraTool[RemoveClassificationMatchInput, RemoveClassificationMatchOutput]{
		Tool: &mcp.Tool{
			Name:        "classification_match_remove",
			Description: "Remove a classification match (association between a data class and an asset) from Collibra. Requires the UUID of the classification match to remove.",
		},
		ToolHandler: handleRemoveClassificationMatch,
	}
}

func handleRemoveClassificationMatch(ctx context.Context, collibraHttpClient *http.Client, input RemoveClassificationMatchInput) (RemoveClassificationMatchOutput, error) {
	output, isNotValid := validateRemoveClassificationMatchInput(input)
	if isNotValid {
		return output, nil
	}

	err := clients.RemoveClassificationMatch(ctx, collibraHttpClient, input.ClassificationMatchID)
	if err != nil {
		return RemoveClassificationMatchOutput{
			Success: false,
			Error:   fmt.Sprintf("Failed to remove classification match: %s", err.Error()),
		}, nil
	}

	return RemoveClassificationMatchOutput{
		Success: true,
	}, nil
}

func validateRemoveClassificationMatchInput(input RemoveClassificationMatchInput) (RemoveClassificationMatchOutput, bool) {
	if strings.TrimSpace(input.ClassificationMatchID) == "" {
		return RemoveClassificationMatchOutput{
			Success: false,
			Error:   "Classification Match ID is required",
		}, true
	}

	return RemoveClassificationMatchOutput{}, false
}
