// Package delete_dq_rule implements the delete_dq_rule MCP tool: it deletes a
// data quality rule (monitor) from an existing DQ job.
package delete_dq_rule

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/collibra/chip/pkg/chip"
	"github.com/collibra/chip/pkg/clients"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// OutputStatus is the overall outcome of a delete_dq_rule call.
type OutputStatus string

const (
	// StatusSuccess means the rule was deleted.
	StatusSuccess OutputStatus = "success"
	// StatusValidationError means the inputs failed validation before any write.
	StatusValidationError OutputStatus = "validation_error"
	// StatusError means the rule could not be deleted due to a downstream error.
	StatusError OutputStatus = "error"
)

// Input is the tool's typed input.
type Input struct {
	JobName     string `json:"jobName" jsonschema:"Required. Name of the data quality job (dataset) the rule is attached to."`
	MonitorName string `json:"monitorName" jsonschema:"Required. Name of the rule (monitor) to delete."`
}

// Output is the typed response.
type Output struct {
	Status  OutputStatus `json:"status" jsonschema:"'success' when the rule was deleted; 'validation_error' for bad inputs; 'error' for downstream DQ failures."`
	Message string       `json:"message" jsonschema:"Human-readable summary."`
}

// NewTool returns the registered tool.
func NewTool(collibraClient *http.Client) *chip.Tool[Input, Output] {
	return &chip.Tool[Input, Output]{
		Name:  "delete_dq_rule",
		Title: "Delete Data Quality Rule",
		Description: "Delete a data quality rule (monitor) from an existing data quality job. " +
			"This is irreversible. Requires permission to delete rules on the target job.",
		Handler:     handler(collibraClient),
		Permissions: []string{},
		Annotations: &mcp.ToolAnnotations{DestructiveHint: chip.Ptr(true)},
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

		err := clients.DeleteDQRule(ctx, collibraClient, strings.TrimSpace(input.JobName), strings.TrimSpace(input.MonitorName))
		if err != nil {
			return Output{Status: StatusError, Message: fmt.Sprintf("Could not delete rule: %v", err)}, nil
		}

		return Output{
			Status:  StatusSuccess,
			Message: fmt.Sprintf("Deleted rule %q from job %q.", strings.TrimSpace(input.MonitorName), strings.TrimSpace(input.JobName)),
		}, nil
	}
}
