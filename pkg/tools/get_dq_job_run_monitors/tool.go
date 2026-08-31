// Package get_dq_job_run_monitors implements the dq_get_job_run_monitors MCP tool — read the
// per-monitor (adaptive + custom) results for a single Collibra data-quality job run by id.
//
//	GET /rest/dq/1.0/jobRuns/{jobRunId}/monitors
//
// This is a pure read: no confirm checkpoint, no writes. 400/401/403/404/500 and transport failures
// are surfaced as messages with actionable guidance rather than Go errors.
package get_dq_job_run_monitors

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
	RunID string `json:"run_id" jsonschema:"Required. The id (jobRunId) of the job run whose monitor results to retrieve."`
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
	Tolerance     string   `json:"tolerance,omitempty"`
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
	Tolerance          int      `json:"tolerance,omitempty"`
}

// Output is the typed response.
type Output struct {
	Status           Status                  `json:"status" jsonschema:"'success' when the run was found; 'needs_input' for bad inputs; 'error' for downstream DQ failures."`
	Message          string                  `json:"message" jsonschema:"Human-readable summary."`
	AdaptiveMonitors []AdaptiveMonitorResult `json:"adaptiveMonitors,omitempty" jsonschema:"Per-monitor results for the job's built-in adaptive monitors."`
	CustomMonitors   []CustomMonitorResult   `json:"customMonitors,omitempty" jsonschema:"Per-monitor results for the job's custom DQ rules."`
	Guidance         string                  `json:"guidance,omitempty" jsonschema:"On needs_input/error, what to do next."`
}

// NewTool returns the registered tool.
func NewTool(collibraClient *http.Client) *chip.Tool[Input, Output] {
	return &chip.Tool[Input, Output]{
		Name:  "dq_get_job_run_monitors",
		Title: "Get Data Quality Job Run Monitors",
		Description: "Reads the per-monitor breakdown (adaptive + custom DQ rules) behind a Collibra " +
			"data-quality job run's score, given its run_id (jobRunId).\n\n" +
			"Example user requests: \"Show me the monitor results for DQ run <id>\"; \"Which monitors broke " +
			"in run <id>?\"",
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
				Message:  "Provide the run whose monitor results to retrieve.",
				Guidance: "Supply run_id — the id (jobRunId) of the job run to retrieve.",
			}, nil
		}

		monitors, code, err := clients.GetDqJobRunMonitors(ctx, collibraClient, runID)
		if err != nil {
			return lookupError(code, err, runID), nil
		}

		out := Output{
			Status:  StatusSuccess,
			Message: fmt.Sprintf("Found monitor results for run %q.", runID),
		}
		for _, m := range monitors.AdaptiveMonitors {
			out.AdaptiveMonitors = append(out.AdaptiveMonitors, AdaptiveMonitorResult{
				MonitorName: m.MonitorName, MonitorType: m.MonitorType, PrimaryColumn: m.PrimaryColumn,
				State: m.State, ObservedValue: m.ObservedValue, ExpectedMin: m.ExpectedMin,
				ExpectedMax: m.ExpectedMax, Tolerance: m.Tolerance, IsSuppressed: m.IsSuppressed,
				Dimensions: m.Dimensions,
			})
		}
		for _, m := range monitors.CustomMonitors {
			out.CustomMonitors = append(out.CustomMonitors, CustomMonitorResult{
				MonitorName: m.MonitorName, State: m.State, Score: m.Score,
				BreakingPercentage: m.BreakingPercentage, RowsPassing: m.RowsPassing,
				RowsBreaking: m.RowsBreaking, RowsTotal: m.RowsTotal, Exception: m.Exception,
				Dimensions: m.Dimensions, Tolerance: m.Tolerance,
			})
		}

		return out, nil
	}
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
		out.Message = fmt.Sprintf("You do not have permission to view monitor results for run %q (HTTP 403).", runID)
		out.Guidance = "You need the Data Quality Job > View permission on the connection. Ask an administrator for the Data Quality Editor/Manager role, then retry."
	case http.StatusBadRequest:
		out.Message = fmt.Sprintf("The data-quality API rejected the lookup of run %q (HTTP 400): %v", runID, err)
		out.Guidance = "Check that run_id is correct and well-formed, then retry."
	case 0:
		out.Message = fmt.Sprintf("Failed to look up monitor results for run %q: %v", runID, err)
		out.Guidance = "A network/transport error occurred contacting the data-quality API. Retry."
	default:
		out.Message = fmt.Sprintf("Failed to look up monitor results for run %q (HTTP %d): %v", runID, code, err)
		out.Guidance = "This is likely a server-side error. Retry shortly; if it persists, contact your Collibra administrator."
	}
	return out
}
