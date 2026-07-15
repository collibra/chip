// Package get_dq_rule implements the get_dq_rule MCP tool: it reads the
// definition of a single data quality rule (monitor) on an existing DQ job.
package get_dq_rule

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/collibra/chip/pkg/chip"
	"github.com/collibra/chip/pkg/clients"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// OutputStatus is the overall outcome of a get_dq_rule call.
type OutputStatus string

const (
	// StatusSuccess means the rule was found and returned.
	StatusSuccess OutputStatus = "success"
	// StatusValidationError means the inputs failed validation before any read.
	StatusValidationError OutputStatus = "validation_error"
	// StatusError means the rule could not be read due to a downstream DQ error.
	StatusError OutputStatus = "error"
)

// Input is the tool's typed input.
type Input struct {
	JobName     string `json:"jobName" jsonschema:"Required. Name of the data quality job the rule is attached to (a job, also called a 'dataset', is a saved check on one database table), e.g. 'PUBLIC.SAMPLE_DATASET'."`
	MonitorName string `json:"monitorName" jsonschema:"Required. Name of the rule (the check; Collibra calls it a 'monitor') to read."`
}

// Rule is the returned rule definition.
type Rule struct {
	JobName      string   `json:"jobName"`
	MonitorName  string   `json:"monitorName"`
	MonitorType  string   `json:"monitorType"`
	MonitorValue string   `json:"monitorValue"`
	FilterQuery  string   `json:"filterQuery,omitempty"`
	ColumnName   string   `json:"columnName,omitempty"`
	Description  string   `json:"description,omitempty"`
	Dimensions   []string `json:"dimensions,omitempty"`
	Tolerance    int      `json:"tolerance"`
	Active       bool     `json:"active"`
	Suppressed   bool     `json:"suppressed"`
	TemplateID   string   `json:"templateId,omitempty"`
}

// Output is the typed response.
type Output struct {
	Status  OutputStatus `json:"status" jsonschema:"'success' when the rule was found; 'validation_error' for bad inputs; 'error' for downstream DQ failures."`
	Message string       `json:"message" jsonschema:"Human-readable summary."`
	Rule    *Rule        `json:"rule,omitempty" jsonschema:"The rule definition, on success."`
}

// NewTool returns the registered tool.
func NewTool(collibraClient *http.Client) *chip.Tool[Input, Output] {
	return &chip.Tool[Input, Output]{
		Name:  "get_dq_rule",
		Title: "Get Data Quality Rule",
		Description: "Read the definition of a single data quality rule (a check on a table's data; Collibra calls it a 'monitor') on an existing data quality job (a saved check on ONE database table; also called a 'dataset'). " +
			"Returns the rule's type, SQL, filter, tolerance (count of failing records allowed before it fails) and active/suppressed (kept but not scored) state.",
		Handler:     handler(collibraClient),
		Permissions: []string{},
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}
}

func handler(collibraClient *http.Client) chip.ToolHandlerFunc[Input, Output] {
	return func(ctx context.Context, input Input) (Output, error) {
		if strings.TrimSpace(input.JobName) == "" {
			return Output{Status: StatusValidationError, Message: "jobName is required."}, nil
		}
		if strings.TrimSpace(input.MonitorName) == "" {
			return Output{Status: StatusValidationError, Message: "monitorName is required."}, nil
		}

		r, err := clients.GetDQRule(ctx, collibraClient, strings.TrimSpace(input.JobName), strings.TrimSpace(input.MonitorName))
		if err != nil {
			return Output{Status: StatusError, Message: fmt.Sprintf("Could not read rule: %v", err)}, nil
		}

		return Output{
			Status:  StatusSuccess,
			Message: fmt.Sprintf("Found rule %q on job %q.", r.MonitorName, r.JobName),
			Rule: &Rule{
				JobName:      r.JobName,
				MonitorName:  r.MonitorName,
				MonitorType:  r.MonitorType,
				MonitorValue: r.MonitorValue,
				FilterQuery:  r.FilterQuery,
				ColumnName:   r.ColumnName,
				Description:  r.Description,
				Dimensions:   r.Dimensions,
				Tolerance:    r.Tolerance,
				Active:       r.IsActive == 1,
				Suppressed:   r.IsSuppressed,
				TemplateID:   r.TemplateID,
			},
		}, nil
	}
}
