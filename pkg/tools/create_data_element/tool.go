package create_data_element

import (
	"context"
	"fmt"
	"net/http"

	"github.com/collibra/chip/pkg/chip"
	"github.com/collibra/chip/pkg/clients"
)

// Input holds the parameters for the create_data_element tool.
type Input struct {
	Name        string `json:"name" jsonschema:"The name of the Data Element asset to create"`
	DomainID    string `json:"domain_id" jsonschema:"The UUID of the domain in which to create the Data Element"`
	DisplayName string `json:"display_name,omitempty" jsonschema:"Optional. The display name for the Data Element asset"`
	StatusID    string `json:"status_id,omitempty" jsonschema:"Optional. The status ID for the Data Element asset"`
}

// Output holds the result of the create_data_element tool.
type Output struct {
	ID           string `json:"id" jsonschema:"The UUID of the created Data Element asset"`
	ResourceType string `json:"resource_type" jsonschema:"The resource type of the created asset"`
}

// NewTool creates a new create_data_element tool.
func NewTool(collibraClient *http.Client) *chip.Tool[Input, Output] {
	return &chip.Tool[Input, Output]{
		Name:        "create_data_element",
		Description: "Create a new Data Element asset in a specified Collibra domain.",
		Handler:     handler(collibraClient),
		Permissions: []string{"dgc.ai-copilot"},
	}
}

func handler(collibraClient *http.Client) chip.ToolHandlerFunc[Input, Output] {
	return func(ctx context.Context, input Input) (Output, error) {
		if input.Name == "" {
			return Output{}, fmt.Errorf("name is required")
		}
		if input.DomainID == "" {
			return Output{}, fmt.Errorf("domain_id is required")
		}

		params := clients.CreateDataElementParams{
			Name:        input.Name,
			DomainID:    input.DomainID,
			DisplayName: input.DisplayName,
			StatusID:    input.StatusID,
		}

		result, err := clients.CreateDataElement(ctx, collibraClient, params)
		if err != nil {
			return Output{}, err
		}

		return Output{
			ID:           result.ID,
			ResourceType: result.ResourceType,
		}, nil
	}
}
