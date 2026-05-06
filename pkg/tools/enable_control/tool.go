package enable_control

import (
	"context"
	"net/http"

	"github.com/collibra/chip/pkg/chip"
	"github.com/collibra/chip/pkg/clients"
)

type Input struct {
	ControlID string `json:"controlId" jsonschema:"Required. UUID of the control to enable (returned by create_control)."`
}

type Output struct {
	Enabled bool `json:"enabled" jsonschema:"True when the management API responded 2xx to the enable request."`
}

func NewTool(collibraClient *http.Client) *chip.Tool[Input, Output] {
	return &chip.Tool[Input, Output]{
		Name:        "enable_control",
		Description: "Enable a previously-created control via POST /rest/controlManagement/v1/controls/{controlId}/enable. Until enabled, the control will not run on its execution schedule.",
		Handler:     handler(collibraClient),
		Permissions: []string{},
	}
}

func handler(collibraClient *http.Client) chip.ToolHandlerFunc[Input, Output] {
	return func(ctx context.Context, input Input) (Output, error) {
		if _, err := clients.EnableControl(ctx, collibraClient, input.ControlID); err != nil {
			return Output{}, err
		}
		return Output{Enabled: true}, nil
	}
}
