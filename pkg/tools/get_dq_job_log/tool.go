// Package get_dq_job_log implements the get_dq_job_log MCP tool: it reads the
// execution log for a job run so run stages and exceptions can be observed.
package get_dq_job_log

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/collibra/chip/pkg/chip"
	"github.com/collibra/chip/pkg/clients"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// OutputStatus is the overall outcome of a get_dq_job_log call.
type OutputStatus string

const (
	// StatusSuccess means the log was returned.
	StatusSuccess OutputStatus = "success"
	// StatusValidationError means the inputs failed validation before any read.
	StatusValidationError OutputStatus = "validation_error"
	// StatusError means the log could not be read due to a downstream error.
	StatusError OutputStatus = "error"
)

// Input is the tool's typed input.
type Input struct {
	JobRunID string `json:"jobRunId" jsonschema:"Required. The job run id returned by run_dq_job (or create_dq_job)."`
}

// LogEntry is one entry in a job run's execution log.
type LogEntry struct {
	Activity        string `json:"activity,omitempty" jsonschema:"Activity the entry belongs to, e.g. LOAD, RULES."`
	Stage           string `json:"stage,omitempty" jsonschema:"Stage within the activity."`
	Description     string `json:"description,omitempty" jsonschema:"Log description."`
	Hint            string `json:"hint,omitempty" jsonschema:"Optional hint / remediation text."`
	PrettyStageTime string `json:"prettyStageTime,omitempty" jsonschema:"Human-readable stage duration."`
}

// Output is the typed response.
type Output struct {
	Status  OutputStatus `json:"status" jsonschema:"'success' when the log was returned; 'validation_error' for bad inputs; 'error' for downstream DQ failures."`
	Message string       `json:"message" jsonschema:"Human-readable summary."`
	Entries []LogEntry   `json:"entries,omitempty" jsonschema:"Log entries for the run, in order."`
}

// NewTool returns the registered tool.
func NewTool(collibraClient *http.Client) *chip.Tool[Input, Output] {
	return &chip.Tool[Input, Output]{
		Name:  "get_dq_job_log",
		Title: "Get Data Quality Job Log",
		Description: "Read the execution log for a data quality job run by its jobRunId (returned by run_dq_job / create_dq_job) — " +
			"the run stages, timings, descriptions and any exceptions. " +
			"Note: the DQ public API has no log endpoint, so this uses the internal DQ UI surface.",
		Handler:     handler(collibraClient),
		Permissions: []string{},
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}
}

func handler(collibraClient *http.Client) chip.ToolHandlerFunc[Input, Output] {
	return func(ctx context.Context, input Input) (Output, error) {
		if strings.TrimSpace(input.JobRunID) == "" {
			return Output{Status: StatusValidationError, Message: "jobRunId is required."}, nil
		}

		entries, err := clients.GetDQJobLog(ctx, collibraClient, strings.TrimSpace(input.JobRunID))
		if err != nil {
			return Output{Status: StatusError, Message: fmt.Sprintf("Could not read job log: %v", err)}, nil
		}

		out := make([]LogEntry, 0, len(entries))
		for _, e := range entries {
			out = append(out, LogEntry{
				Activity:        e.Activity,
				Stage:           e.Stage,
				Description:     e.LogDesc,
				Hint:            e.LogHint,
				PrettyStageTime: e.PrettyStageTime,
			})
		}

		return Output{
			Status:  StatusSuccess,
			Message: fmt.Sprintf("Returned %d log entr(ies) for run %s.", len(out), strings.TrimSpace(input.JobRunID)),
			Entries: out,
		}, nil
	}
}
