package get_data_access_control_details

import (
	"context"
	"fmt"
	"net/http"

	"github.com/collibra/chip/pkg/chip"
	"github.com/collibra/chip/pkg/clients"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type DataAccessControlInput struct {
	ID string `json:"id" jsonschema:"The id of the data access control to retrieve"`
}

type DataAccessControlOutput struct {
	AccessControl *clients.DataAccessControlDetails `json:"accessControl,omitempty" jsonschema:"The data access control details if found"`
	Error         string                            `json:"error,omitempty" jsonschema:"Error message if the access control could not be retrieved"`
	Found         bool                              `json:"found" jsonschema:"Whether the data access control was found"`
}

func NewTool(collibraClient *http.Client) *chip.Tool[DataAccessControlInput, DataAccessControlOutput] {
	return &chip.Tool[DataAccessControlInput, DataAccessControlOutput]{
		Name:        "get_data_access_control_details",
		Description: "Retrieve detailed information about a specific Collibra Data Access control by its id. Returns the access control's name, description, state (ACTIVE, INACTIVE, DELETED), action type (GRANT, MASK, FILTER, SHARE, GROUP, FILTERRULE), grant category, policy rule, external management status, ABAC scope parse status, and timestamps. Use this to inspect an individual access control when you know its ID.",
		Handler:     handleGetDataAccessControlDetails(collibraClient),
		Permissions: []string{},
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, DestructiveHint: new(false)},
	}
}

func handleGetDataAccessControlDetails(collibraClient *http.Client) chip.ToolHandlerFunc[DataAccessControlInput, DataAccessControlOutput] {
	return func(ctx context.Context, input DataAccessControlInput) (DataAccessControlOutput, error) {
		details, err := clients.GetDataAccessControl(ctx, collibraClient, input.ID)
		if err != nil {
			return DataAccessControlOutput{
				Error: fmt.Sprintf("Failed to retrieve data access control: %s", err.Error()),
				Found: false,
			}, nil
		}

		return DataAccessControlOutput{
			AccessControl: details,
			Found:         true,
		}, nil
	}
}
