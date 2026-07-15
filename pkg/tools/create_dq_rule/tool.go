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
	// StatusPreview means confirm was not set: the tool returned the composed
	// rule (including its SQL) for review and created nothing.
	StatusPreview OutputStatus = "preview"
)

// monitorType discriminators accepted by the DQ API.
const (
	monitorTypeFreeformSQL = "FREEFORM_SQL"
	monitorTypeSimpleSQL   = "SIMPLE_SQL"
)

// Input is the tool's typed input.
type Input struct {
	JobName      string   `json:"jobName" jsonschema:"Required. Name of the existing data quality job the rule is attached to (a job, also called a 'dataset', is a saved data-quality check on one database table), e.g. 'PUBLIC.SAMPLE_DATASET'."`
	MonitorName  string   `json:"monitorName" jsonschema:"Required. Rule name. Only letters, digits, '-' and '_' are allowed; max 256 characters."`
	MonitorType  string   `json:"monitorType" jsonschema:"Required. Rule type: 'FREEFORM_SQL' for a full SQL expression, or 'SIMPLE_SQL' for a single-column check."`
	MonitorValue string   `json:"monitorValue" jsonschema:"Required. The rule's SQL. For FREEFORM_SQL this is a full query (e.g. 'SELECT * FROM @PUBLIC.SAMPLE_DATASET WHERE NAME IS NULL'); for SIMPLE_SQL it is the column predicate."`
	FilterQuery  string   `json:"filterQuery,omitempty" jsonschema:"Optional. Additional WHERE-clause filter applied to the rule (e.g. ' where NAME IS NULL')."`
	ColumnName   string   `json:"columnName,omitempty" jsonschema:"Optional. Target column name; used with SIMPLE_SQL rules."`
	Description  string   `json:"description,omitempty" jsonschema:"Optional. Human-readable description; max 256 characters."`
	Dimensions   []string `json:"dimensions,omitempty" jsonschema:"Optional. Data quality dimensions — categories such as Accuracy, Completeness, Validity — to associate with the rule (e.g. ['Accuracy','Completeness'])."`
	Tolerance    int      `json:"tolerance,omitempty" jsonschema:"Optional. Number of failing ('breaking') records allowed before the rule is considered failed — a count, NOT a percentage. Defaults to 0."`
	Active       *bool    `json:"active,omitempty" jsonschema:"Optional. Whether the rule is active. Defaults to true."`
	Suppressed   bool     `json:"suppressed,omitempty" jsonschema:"Optional. Whether the rule is suppressed (kept but not scored). Defaults to false."`
	TemplateID   string   `json:"templateId,omitempty" jsonschema:"Optional. UUID of a rule template to link this rule to so it appears under the template's 'Used In' tab."`
	Confirm      bool     `json:"confirm,omitempty" jsonschema:"Safety checkpoint. false (default) returns a PREVIEW of the rule — including its SQL — WITHOUT creating it, so it can be reviewed with the user. Set true to actually create the rule after the user has approved."`
}

// RulePreview is the composed rule echoed back for review when confirm is false.
type RulePreview struct {
	JobName      string   `json:"jobName"`
	MonitorName  string   `json:"monitorName"`
	MonitorType  string   `json:"monitorType"`
	MonitorValue string   `json:"monitorValue" jsonschema:"The rule's SQL that will be saved — review this with the user before confirming."`
	FilterQuery  string   `json:"filterQuery,omitempty"`
	ColumnName   string   `json:"columnName,omitempty"`
	Dimensions   []string `json:"dimensions,omitempty"`
	Tolerance    int      `json:"tolerance"`
	Active       bool     `json:"active"`
	Suppressed   bool     `json:"suppressed"`
}

// Output is the typed response.
type Output struct {
	Status      OutputStatus `json:"status" jsonschema:"'preview' when confirm was not set (nothing created — review the preview and call again with confirm=true); 'success' when the rule was created; 'validation_error' for bad inputs; 'error' for downstream DQ failures."`
	Message     string       `json:"message" jsonschema:"Human-readable summary."`
	Preview     *RulePreview `json:"preview,omitempty" jsonschema:"The composed rule (with its SQL) returned when confirm=false; nothing was created."`
	JobName     string       `json:"jobName,omitempty" jsonschema:"Job the rule was created on, on success."`
	MonitorName string       `json:"monitorName,omitempty" jsonschema:"Name of the created rule, on success."`
}

// NewTool returns the registered tool.
func NewTool(collibraClient *http.Client) *chip.Tool[Input, Output] {
	return &chip.Tool[Input, Output]{
		Name:  "create_dq_rule",
		Title: "Create Data Quality Rule",
		Description: "Create a data quality rule (a single data-quality check on a table's data; Collibra calls it a 'monitor') " +
			"on an existing data quality job (a saved data-quality check on ONE database table that scans the table and runs its rules; also called a 'dataset'), identified by its job name. " +
			"monitorType is 'FREEFORM_SQL' (a full SQL query) or 'SIMPLE_SQL' (a single-column check). " +
			"The rule defaults to active and not suppressed (suppressed = kept but not scored). " +
			"Built around a confirm checkpoint: confirm=false (default) returns a PREVIEW of the rule and its SQL without creating anything — review it with the user; confirm=true creates the rule. " +
			"Returns the job name and rule name on success. " +
			"Note: requires permission to create rules on the target job.",
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

		request := clients.CreateDQRuleRequest{
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
		}

		// Confirm checkpoint: without confirm, return the composed rule (SQL
		// included) for review and create nothing.
		if !input.Confirm {
			return Output{
				Status: StatusPreview,
				Message: fmt.Sprintf("Preview only — nothing created. Will create rule %q on job %q with SQL: %s. "+
					"Review this with the user, then call again with confirm=true.", request.MonitorName, request.JobName, request.MonitorValue),
				Preview: &RulePreview{
					JobName:      request.JobName,
					MonitorName:  request.MonitorName,
					MonitorType:  request.MonitorType,
					MonitorValue: request.MonitorValue,
					FilterQuery:  request.FilterQuery,
					ColumnName:   request.ColumnName,
					Dimensions:   request.Dimensions,
					Tolerance:    request.Tolerance,
					Active:       request.IsActive == 1,
					Suppressed:   request.IsSuppressed,
				},
			}, nil
		}

		resp, err := clients.CreateDQRule(ctx, collibraClient, request)
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
