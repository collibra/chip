package create_data_element

import (
	"context"
	"fmt"
	"net/http"

	"github.com/collibra/chip/pkg/chip"
	"github.com/collibra/chip/pkg/clients"
)

// Input defines the parameters for creating a Data Element asset.
type Input struct {
	Name        string `json:"name" jsonschema:"The full name of the new Data Element asset (must be unique within the domain)"`
	DomainID    string `json:"domain_id" jsonschema:"The UUID of the domain to create the Data Element in"`
	DisplayName string `json:"display_name,omitempty" jsonschema:"Optional. The display name of the new Data Element asset."`
	StatusID    string `json:"status_id,omitempty" jsonschema:"Optional. The UUID of the status to assign to the new Data Element."`
}

// Output defines the result of creating a Data Element asset.
type Output struct {
	ID           string `json:"id" jsonschema:"The UUID of the created Data Element asset"`
	ResourceType string `json:"resource_type" jsonschema:"The resource type of the created entity (Asset)"`
}

// NewTool creates a new create_data_element tool that creates Data Element assets in Collibra.
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

		reqBody := clients.CreateDataElementRequest{
			Name:        input.Name,
			DomainID:    input.DomainID,
			DisplayName: input.DisplayName,
			StatusID:    input.StatusID,
		}

		result, err := clients.CreateDataElement(ctx, collibraClient, reqBody)
		if err != nil {
			return Output{}, err
		}

		return Output{
			ID:           result.ID,
			ResourceType: result.ResourceType,
		}, nil
	}
}
