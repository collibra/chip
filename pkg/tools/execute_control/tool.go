package execute_control

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/collibra/chip/pkg/chip"
	"github.com/collibra/chip/pkg/clients"
)

type Input struct {
	ControlID string `json:"controlId" jsonschema:"Required. UUID of the control to execute once."`
}

type Output struct {
	Result json.RawMessage `json:"result" jsonschema:"Raw response from POST /rest/controlExecution/v1/controls/{controlId}/execute. Used as a post-create smoke check — compare the result count against the last dry-run."`
}

func NewTool(collibraClient *http.Client) *chip.Tool[Input, Output] {
	return &chip.Tool[Input, Output]{
		Name:        "execute_control",
		Description: "Execute a control once via the execution service (off-schedule, ad-hoc). The control does not need to be enabled. Used as a post-create smoke check to compare the live result count against the last dry-run.",
		Handler:     handler(collibraClient),
		Permissions: []string{},
	}
}

func handler(collibraClient *http.Client) chip.ToolHandlerFunc[Input, Output] {
	return func(ctx context.Context, input Input) (Output, error) {
		body, err := clients.ExecuteControl(ctx, collibraClient, input.ControlID)
		if err != nil {
			return Output{}, err
		}
		return Output{Result: json.RawMessage(body)}, nil
	}
}
