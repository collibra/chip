package remove_data_classification_match

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/collibra/chip/pkg/chip"
	"github.com/collibra/chip/pkg/clients"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type Input struct {
	ClassificationMatchID string `json:"classificationMatchId" jsonschema:"Required. The UUID of the classification match to remove (e.g., '12345678-1234-1234-1234-123456789abc')"`
}

type Output struct {
	Success bool   `json:"success" jsonschema:"Whether the classification match was successfully removed"`
	Error   string `json:"error,omitempty" jsonschema:"Error message if the operation failed"`
}

func NewTool(collibraClient *http.Client) *chip.Tool[Input, Output] {
	return &chip.Tool[Input, Output]{
		Name:        "remove_data_classification_match",
		Description: "Remove a classification match (association between a data class and an asset) from Collibra. Requires the UUID of the classification match to remove.",
		Handler:     handler(collibraClient),
		Permissions:  []string{"dgc.classify", "dgc.catalog", "dgc.data-classes-edit"},
		Annotations: &mcp.ToolAnnotations{DestructiveHint: chip.Ptr(true), IdempotentHint: true},
	}
}

func handler(collibraClient *http.Client) chip.ToolHandlerFunc[Input, Output] {
	return func(ctx context.Context, input Input) (Output, error) {
		output, isNotValid := validateInput(input)
		if isNotValid {
			return output, nil
		}

		err := clients.RemoveDataClassificationMatch(ctx, collibraClient, input.ClassificationMatchID)
		if err != nil {
			return Output{
				Success: false,
				Error:   fmt.Sprintf("Failed to remove classification match: %s", err.Error()),
			}, nil
		}

		return Output{
			Success: true,
		}, nil
	}
}

func validateInput(input Input) (Output, bool) {
	if strings.TrimSpace(input.ClassificationMatchID) == "" {
		return Output{
			Success: false,
			Error:   "Classification Match ID is required",
		}, true
	}

	return Output{}, false
}
