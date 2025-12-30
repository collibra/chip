package tools

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/collibra/chip/pkg/chip"
	"github.com/collibra/chip/pkg/clients"
)

type RemoveDataClassificationMatchInput struct {
	ClassificationMatchID string `json:"classificationMatchId" jsonschema:"Required. The UUID of the classification match to remove (e.g., '12345678-1234-1234-1234-123456789abc')"`
}

type RemoveDataClassificationMatchOutput struct {
	Success bool   `json:"success" jsonschema:"Whether the classification match was successfully removed"`
	Error   string `json:"error,omitempty" jsonschema:"Error message if the operation failed"`
}

func NewRemoveDataClassificationMatchTool(collibraClient *http.Client) *chip.Tool[RemoveDataClassificationMatchInput, RemoveDataClassificationMatchOutput] {
	return &chip.Tool[RemoveDataClassificationMatchInput, RemoveDataClassificationMatchOutput]{
		Name:        "data_classification_match_remove",
		Description: "Remove a classification match (association between a data class and an asset) from Collibra. Requires the UUID of the classification match to remove.",
		Handler:     handleRemoveDataClassificationMatch(collibraClient),
	}
}

func handleRemoveDataClassificationMatch(collibraClient *http.Client) chip.ToolHandlerFunc[RemoveDataClassificationMatchInput, RemoveDataClassificationMatchOutput] {
	return func(ctx context.Context, input RemoveDataClassificationMatchInput) (RemoveDataClassificationMatchOutput, error) {
		output, isNotValid := validateRemoveClassificationMatchInput(input)
		if isNotValid {
			return output, nil
		}

		err := clients.RemoveDataClassificationMatch(ctx, collibraClient, input.ClassificationMatchID)
		if err != nil {
			return RemoveDataClassificationMatchOutput{
				Success: false,
				Error:   fmt.Sprintf("Failed to remove classification match: %s", err.Error()),
			}, nil
		}

		return RemoveDataClassificationMatchOutput{
			Success: true,
		}, nil
	}
}

func validateRemoveClassificationMatchInput(input RemoveDataClassificationMatchInput) (RemoveDataClassificationMatchOutput, bool) {
	if strings.TrimSpace(input.ClassificationMatchID) == "" {
		return RemoveDataClassificationMatchOutput{
			Success: false,
			Error:   "Classification Match ID is required",
		}, true
	}

	return RemoveDataClassificationMatchOutput{}, false
}
