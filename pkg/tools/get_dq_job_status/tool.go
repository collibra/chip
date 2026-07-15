// Package get_dq_job_status implements the get_dq_job_status MCP tool: it reads
// the status of a job run by the jobRunId returned when the job was run.
package get_dq_job_status

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/collibra/chip/pkg/chip"
	"github.com/collibra/chip/pkg/clients"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// OutputStatus is the overall outcome of a get_dq_job_status call.
type OutputStatus string

const (
	// StatusSuccess means the run status was returned.
	StatusSuccess OutputStatus = "success"
	// StatusValidationError means the inputs failed validation before any read.
	StatusValidationError OutputStatus = "validation_error"
	// StatusError means the status could not be read due to a downstream error.
	StatusError OutputStatus = "error"
)

// Input is the tool's typed input.
type Input struct {
	JobRunID string `json:"jobRunId" jsonschema:"Required. The jobRunId — id of one execution (run) of the job — returned by run_dq_job (or create_dq_job)."`
}

// Output is the typed response.
type Output struct {
	Status           OutputStatus `json:"status" jsonschema:"'success' when the status was returned; 'validation_error' for bad inputs; 'error' for downstream DQ failures."`
	Message          string       `json:"message" jsonschema:"Human-readable summary."`
	JobRunID         string       `json:"jobRunId,omitempty" jsonschema:"The job run id, on success."`
	JobName          string       `json:"jobName,omitempty" jsonschema:"The job name (the job, also called a 'dataset'), on success."`
	RunStatus        string       `json:"runStatus,omitempty" jsonschema:"Run status, e.g. WAITING, RUNNING, FINISHED, CANCELLED, FAILED (open set)."`
	Activity         string       `json:"activity,omitempty" jsonschema:"Current/last activity, e.g. LOAD, RULES, RESULTS."`
	Exception        string       `json:"exception,omitempty" jsonschema:"Failure detail, populated only on FAILED runs."`
	Score            *float64     `json:"score,omitempty" jsonschema:"Overall run score (0-100), when available."`
	BreakingMonitors *int         `json:"breakingMonitors,omitempty" jsonschema:"Number of breaking (failing) rules ('monitors') in this run, when available."`
	RowCount         *int64       `json:"rowCount,omitempty" jsonschema:"Rows scanned, when available."`
}

// NewTool returns the registered tool.
func NewTool(collibraClient *http.Client) *chip.Tool[Input, Output] {
	return &chip.Tool[Input, Output]{
		Name:  "get_dq_job_status",
		Title: "Get Data Quality Job Status",
		Description: "Read the status of one execution (run) of a data quality job (a saved check on ONE database table; also called a 'dataset') by its jobRunId — the id of that run, returned by run_dq_job / create_dq_job. " +
			"Returns the run status (RUNNING, FINISHED, FAILED, ...), current activity, the 0-100 score, the count of breaking (failing) rules and any failure exception.",
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

		run, err := clients.GetDQJobRunStatus(ctx, collibraClient, strings.TrimSpace(input.JobRunID))
		if err != nil {
			return Output{Status: StatusError, Message: fmt.Sprintf("Could not read job status: %v", err)}, nil
		}

		return Output{
			Status:           StatusSuccess,
			Message:          fmt.Sprintf("Job %q run %s is %s.", run.JobName, run.JobRunID, run.Status),
			JobRunID:         run.JobRunID,
			JobName:          run.JobName,
			RunStatus:        run.Status,
			Activity:         run.Activity,
			Exception:        run.Exception,
			Score:            run.Score,
			BreakingMonitors: run.BreakingMonitors,
			RowCount:         run.RowCount,
		}, nil
	}
}
