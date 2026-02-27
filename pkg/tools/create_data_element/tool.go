package create_data_element

import (
	"context"
	"fmt"
	"net/http"

	"github.com/collibra/chip/pkg/chip"
	"github.com/collibra/chip/pkg/clients"
)

// DataElementTypeID is the Collibra asset type UUID for Data Element.
const DataElementTypeID = "00000000-0000-0000-0000-000000031008"

type Input struct {
	Name        string `json:"name" jsonschema:"Full name of the Data Element. Must be unique within the domain."`
	DomainID    string `json:"domain_id" jsonschema:"UUID of the domain where the Data Element will be created."`
	DisplayName string `json:"display_name,omitempty" jsonschema:"Optional. Display name for the Data Element."`
	StatusID    string `json:"status_id,omitempty" jsonschema:"Optional. UUID of the status to assign to the Data Element."`
}

type Output struct {
	ID           string `json:"id" jsonschema:"UUID of the created Data Element asset."`
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

		reqBody := clients.CreateDataElementRequest{
			Name:        input.Name,
			DomainID:    input.DomainID,
			TypeID:      DataElementTypeID,
			DisplayName: input.DisplayName,
			StatusID:    input.StatusID,
		}

		resp, err := clients.CreateDataElement(ctx, collibraClient, reqBody)
		if err != nil {
			return Output{}, err
		}

		return Output{
			ID:           resp.ID,
			ResourceType: resp.ResourceType,
		}, nil
	}
}
