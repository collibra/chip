package create_control

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/collibra/chip/pkg/chip"
	"github.com/collibra/chip/pkg/clients"
)

type Input struct {
	Name                 string         `json:"name" jsonschema:"Required. Display name of the new control."`
	Description          string         `json:"description" jsonschema:"Required. One-sentence description of what the control checks."`
	Category             string         `json:"category" jsonschema:"Required. ControlCategory value. Source candidate values from list_managed_control_attributes (.attributes.ControlCategory.allowedValues)."`
	ControlType          string         `json:"controlType" jsonschema:"Required. ControlType value. Source candidate values from list_managed_control_attributes (.attributes.ControlType.allowedValues)."`
	Severity             string         `json:"severity" jsonschema:"Required. Severity value. Source candidate values from list_managed_control_attributes (.attributes.Severity.allowedValues)."`
	DomainID             string         `json:"domainId" jsonschema:"Required. UUID of the domain that owns the control. Use resolve_domain to validate before calling."`
	Query                map[string]any `json:"query" jsonschema:"Required. The ControlQuery object — same shape accepted by dry_run_control_query."`
	ExecutionSchedule    map[string]any `json:"executionSchedule,omitempty" jsonschema:"Optional. Defaults to {\"frequency\":\"Daily\",\"timeOfDay\":\"00:00:00Z\",\"daysOfWeek\":[]} when omitted."`
	NotificationSettings map[string]any `json:"notificationSettings,omitempty" jsonschema:"Optional. Defaults to {} when omitted."`
}

type Output struct {
	Control json.RawMessage `json:"control" jsonschema:"Raw response from POST /rest/controlManagement/v1/controls. Includes the assigned controlId. The control is created in disabled state — call enable_control to activate it."`
}

var defaultExecutionSchedule = json.RawMessage(`{"frequency":"Daily","timeOfDay":"00:00:00Z","daysOfWeek":[]}`)
var defaultNotificationSettings = json.RawMessage(`{}`)

func NewTool(collibraClient *http.Client) *chip.Tool[Input, Output] {
	return &chip.Tool[Input, Output]{
		Name:        "create_control",
		Description: "Create a new Control Tower control by POSTing the full payload to the management API. The control is created disabled — use enable_control to turn it on. Returns the assigned controlId in the response body.",
		Handler:     handler(collibraClient),
		Permissions: []string{},
	}
}

func handler(collibraClient *http.Client) chip.ToolHandlerFunc[Input, Output] {
	return func(ctx context.Context, input Input) (Output, error) {
		queryBytes, err := json.Marshal(input.Query)
		if err != nil {
			return Output{}, err
		}
		schedule, err := marshalOrDefault(input.ExecutionSchedule, defaultExecutionSchedule)
		if err != nil {
			return Output{}, err
		}
		notifications, err := marshalOrDefault(input.NotificationSettings, defaultNotificationSettings)
		if err != nil {
			return Output{}, err
		}
		req := clients.CreateControlRequest{
			Name:                 input.Name,
			Description:          input.Description,
			Category:             input.Category,
			ControlType:          input.ControlType,
			Severity:             input.Severity,
			DomainID:             input.DomainID,
			Query:                queryBytes,
			ExecutionSchedule:    schedule,
			NotificationSettings: notifications,
		}
		raw, err := clients.CreateControl(ctx, collibraClient, req)
		if err != nil {
			return Output{}, err
		}
		return Output{Control: json.RawMessage(raw)}, nil
	}
}

func marshalOrDefault(m map[string]any, fallback json.RawMessage) (json.RawMessage, error) {
	if len(m) == 0 {
		return fallback, nil
	}
	return json.Marshal(m)
}
