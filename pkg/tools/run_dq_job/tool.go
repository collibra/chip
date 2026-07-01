// Package run_dq_job implements the run_dq_job MCP tool: it triggers a run of
// an existing data quality job (dataset) via the DQ public API, optionally
// scoped to a run date / time slice and with an optional historical backfill.
package run_dq_job

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/collibra/chip/pkg/chip"
	"github.com/collibra/chip/pkg/clients"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// OutputStatus is the overall outcome of a run_dq_job call.
type OutputStatus string

const (
	// StatusSuccess means the run was created and queued.
	StatusSuccess OutputStatus = "success"
	// StatusValidationError means the inputs failed validation before any
	// write — empty job name or an unsupported dateKind.
	StatusValidationError OutputStatus = "validation_error"
	// StatusError means the run could not be triggered due to a downstream
	// DQ service error.
	StatusError OutputStatus = "error"
)

// dateKind discriminators accepted by the DQ public API.
const (
	dateKindDate      = "DATE"
	dateKindTimestamp = "TIMESTAMP"
)

// Input is the tool's typed input.
type Input struct {
	JobName    string        `json:"jobName" jsonschema:"Required. Name of the data quality job (dataset) to run, e.g. 'PUBLIC.SAMPLE_DATASET'."`
	RunDate    string        `json:"runDate,omitempty" jsonschema:"Optional. Run date. Use 'yyyy-MM-dd' when dateKind is DATE (the default), or an RFC 3339 timestamp ('2026-06-28T10:00:00Z') when dateKind is TIMESTAMP. Defaults to the current date/time."`
	RunDateEnd string        `json:"runDateEnd,omitempty" jsonschema:"Optional. End of the run time slice, same format as runDate."`
	DateKind   string        `json:"dateKind,omitempty" jsonschema:"Optional. How runDate/runDateEnd are interpreted: 'DATE' (yyyy-MM-dd, the default) or 'TIMESTAMP' (RFC 3339)."`
	Backrun    *BackrunInput `json:"backrun,omitempty" jsonschema:"Optional. Historical backfill: trigger additional runs for prior periods relative to runDate."`
}

// BackrunInput configures an optional historical backfill.
type BackrunInput struct {
	TimeBin  string `json:"timeBin" jsonschema:"Time bin for the backfill: 'DAY', 'MONTH', or 'YEAR'."`
	BinValue int    `json:"binValue" jsonschema:"Number of past bins to backfill (minimum 1). E.g. timeBin=DAY, binValue=10 backfills the previous 10 days."`
}

// Output is the typed response.
type Output struct {
	Status   OutputStatus `json:"status" jsonschema:"'success' when the run was created and queued; 'validation_error' for bad inputs; 'error' for downstream DQ failures."`
	Message  string       `json:"message" jsonschema:"Human-readable summary."`
	JobRunID string       `json:"jobRunId,omitempty" jsonschema:"Identifier of the created run, on success. Use it to track the run."`
}

// NewTool returns the registered tool.
func NewTool(collibraClient *http.Client) *chip.Tool[Input, Output] {
	return &chip.Tool[Input, Output]{
		Name:  "run_dq_job",
		Title: "Run Data Quality Job",
		Description: "Trigger a run of an existing data quality job (dataset), executing its configured rules. " +
			"runDate/runDateEnd are optional and default to the current date/time; use dateKind to switch between a calendar date (DATE) and a timestamp (TIMESTAMP). " +
			"An optional backrun backfills prior periods. " +
			"Returns the job run id so the run can be tracked. " +
			"Requires permission to run the target job.",
		Handler:     handler(collibraClient),
		Permissions: []string{},
		Annotations: &mcp.ToolAnnotations{DestructiveHint: chip.Ptr(false)},
	}
}

func handler(collibraClient *http.Client) chip.ToolHandlerFunc[Input, Output] {
	return func(ctx context.Context, input Input) (Output, error) {
		if strings.TrimSpace(input.JobName) == "" {
			return Output{Status: StatusValidationError, Message: "jobName is required."}, nil
		}

		kind, out := resolveDateKind(input.DateKind)
		if out != nil {
			return *out, nil
		}

		request := buildRequest(input, kind)

		resp, err := clients.RunDQJob(ctx, collibraClient, strings.TrimSpace(input.JobName), request)
		if err != nil {
			return Output{Status: StatusError, Message: fmt.Sprintf("Could not run job: %v", err)}, nil
		}

		return Output{
			Status:   StatusSuccess,
			Message:  fmt.Sprintf("Triggered run of job %q (run id %s).", strings.TrimSpace(input.JobName), resp.JobRunID),
			JobRunID: resp.JobRunID,
		}, nil
	}
}

// resolveDateKind defaults an empty dateKind to DATE and rejects anything else.
func resolveDateKind(dateKind string) (string, *Output) {
	switch dateKind {
	case "", dateKindDate:
		return dateKindDate, nil
	case dateKindTimestamp:
		return dateKindTimestamp, nil
	default:
		return "", &Output{
			Status:  StatusValidationError,
			Message: fmt.Sprintf("dateKind %q is invalid. Use %q or %q.", dateKind, dateKindDate, dateKindTimestamp),
		}
	}
}

// buildRequest assembles the optional run request body. It returns nil when no
// run parameters were supplied, so the job runs with the service defaults.
func buildRequest(input Input, kind string) *clients.RunDQJobRequest {
	var request clients.RunDQJobRequest
	set := false

	if strings.TrimSpace(input.RunDate) != "" {
		request.RunDate = &clients.DQRunDate{Kind: kind, Value: input.RunDate}
		set = true
	}
	if strings.TrimSpace(input.RunDateEnd) != "" {
		request.RunDateEnd = &clients.DQRunDate{Kind: kind, Value: input.RunDateEnd}
		set = true
	}
	if input.Backrun != nil {
		request.Backrun = &clients.DQBackrun{TimeBin: input.Backrun.TimeBin, BinValue: input.Backrun.BinValue}
		set = true
	}

	if !set {
		return nil
	}
	return &request
}
