// Package create_dq_rule implements the create_dq_rule MCP tool: it creates a
// data quality rule (a "monitor") on an existing DQ job (dataset) via the DQ
// monitoring API. The agent supplies the job name, a rule name, the rule type
// and its SQL; the tool validates the inputs and writes the rule.
package create_dq_rule

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/collibra/chip/pkg/chip"
	"github.com/collibra/chip/pkg/clients"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// OutputStatus is the overall outcome of a create_dq_rule call.
type OutputStatus string

const (
	// StatusSuccess means the rule was created.
	StatusSuccess OutputStatus = "success"
	// StatusValidationError means the inputs failed validation before any
	// write — empty required field or an unsupported monitorType.
	StatusValidationError OutputStatus = "validation_error"
	// StatusError means the rule could not be created due to a downstream
	// DQ service error.
	StatusError OutputStatus = "error"
)

// monitorType discriminators accepted by the DQ API.
const (
	monitorTypeFreeformSQL = "FREEFORM_SQL"
	monitorTypeSimpleSQL   = "SIMPLE_SQL"
)

// Input is the tool's typed input.
type Input struct {
	JobName      string   `json:"jobName" jsonschema:"Required. Name of the existing data quality job (dataset) the rule is attached to, e.g. 'PUBLIC.SAMPLE_DATASET'."`
	MonitorName  string   `json:"monitorName" jsonschema:"Required. Rule name. Only letters, digits, '-' and '_' are allowed; max 256 characters."`
	MonitorType  string   `json:"monitorType" jsonschema:"Required. Rule type: 'FREEFORM_SQL' for a full SQL expression, or 'SIMPLE_SQL' for a single-column check."`
	MonitorValue string   `json:"monitorValue" jsonschema:"Required. The rule's SQL. For FREEFORM_SQL this is a full query (e.g. 'SELECT * FROM @PUBLIC.SAMPLE_DATASET WHERE NAME IS NULL'); for SIMPLE_SQL it is the column predicate."`
	FilterQuery  string   `json:"filterQuery,omitempty" jsonschema:"Optional. Additional WHERE-clause filter applied to the rule (e.g. ' where NAME IS NULL')."`
	ColumnName   string   `json:"columnName,omitempty" jsonschema:"Optional. Target column name; used with SIMPLE_SQL rules."`
	Description  string   `json:"description,omitempty" jsonschema:"Optional. Human-readable description; max 256 characters."`
	Dimensions   []string `json:"dimensions,omitempty" jsonschema:"Optional. Data quality dimensions to associate with the rule (e.g. ['Accuracy','Completeness'])."`
	Tolerance    int      `json:"tolerance,omitempty" jsonschema:"Optional. Tolerance threshold — number of breaking records allowed before the rule is considered failing. Defaults to 0."`
	Active       *bool    `json:"active,omitempty" jsonschema:"Optional. Whether the rule is active. Defaults to true."`
	Suppressed   bool     `json:"suppressed,omitempty" jsonschema:"Optional. Whether the rule is suppressed (kept but not scored). Defaults to false."`
	TemplateID   string   `json:"templateId,omitempty" jsonschema:"Optional. UUID of a rule template to link this rule to so it appears under the template's 'Used In' tab."`
}

// Output is the typed response.
type Output struct {
	Status      OutputStatus `json:"status" jsonschema:"'success' when the rule was created; 'validation_error' for bad inputs; 'error' for downstream DQ failures."`
	Message     string       `json:"message" jsonschema:"Human-readable summary."`
	JobName     string       `json:"jobName,omitempty" jsonschema:"Job the rule was created on, on success."`
	MonitorName string       `json:"monitorName,omitempty" jsonschema:"Name of the created rule, on success."`
}

// NewTool returns the registered tool.
func NewTool(collibraClient *http.Client) *chip.Tool[Input, Output] {
	return &chip.Tool[Input, Output]{
		Name:  "create_dq_rule",
		Title: "Create Data Quality Rule",
		Description: "Create a data quality rule (monitor) on an existing data quality job (dataset). " +
			"monitorType is 'FREEFORM_SQL' (a full SQL query) or 'SIMPLE_SQL' (a single-column check). " +
			"The rule defaults to active and not suppressed. " +
			"Returns the job name and rule name on success. " +
			"Note: this uses the DQ monitoring API and requires permission to create rules on the target job.",
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

		resp, err := clients.CreateDQRule(ctx, collibraClient, clients.CreateDQRuleRequest{
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
		})
		if err != nil {
			return Output{Status: StatusError, Message: fmt.Sprintf("Could not create rule: %v", err)}, nil
		}

		return Output{
			Status:      StatusSuccess,
			Message:     fmt.Sprintf("Created rule %q on job %q.", resp.MonitorName, resp.JobName),
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
