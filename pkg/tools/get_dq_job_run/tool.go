// Package get_dq_job_run implements the dq_get_job_run MCP tool — read the full details of a single
// Collibra data-quality job run by id.
//
// The flow combines two public GETs:
//
//	Run     : GET /rest/dq/1.0/jobRuns/{jobRunId}       -> lifecycle fields, and (once terminal)
//	          score/rowCount/executionTimeSeconds/activeMonitors/breakingMonitors.
//	Monitors: GET /rest/dq/1.0/jobRuns/{jobRunId}/monitors -> the per-monitor (adaptive + custom)
//	          breakdown behind the run's aggregate score.
//
// This is a pure read: no confirm checkpoint, no writes. A failure fetching the per-monitor breakdown
// does not fail the whole call — the run's own details are still returned, with a note in guidance.
// 400/401/403/404/500 and transport failures are surfaced as messages with actionable guidance rather
// than Go errors.
package get_dq_job_run

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/collibra/chip/pkg/chip"
	"github.com/collibra/chip/pkg/clients"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type Status string

const (
	StatusSuccess    Status = "success"
	StatusNeedsInput Status = "needs_input"
	StatusError      Status = "error"
)

// Input is the tool's typed input.
type Input struct {
	RunID string `json:"run_id" jsonschema:"Required. The id (jobRunId) of the job run to retrieve."`
}

// AdaptiveMonitorResult is one adaptive monitor's outcome for this run.
type AdaptiveMonitorResult struct {
	MonitorName   string   `json:"monitorName"`
	MonitorType   string   `json:"monitorType,omitempty"`
	PrimaryColumn string   `json:"primaryColumn,omitempty"`
	State         string   `json:"state,omitempty" jsonschema:"LEARNING | PASSING | BREAKING | SUPPRESSED | USER_PASSED | EXCEPTION | STALE | SKIPPED."`
	ObservedValue string   `json:"observedValue,omitempty"`
	ExpectedMin   string   `json:"expectedMin,omitempty"`
	ExpectedMax   string   `json:"expectedMax,omitempty"`
	IsSuppressed  bool     `json:"isSuppressed,omitempty"`
	Dimensions    []string `json:"dimensions,omitempty"`
}

// CustomMonitorResult is one custom monitor's (DQ rule's) outcome for this run.
type CustomMonitorResult struct {
	MonitorName        string   `json:"monitorName"`
	State              string   `json:"state,omitempty"`
	Score              float64  `json:"score" jsonschema:"Score based on sum of points per breaking row (0-100)."`
	BreakingPercentage float64  `json:"breakingPercentage,omitempty"`
	RowsPassing        int64    `json:"rowsPassing,omitempty"`
	RowsBreaking       int64    `json:"rowsBreaking,omitempty"`
	RowsTotal          int64    `json:"rowsTotal,omitempty"`
	Exception          string   `json:"exception,omitempty"`
	Dimensions         []string `json:"dimensions,omitempty"`
}

// RunDetail is the full run detail returned on success.
type RunDetail struct {
	JobRunID             string                  `json:"jobRunId"`
	JobName              string                  `json:"jobName,omitempty"`
	Status               string                  `json:"status" jsonschema:"WAITING | DISPATCHED | SETUP | RUNNING | SENDING | FINISHED | CANCELLED | FAILED | UNKNOWN."`
	Activity             string                  `json:"activity,omitempty" jsonschema:"Current activity within the run (e.g. PROFILE, RULES), when in progress."`
	RunDate              string                  `json:"runDate,omitempty"`
	StartTime            string                  `json:"startTime,omitempty"`
	EndTime              string                  `json:"endTime,omitempty" jsonschema:"When the run reached a terminal state; empty while in progress."`
	ExecutionTimeSeconds *int64                  `json:"executionTimeSeconds,omitempty" jsonschema:"Wall-clock duration; set once terminal."`
	Score                *float64                `json:"score,omitempty" jsonschema:"Overall data quality score for the run as a percentage [0,100]; set once scoring completed."`
	RowCount             *int64                  `json:"rowCount,omitempty" jsonschema:"Rows scanned/processed by the run; set once the data scan completed."`
	ActiveMonitors       *int                    `json:"activeMonitors,omitempty" jsonschema:"Count of monitors evaluated during the run."`
	BreakingMonitors     *int                    `json:"breakingMonitors,omitempty" jsonschema:"Count of monitors that reported a breaking status."`
	Exception            string                  `json:"exception,omitempty" jsonschema:"Failure message, only set when status is FAILED."`
	SourceQuery          string                  `json:"sourceQuery,omitempty"`
	ExecutedQuery        string                  `json:"executedQuery,omitempty" jsonschema:"sourceQuery with ${rd}/${rdEnd} substituted for this run."`
	AdaptiveMonitors     []AdaptiveMonitorResult `json:"adaptiveMonitors,omitempty" jsonschema:"Per-monitor results for the job's built-in adaptive monitors."`
	CustomMonitors       []CustomMonitorResult   `json:"customMonitors,omitempty" jsonschema:"Per-monitor results for the job's custom DQ rules."`
	JobDetailsLink       string                  `json:"jobDetailsLink,omitempty" jsonschema:"Job Details deep-link path (relative to the Collibra instance URL), when jobName is known."`
}

// Output is the typed response.
type Output struct {
	Status   Status     `json:"status" jsonschema:"'success' when the run was found; 'needs_input' for bad inputs; 'error' for downstream DQ failures."`
	Message  string     `json:"message" jsonschema:"Human-readable summary."`
	Run      *RunDetail `json:"run,omitempty" jsonschema:"The run's details, on success."`
	Guidance string     `json:"guidance,omitempty" jsonschema:"On needs_input/error, what to do next."`
}

// NewTool returns the registered tool.
func NewTool(collibraClient *http.Client) *chip.Tool[Input, Output] {
	return &chip.Tool[Input, Output]{
		Name:  "dq_get_job_run",
		Title: "Get Data Quality Job Run",
		Description: "Reads the full details of a single Collibra data-quality job run by its run_id " +
			"(jobRunId). Returns the run's lifecycle status/activity, timing, and — once the run reaches a " +
			"terminal state (FINISHED/CANCELLED/FAILED) — its overall score, row count, execution time, and " +
			"the per-monitor breakdown (adaptive + custom DQ rules) behind that score.\n\n" +
			"Fields that are only meaningful once a run has finished (score, rowCount, executionTimeSeconds, " +
			"per-monitor results) are absent while the run is still in progress.\n\n" +
			"Example user requests: \"Show me the details of DQ run <id>\"; \"What was the score for run <id>?\"; " +
			"\"Why did job run <id> fail?\"",
		Handler:     handler(collibraClient),
		Permissions: []string{},
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, DestructiveHint: chip.Ptr(false), IdempotentHint: true, OpenWorldHint: chip.Ptr(false)},
	}
}

func handler(collibraClient *http.Client) chip.ToolHandlerFunc[Input, Output] {
	return func(ctx context.Context, input Input) (Output, error) {
		runID := strings.TrimSpace(input.RunID)
		if runID == "" {
			return Output{
				Status:   StatusNeedsInput,
				Message:  "Provide the run to retrieve.",
				Guidance: "Supply run_id — the id (jobRunId) of the job run to retrieve.",
			}, nil
		}

		run, code, err := clients.GetDqJobRun(ctx, collibraClient, runID)
		if err != nil {
			return lookupError(code, err, runID), nil
		}

		detail := runDetail(run)

		monitors, mCode, mErr := clients.GetDqJobRunMonitors(ctx, collibraClient, runID)
		if mErr != nil {
			out := Output{
				Status:  StatusSuccess,
				Message: fmt.Sprintf("Found run %q, but could not read its per-monitor results.", runID),
				Run:     &detail,
			}
			out.Guidance = monitorsLookupNote(mCode, mErr)
			return out, nil
		}
		for _, m := range monitors.AdaptiveMonitors {
			detail.AdaptiveMonitors = append(detail.AdaptiveMonitors, AdaptiveMonitorResult{
				MonitorName: m.MonitorName, MonitorType: m.MonitorType, PrimaryColumn: m.PrimaryColumn,
				State: m.State, ObservedValue: m.ObservedValue, ExpectedMin: m.ExpectedMin,
				ExpectedMax: m.ExpectedMax, IsSuppressed: m.IsSuppressed, Dimensions: m.Dimensions,
			})
		}
		for _, m := range monitors.CustomMonitors {
			detail.CustomMonitors = append(detail.CustomMonitors, CustomMonitorResult{
				MonitorName: m.MonitorName, State: m.State, Score: m.Score,
				BreakingPercentage: m.BreakingPercentage, RowsPassing: m.RowsPassing,
				RowsBreaking: m.RowsBreaking, RowsTotal: m.RowsTotal, Exception: m.Exception,
				Dimensions: m.Dimensions,
			})
		}

		return Output{
			Status:  StatusSuccess,
			Message: fmt.Sprintf("Found run %q (status %s).", runID, run.Status),
			Run:     &detail,
		}, nil
	}
}

func runDetail(run *clients.DqJobRun) RunDetail {
	detail := RunDetail{
		JobRunID:             run.JobRunID,
		JobName:              run.JobName,
		Status:               run.Status,
		Activity:             run.Activity,
		RunDate:              run.RunDateValue(),
		StartTime:            run.StartTime,
		EndTime:              run.EndTime,
		ExecutionTimeSeconds: run.ExecutionTimeSeconds,
		Score:                run.Score,
		RowCount:             run.RowCount,
		ActiveMonitors:       run.ActiveMonitors,
		BreakingMonitors:     run.BreakingMonitors,
		Exception:            run.Exception,
		SourceQuery:          run.SourceQuery,
		ExecutedQuery:        run.ExecutedQuery,
	}
	if run.JobName != "" {
		detail.JobDetailsLink = clients.DqJobDetailsPath(run.JobName)
	}
	return detail
}

func monitorsLookupNote(code int, err error) string {
	return fmt.Sprintf("The run itself was found, but reading its per-monitor results failed (HTTP %d): %v. Retry, or check the run's monitor breakdown in the Collibra UI.", code, err)
}

func lookupError(code int, err error, runID string) Output {
	out := Output{Status: StatusError}
	switch code {
	case http.StatusNotFound:
		out.Message = fmt.Sprintf("No job run with id %q was found (HTTP 404).", runID)
		out.Guidance = "Verify the run_id — it may be wrong, or the run has since been deleted."
	case http.StatusUnauthorized:
		out.Message = "Not authenticated to the data-quality API (HTTP 401)."
		out.Guidance = "Your Collibra session/token is missing or expired — re-authenticate and retry."
	case http.StatusForbidden:
		out.Message = fmt.Sprintf("You do not have permission to view run %q (HTTP 403).", runID)
		out.Guidance = "You need the Data Quality Job > View permission on the connection. Ask an administrator for the Data Quality Editor/Manager role, then retry."
	case http.StatusBadRequest:
		out.Message = fmt.Sprintf("The data-quality API rejected the lookup of run %q (HTTP 400): %v", runID, err)
		out.Guidance = "Check that run_id is correct and well-formed, then retry."
	case 0:
		out.Message = fmt.Sprintf("Failed to look up run %q: %v", runID, err)
		out.Guidance = "A network/transport error occurred contacting the data-quality API. Retry."
	default:
		out.Message = fmt.Sprintf("Failed to look up run %q (HTTP %d): %v", runID, code, err)
		out.Guidance = "This is likely a server-side error. Retry shortly; if it persists, contact your Collibra administrator."
	}
	return out
}
