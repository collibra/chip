package create_data_element

import (
	"context"
	"fmt"
	"net/http"

	"github.com/collibra/chip/pkg/chip"
	"github.com/collibra/chip/pkg/clients"
)

type Input struct {
	Name        string `json:"name" jsonschema:"Full name of the Data Element. Must be unique within the domain."`
	DomainID    string `json:"domain_id" jsonschema:"ID of the target domain (UUID format)."`
	TypeID      string `json:"type_id" jsonschema:"ID of the Data Element asset type (UUID format)."`
	DisplayName string `json:"display_name,omitempty" jsonschema:"Optional. Display name for the asset."`
	StatusID    string `json:"status_id,omitempty" jsonschema:"Optional. Status ID for the new asset (UUID format)."`
}

type Output struct {
	ID           string `json:"id" jsonschema:"ID of the created Data Element asset."`
	ResourceType string `json:"resource_type" jsonschema:"Resource type of the created asset."`
}

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

		resp, err := clients.CreateDataElement(ctx, collibraClient, clients.CreateDataElementRequest{
			Name:        input.Name,
			DomainID:    input.DomainID,
			TypeID:      input.TypeID,
			DisplayName: input.DisplayName,
			StatusID:    input.StatusID,
		})
		if err != nil {
			return Output{}, err
		}

		return Output{
			ID:           resp.ID,
			ResourceType: resp.ResourceType,
		}, nil
	}
}
