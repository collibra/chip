package create_data_element

import (
	"context"
	"fmt"
	"net/http"

	"github.com/collibra/chip/pkg/chip"
	"github.com/collibra/chip/pkg/clients"
)

// DataElementAssetTypeId is the Collibra asset type UUID for Data Element.
const DataElementAssetTypeId = "00000000-0000-0000-0000-000000031302"

// Input defines the parameters for creating a Data Element asset.
type Input struct {
	Name        string `json:"name" jsonschema:"Name of the Data Element asset to create. Must be unique within the domain."`
	DomainId    string `json:"domain_id" jsonschema:"UUID of the domain to create the Data Element in."`
	DisplayName string `json:"display_name,omitempty" jsonschema:"Optional. Display name for the Data Element asset."`
	StatusId    string `json:"status_id,omitempty" jsonschema:"Optional. UUID of the status to assign to the Data Element."`
}

// Output defines the result of creating a Data Element asset.
type Output struct {
	Id           string `json:"id" jsonschema:"UUID of the created Data Element asset."`
	ResourceType string `json:"resource_type" jsonschema:"Resource type of the created asset."`
}

// NewTool creates a new create_data_element tool instance.
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
		if input.DomainId == "" {
			return Output{}, fmt.Errorf("domain_id is required")
		}

		resp, err := clients.CreateDataElement(ctx, collibraClient, clients.CreateDataElementRequest{
			Name:        input.Name,
			DomainId:    input.DomainId,
			TypeId:      DataElementAssetTypeId,
			DisplayName: input.DisplayName,
			StatusId:    input.StatusId,
		})
		if err != nil {
			return Output{}, err
		}

		return Output{
			Id:           resp.Id,
			ResourceType: resp.ResourceType,
		}, nil
	}
}
