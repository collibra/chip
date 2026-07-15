// Package edit_dq_rule implements the edit_dq_rule MCP tool: it updates an
// existing data quality rule (monitor) on a DQ job, optionally renaming it.
package edit_dq_rule

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/collibra/chip/pkg/chip"
	"github.com/collibra/chip/pkg/clients"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// OutputStatus is the overall outcome of an edit_dq_rule call.
type OutputStatus string

const (
	// StatusSuccess means the rule was updated.
	StatusSuccess OutputStatus = "success"
	// StatusValidationError means the inputs failed validation before any write.
	StatusValidationError OutputStatus = "validation_error"
	// StatusError means the rule could not be updated due to a downstream error.
	StatusError OutputStatus = "error"
)

// monitorType discriminators accepted by the DQ API.
const (
	monitorTypeFreeformSQL = "FREEFORM_SQL"
	monitorTypeSimpleSQL   = "SIMPLE_SQL"
)

// Input is the tool's typed input. The rule is identified by (jobName,
// monitorName); the remaining fields are the new definition (send the full
// definition — this is a replace, not a patch).
type Input struct {
	JobName        string   `json:"jobName" jsonschema:"Required. Name of the data quality job the rule is attached to (a job, also called a 'dataset', is a saved check on one database table)."`
	MonitorName    string   `json:"monitorName" jsonschema:"Required. Name of the existing rule (the check; Collibra calls it a 'monitor') to edit."`
	MonitorType    string   `json:"monitorType" jsonschema:"Required. Rule type: 'FREEFORM_SQL' (a full SQL query) or 'SIMPLE_SQL' (a single-column check)."`
	MonitorValue   string   `json:"monitorValue" jsonschema:"Required. The rule's SQL. For FREEFORM_SQL a full query; for SIMPLE_SQL the column predicate."`
	NewMonitorName string   `json:"newMonitorName,omitempty" jsonschema:"Optional. New name for the rule (rename). Only letters, digits, '-' and '_'; max 256 characters."`
	FilterQuery    string   `json:"filterQuery,omitempty" jsonschema:"Optional. Additional WHERE-clause filter applied to the rule."`
	ColumnName     string   `json:"columnName,omitempty" jsonschema:"Optional. Target column name; used with SIMPLE_SQL rules."`
	Description    string   `json:"description,omitempty" jsonschema:"Optional. Human-readable description; max 256 characters."`
	Dimensions     []string `json:"dimensions,omitempty" jsonschema:"Optional. Data quality dimensions — categories such as Accuracy, Completeness, Validity — to associate with the rule."`
	Tolerance      int      `json:"tolerance,omitempty" jsonschema:"Optional. Number of failing ('breaking') records allowed before the rule fails — a count, NOT a percentage. Defaults to 0."`
	Active         *bool    `json:"active,omitempty" jsonschema:"Optional. Whether the rule is active. Defaults to true."`
	Suppressed     bool     `json:"suppressed,omitempty" jsonschema:"Optional. Whether the rule is suppressed (kept but not scored). Defaults to false."`
	TemplateID     string   `json:"templateId,omitempty" jsonschema:"Optional. UUID of a rule template to link this rule to."`
}

// Output is the typed response.
type Output struct {
	Status      OutputStatus `json:"status" jsonschema:"'success' when the rule was updated; 'validation_error' for bad inputs; 'error' for downstream DQ failures."`
	Message     string       `json:"message" jsonschema:"Human-readable summary."`
	JobName     string       `json:"jobName,omitempty" jsonschema:"Job the rule belongs to, on success."`
	MonitorName string       `json:"monitorName,omitempty" jsonschema:"Name of the updated rule (the new name if renamed), on success."`
}

// NewTool returns the registered tool.
func NewTool(collibraClient *http.Client) *chip.Tool[Input, Output] {
	return &chip.Tool[Input, Output]{
		Name:  "edit_dq_rule",
		Title: "Edit Data Quality Rule",
		Description: "Update an existing data quality rule (a check on a table's data; Collibra calls it a 'monitor') on a data quality job (a saved check on ONE database table; also called a 'dataset'). " +
			"Send the full rule definition (this replaces the rule, it is not a partial patch). " +
			"Set newMonitorName to rename the rule. " +
			"Note: requires permission to edit rules on the target job.",
		Handler:     handler(collibraClient),
		Permissions: []string{},
		Annotations: &mcp.ToolAnnotations{DestructiveHint: chip.Ptr(false)},
	}
}

func handler(collibraClient *http.Client) chip.ToolHandlerFunc[Input, Output] {
	return func(ctx context.Context, input Input) (Output, error) {
		if out := validate(input); out != nil {
			return *out, nil
		}

		resp, err := clients.EditDQRule(ctx, collibraClient, clients.EditDQRuleRequest{
			JobName:      strings.TrimSpace(input.JobName),
			MonitorName:  strings.TrimSpace(input.MonitorName),
			MonitorType:  input.MonitorType,
			MonitorValue: input.MonitorValue,
			FilterQuery:  input.FilterQuery,
			ColumnName:   input.ColumnName,
			Description:  input.Description,
			Dimensions:   input.Dimensions,
			Tolerance:    input.Tolerance,
			IsActive:     activeFlag(input.Active),
			IsSuppressed: input.Suppressed,
			TemplateID:   strings.TrimSpace(input.TemplateID),
		}, strings.TrimSpace(input.NewMonitorName))
		if err != nil {
			return Output{Status: StatusError, Message: fmt.Sprintf("Could not edit rule: %v", err)}, nil
		}

		return Output{
			Status:      StatusSuccess,
			Message:     fmt.Sprintf("Updated rule %q on job %q.", resp.MonitorName, resp.JobName),
			JobName:     resp.JobName,
			MonitorName: resp.MonitorName,
		}, nil
	}
}

// validate enforces the required fields and the monitorType enum before any
// network call, so the agent gets a cheap, self-correcting error.
func validate(input Input) *Output {
	if strings.TrimSpace(input.JobName) == "" {
		return &Output{Status: StatusValidationError, Message: "jobName is required."}
	}
	if strings.TrimSpace(input.MonitorName) == "" {
		return &Output{Status: StatusValidationError, Message: "monitorName is required."}
	}
	if strings.TrimSpace(input.MonitorValue) == "" {
		return &Output{Status: StatusValidationError, Message: "monitorValue is required."}
	}
	switch input.MonitorType {
	case monitorTypeFreeformSQL, monitorTypeSimpleSQL:
	default:
		return &Output{
			Status:  StatusValidationError,
			Message: fmt.Sprintf("monitorType %q is invalid. Use %q or %q.", input.MonitorType, monitorTypeFreeformSQL, monitorTypeSimpleSQL),
		}
	}
	return nil
}

// activeFlag maps the optional `active` input to the DQ API's isActive int.
// A nil pointer (field omitted) defaults to active.
func activeFlag(active *bool) int {
	if active == nil || *active {
		return 1
	}
	return 0
}
