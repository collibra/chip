// Package start_workflow implements the start_workflow MCP tool. The tool is
// two-phase by design: if the caller hasn't supplied values for all required
// form properties, the tool returns the form schema and a list of missing keys
// so the LLM can prompt the user. Once every required property is provided, it
// POSTs to /workflowInstances and returns the started workflow instance.
package start_workflow

import (
	"context"
	"fmt"
	"net/http"

	"github.com/collibra/chip/pkg/chip"
	"github.com/collibra/chip/pkg/clients"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	StatusFormRequired = "form_required"
	StatusStarted      = "started"
	StatusDryRun       = "dry_run"

	formSourceStart         = "startFormData"
	formSourceConfiguration = "configurationStartFormData"
)

type Input struct {
	WorkflowDefinitionID string            `json:"workflowDefinitionId" jsonschema:"REQUIRED. UUID of the workflow definition to start. Use find_workflow_definitions to look it up by name."`
	FormProperties       map[string]string `json:"formProperties,omitempty" jsonschema:"Map of form-property id -> stringified value. Omit (or leave keys missing) on the first call to discover which properties the workflow requires; the tool will return the form schema. Call again with the user-provided values filled in to actually start the workflow."`
	BusinessItemIDs      []string          `json:"businessItemIds,omitempty" jsonschema:"Optional. UUIDs of the business items (assets/domains/communities) the workflow operates on. Omit for GLOBAL workflows."`
	BusinessItemType     string            `json:"businessItemType,omitempty" jsonschema:"Optional. One of ASSET, DOMAIN, COMMUNITY, USER, GLOBAL. Should match the workflow definition's businessItemDiscriminator."`
	SendNotification     bool              `json:"sendNotification,omitempty" jsonschema:"Optional. Whether the platform should send a notification when the workflow starts."`
	DryRun               bool              `json:"dryRun,omitempty" jsonschema:"Optional. When true, fetches and returns the start-form schema (from both startFormData and, if that is empty, configurationStartFormData) and never POSTs. Use this when start_workflow keeps failing with 'workflowNotStarted' to see what (if anything) the platform exposes as the form schema for this workflow definition."`
}

type Output struct {
	Status          string             `json:"status" jsonschema:"One of 'form_required' (caller must collect form values and call again), 'started' (workflow began), or 'dry_run' (schema returned, nothing posted)."`
	Message         string             `json:"message,omitempty" jsonschema:"Human-readable explanation of the current status, suitable for relaying to the user."`
	FormSource      string             `json:"formSource,omitempty" jsonschema:"Which platform endpoint the form schema came from: 'startFormData' (BPMN task-style form) or 'configurationStartFormData' (Marketplace / Global-Create-style form). Helpful when debugging an empty schema."`
	MissingRequired []string           `json:"missingRequired,omitempty" jsonschema:"Ids of required form properties that were not supplied. Present when status=form_required."`
	FormProperties  []FormProperty     `json:"formProperties,omitempty" jsonschema:"The form-property schema. Use this to prompt the user for each field. Present when status=form_required or status=dry_run. May be empty if the platform did not expose a schema for this workflow at either endpoint."`
	Instances       []WorkflowInstance `json:"instances,omitempty" jsonschema:"The started workflow instances. Present when status=started."`
}

type FormProperty struct {
	ID         string         `json:"id" jsonschema:"The id to use as the key in formProperties when calling the tool again."`
	Name       string         `json:"name,omitempty" jsonschema:"The display name of the property, suitable for asking the user."`
	Type       string         `json:"type,omitempty" jsonschema:"The form-property type, e.g. 'string', 'enum', 'user', 'date', 'boolean'."`
	Required   bool           `json:"required" jsonschema:"Whether the user must provide a value for this property."`
	MultiValue bool           `json:"multiValue,omitempty" jsonschema:"Whether the property accepts multiple values."`
	HelpText   string         `json:"helpText,omitempty" jsonschema:"Optional guidance from the workflow author. Surface this to the user when asking for input."`
	Default    string         `json:"default,omitempty" jsonschema:"The default value, if any."`
	Options    []OptionValue  `json:"options,omitempty" jsonschema:"Allowed values for enum/dropdown-style properties. Pick one of these as the formProperties value."`
}

type OptionValue struct {
	Value string `json:"value" jsonschema:"The value to send as the formProperties entry."`
	Label string `json:"label,omitempty" jsonschema:"Human-readable label to show the user."`
}

type WorkflowInstance struct {
	ID             string `json:"id" jsonschema:"The UUID of the started workflow instance."`
	Ended          bool   `json:"ended,omitempty" jsonschema:"Whether the instance already finished (some workflows complete synchronously)."`
	InError        bool   `json:"inError,omitempty" jsonschema:"Whether the instance entered an error state."`
	ErrorMessage   string `json:"errorMessage,omitempty" jsonschema:"Error message, when inError=true."`
	StartDate      int64  `json:"startDate,omitempty" jsonschema:"Unix-millisecond start timestamp."`
	CreatedAssetID string `json:"createdAssetId,omitempty" jsonschema:"For workflows that create an asset, the new asset's UUID."`
}

func NewTool(collibraClient *http.Client) *chip.Tool[Input, Output] {
	return &chip.Tool[Input, Output]{
		Name: "start_workflow",
		Description: "Start a Collibra workflow instance. First call (without formProperties, or with required keys missing) returns the workflow's start-form schema and a list of missing required properties — use it to ask the user for each value. Call again with the user-supplied values in formProperties to actually trigger the workflow. If a call fails with 'workflowNotStarted', retry with dryRun=true to inspect what (if any) schema the platform exposes; the tool checks both startFormData and configurationStartFormData (some Marketplace / Global-Create OOTB workflows only expose the latter).",
		Handler:     handler(collibraClient),
		Permissions: []string{},
		Annotations: &mcp.ToolAnnotations{DestructiveHint: chip.Ptr(true)},
	}
}

func handler(collibraClient *http.Client) chip.ToolHandlerFunc[Input, Output] {
	return func(ctx context.Context, input Input) (Output, error) {
		if input.WorkflowDefinitionID == "" {
			return Output{}, fmt.Errorf("workflowDefinitionId is required")
		}

		schema, source, err := fetchFormSchema(ctx, collibraClient, input.WorkflowDefinitionID)
		if err != nil {
			return Output{}, fmt.Errorf("failed to fetch start form data: %w", err)
		}

		if input.DryRun {
			return Output{
				Status:         StatusDryRun,
				Message:        dryRunMessage(source, len(schema)),
				FormSource:     source,
				FormProperties: mapFormProperties(schema),
			}, nil
		}

		missing := missingRequired(schema, input.FormProperties)
		if len(missing) > 0 {
			return Output{
				Status:          StatusFormRequired,
				Message:         fmt.Sprintf("This workflow needs %d more value(s) before it can start. Ask the user for each missing property, then call start_workflow again with formProperties populated.", len(missing)),
				FormSource:      source,
				MissingRequired: missing,
				FormProperties:  mapFormProperties(schema),
			}, nil
		}

		instances, err := clients.StartWorkflowInstances(ctx, collibraClient, clients.StartWorkflowInstancesRequest{
			WorkflowDefinitionID: input.WorkflowDefinitionID,
			BusinessItemIDs:      input.BusinessItemIDs,
			BusinessItemType:     input.BusinessItemType,
			FormProperties:       input.FormProperties,
			SendNotification:     input.SendNotification,
		})
		if err != nil {
			return Output{}, fmt.Errorf("failed to start workflow: %w", err)
		}

		return Output{
			Status:     StatusStarted,
			Message:    fmt.Sprintf("Started %d workflow instance(s).", len(instances)),
			FormSource: source,
			Instances:  mapInstances(instances),
		}, nil
	}
}

// fetchFormSchema tries the task-style /startFormData first, then falls back to
// /configurationStartFormData when the first returns no properties. Some
// Marketplace and Global-Create OOTB workflows (e.g. "Create Data Product
// (Simple)") expose their fields only through the configuration endpoint.
func fetchFormSchema(ctx context.Context, c *http.Client, workflowDefinitionID string) ([]clients.FormProperty, string, error) {
	formData, err := clients.GetWorkflowStartFormData(ctx, c, workflowDefinitionID)
	if err != nil {
		return nil, "", err
	}
	if len(formData.FormProperties) > 0 {
		return formData.FormProperties, formSourceStart, nil
	}

	configForm, err := clients.GetWorkflowConfigurationStartFormData(ctx, c, workflowDefinitionID)
	if err != nil {
		// Fallback failure is non-fatal: report the empty task form so the
		// caller still sees something. Most workflows really do have no form.
		return formData.FormProperties, formSourceStart, nil
	}
	if len(configForm.FormProperties) > 0 {
		return configForm.FormProperties, formSourceConfiguration, nil
	}
	return formData.FormProperties, formSourceStart, nil
}

func dryRunMessage(source string, count int) string {
	if count == 0 {
		return "Neither startFormData nor configurationStartFormData returned any form properties for this workflow. The workflow may not use a BPMN start form, or may require fields exposed by a different mechanism."
	}
	return fmt.Sprintf("Got %d form properties from %s. Pass them in formProperties to start the workflow.", count, source)
}

func missingRequired(schema []clients.FormProperty, provided map[string]string) []string {
	var missing []string
	for _, p := range schema {
		if !p.Required {
			continue
		}
		if v, ok := provided[p.ID]; !ok || v == "" {
			missing = append(missing, p.ID)
		}
	}
	return missing
}

func mapFormProperties(in []clients.FormProperty) []FormProperty {
	out := make([]FormProperty, len(in))
	for i, p := range in {
		out[i] = FormProperty{
			ID:         p.ID,
			Name:       p.Name,
			Type:       p.Type,
			Required:   p.Required,
			MultiValue: p.MultiValue,
			HelpText:   p.HelpText,
			Default:    p.Value,
			Options:    mergeOptions(p),
		}
	}
	return out
}

// mergeOptions collapses the three dropdown lists the API can return into one
// list the LLM can show the user. Order of preference matches the platform's
// own resolution rules.
func mergeOptions(p clients.FormProperty) []OptionValue {
	src := p.EnumValues
	if len(src) == 0 {
		src = p.ProposedDropdownValues
	}
	if len(src) == 0 {
		src = p.DefaultDropdownValues
	}
	if len(src) == 0 {
		return nil
	}
	out := make([]OptionValue, len(src))
	for i, v := range src {
		value := v.IDAsString
		if value == "" {
			value = v.ID
		}
		out[i] = OptionValue{Value: value, Label: v.Text}
	}
	return out
}

func mapInstances(in []clients.WorkflowInstance) []WorkflowInstance {
	out := make([]WorkflowInstance, len(in))
	for i, w := range in {
		out[i] = WorkflowInstance{
			ID:             w.ID,
			Ended:          w.Ended,
			InError:        w.InError,
			ErrorMessage:   w.ErrorMessage,
			StartDate:      w.StartDate,
			CreatedAssetID: w.CreatedAssetID,
		}
	}
	return out
}
