package get_control

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/collibra/chip/pkg/chip"
	"github.com/collibra/chip/pkg/clients"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type Input struct {
	ControlID string `json:"controlId" jsonschema:"Required. UUID of the control to read."`
}

type Output struct {
	Control map[string]any `json:"control" jsonschema:"Raw response from GET /rest/controlManagement/v1/controls/{controlId}. Includes the full ControlQuery JSON, executionSchedule, notificationSettings, severity/controlType/category, domainId, enabled flag, and the assigned controlId."`
}

func NewTool(collibraClient *http.Client) *chip.Tool[Input, Output] {
	return &chip.Tool[Input, Output]{
		Name:        "get_control",
		Description: "Read a single Control Tower control by id. Returns the full ManagedControl payload including the ControlQuery JSON — use this when you need to inspect the query a saved control was created with (the DGC asset only carries metadata).",
		Handler:     handler(collibraClient),
		Permissions: []string{},
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}
}

func handler(collibraClient *http.Client) chip.ToolHandlerFunc[Input, Output] {
	return func(ctx context.Context, input Input) (Output, error) {
		if input.ControlID == "" {
			return Output{}, fmt.Errorf("controlId is required")
		}
		raw, err := clients.GetControl(ctx, collibraClient, input.ControlID)
		if err != nil {
			return Output{}, err
		}
		var control map[string]any
		if err := json.Unmarshal(raw, &control); err != nil {
			return Output{}, err
		}
		return Output{Control: control}, nil
	}
}
