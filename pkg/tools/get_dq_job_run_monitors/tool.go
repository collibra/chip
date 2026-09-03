// Package get_dq_job_run_monitors implements the dq_get_job_run_monitors MCP tool —
// read the per-monitor results, adaptive and custom, that a Collibra data-quality
// job run produced, by run id.
//
// The flow is a single public GET:
//
//	GET /rest/dq/1.0/jobRuns/{jobRunId}/monitors -> the adaptive and custom monitor
//	                                                results behind the run's score.
//
// dq_get_job_run returns the same breakdown alongside a run's lifecycle details;
// this tool is the focused read for when only the monitors are wanted, and it also
// surfaces each monitor's tolerance (the threshold it was judged against), which the
// run tool omits.
//
// This is a pure read: no confirm checkpoint, no writes. Monitor results exist only
// once a run has produced them, so a run that never completed comes back empty; that
// is reported as an error explaining why rather than as an empty success.
// 400/401/403/404/500 and transport failures are surfaced as messages with actionable
// guidance rather than Go errors.
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

// stateBreaking and stateException are the two states that mean a monitor did not
// pass, counted separately in the summary so a caller can triage a run at a glance.
const (
	stateBreaking  = "BREAKING"
	stateException = "EXCEPTION"
	statePassing   = "PASSING"
)

// Input is the tool's typed input.
type Input struct {
	RunID string `json:"run_id" jsonschema:"Required. The id (jobRunId) of the job run whose monitor results to retrieve."`
}

// AdaptiveMonitorResult is one adaptive monitor's outcome for this run. Adaptive
// monitors are the ones DQ maintains itself by learning a column's behaviour, so the
// threshold is the learned expected range rather than a fixed number.
type AdaptiveMonitorResult struct {
	MonitorName   string   `json:"monitorName"`
	MonitorType   string   `json:"monitorType,omitempty" jsonschema:"What the monitor watches, e.g. NULL, EMPTY, UNIQUENESS, MIN VALUE, ROW_COUNT, DATA_TYPE, SCHEMA_CHANGE."`
	PrimaryColumn string   `json:"primaryColumn,omitempty" jsonschema:"Column the monitor watches; absent for monitors that span the whole table."`
	State         string   `json:"state,omitempty" jsonschema:"LEARNING | PASSING | BREAKING | SUPPRESSED | USER_PASSED | EXCEPTION | STALE | SKIPPED."`
	ObservedValue string   `json:"observedValue,omitempty" jsonschema:"The value this run actually observed."`
	ExpectedMin   string   `json:"expectedMin,omitempty" jsonschema:"Lower bound of the learned expected range the observed value was judged against."`
	ExpectedMax   string   `json:"expectedMax,omitempty" jsonschema:"Upper bound of the learned expected range the observed value was judged against."`
	Tolerance     string   `json:"tolerance,omitempty" jsonschema:"Sensitivity tier of the learned threshold (e.g. NARROW, NEUTRAL, WIDE)."`
	IsSuppressed  bool     `json:"isSuppressed,omitempty" jsonschema:"Whether the monitor is suppressed, meaning it is kept but not scored."`
	Dimensions    []string `json:"dimensions,omitempty" jsonschema:"Data quality dimensions the monitor contributes to."`
}

// CustomMonitorResult is one custom monitor's (user-authored DQ rule's) outcome for
// this run. Its threshold is the rule's tolerance: how many breaking rows are allowed
// before the rule fails.
type CustomMonitorResult struct {
	MonitorName        string   `json:"monitorName"`
	State              string   `json:"state,omitempty" jsonschema:"LEARNING | PASSING | BREAKING | SUPPRESSED | USER_PASSED | EXCEPTION | STALE | SKIPPED."`
	Score              float64  `json:"score" jsonschema:"Score based on sum of points per breaking row (0-100)."`
	BreakingPercentage float64  `json:"breakingPercentage,omitempty" jsonschema:"Breaking rows as a percentage of rows evaluated."`
	RowsPassing        int64    `json:"rowsPassing,omitempty"`
	RowsBreaking       int64    `json:"rowsBreaking,omitempty" jsonschema:"The observed value for a custom rule: how many rows failed the rule this run."`
	RowsTotal          int64    `json:"rowsTotal,omitempty"`
	Tolerance          int      `json:"tolerance,omitempty" jsonschema:"The rule's threshold: count of breaking rows allowed before the rule is judged as failing."`
	Exception          string   `json:"exception,omitempty" jsonschema:"Failure message, only set when the monitor errored (state EXCEPTION)."`
	Dimensions         []string `json:"dimensions,omitempty" jsonschema:"Data quality dimensions the monitor contributes to."`
}

// MonitorSummary counts the run's monitors so a caller can triage without walking
// both lists.
type MonitorSummary struct {
	Total     int `json:"total" jsonschema:"Total monitor results for the run."`
	Adaptive  int `json:"adaptive" jsonschema:"Number of adaptive monitor results."`
	Custom    int `json:"custom" jsonschema:"Number of custom (DQ rule) monitor results."`
	Passing   int `json:"passing" jsonschema:"Number of monitors in state PASSING."`
	Breaking  int `json:"breaking" jsonschema:"Number of monitors in state BREAKING."`
	Exception int `json:"exception" jsonschema:"Number of monitors in state EXCEPTION."`
}

// MonitorResults is the run's full monitor breakdown returned on success.
type MonitorResults struct {
	JobRunID         string                  `json:"jobRunId"`
	Summary          MonitorSummary          `json:"summary"`
	AdaptiveMonitors []AdaptiveMonitorResult `json:"adaptiveMonitors" jsonschema:"Per-monitor results for the monitors DQ learns and maintains itself."`
	CustomMonitors   []CustomMonitorResult   `json:"customMonitors" jsonschema:"Per-monitor results for the job's user-authored DQ rules."`
}

// Output is the typed response.
type Output struct {
	Status   Status          `json:"status" jsonschema:"'success' when monitor results were returned; 'needs_input' for bad inputs; 'error' for downstream DQ failures or when the run has no monitor results."`
	Message  string          `json:"message" jsonschema:"Human-readable summary."`
	Monitors *MonitorResults `json:"monitors,omitempty" jsonschema:"The run's monitor results, on success."`
	Guidance string          `json:"guidance,omitempty" jsonschema:"On needs_input/error, what to do next."`
}

// NewTool returns the registered tool.
func NewTool(collibraClient *http.Client) *chip.Tool[Input, Output] {
	return &chip.Tool[Input, Output]{
		Name:  "dq_get_job_run_monitors",
		Title: "Get Data Quality Job Run Monitors",
		Description: "Reads the per-monitor results of a single Collibra data-quality job run by its run_id (jobRunId) — both the " +
			"adaptive monitors DQ learns and maintains itself (nulls, empties, uniqueness, min/max/mean, row count, data type, schema " +
			"change) and the job's custom DQ rules.\n\n" +
			"Adaptive monitors report the observed value against the learned expected range (expectedMin/expectedMax) and the " +
			"sensitivity tier of that range. Custom rules report their score, breaking/passing row counts and the tolerance (how many " +
			"breaking rows are allowed before the rule fails). A summary counts monitors by state so a failing run can be triaged at a " +
			"glance.\n\n" +
			"Monitor results exist only for runs that produced them, so a run that never completed returns an error saying so. " +
			"dq_get_job_run returns this same breakdown together with the run's lifecycle details — prefer this tool when only the " +
			"monitors are wanted, or when the monitor tolerances are needed.\n\n" +
			"Example user requests: \"Which monitors failed in DQ run <id>?\"; \"Show me the monitor results for run <id>\"; " +
			"\"What was the expected range for the monitor that broke in run <id>?\"",
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
				Guidance: "Supply run_id — the id (jobRunId) of the job run whose monitor results to retrieve.",
			}, nil
		}

		result, code, err := clients.GetDqJobRunMonitors(ctx, collibraClient, runID)
		if err != nil {
			return lookupError(code, err, runID), nil
		}
		if len(result.AdaptiveMonitors) == 0 && len(result.CustomMonitors) == 0 {
			return noMonitorResults(runID), nil
		}

		monitors := monitorResults(result, runID)
		return Output{
			Status:   StatusSuccess,
			Message:  monitorSummaryMessage(monitors, runID),
			Monitors: &monitors,
		}, nil
	}
}

func monitorResults(result *clients.DqJobRunMonitorsResult, runID string) MonitorResults {
	monitors := MonitorResults{
		JobRunID:         runID,
		AdaptiveMonitors: make([]AdaptiveMonitorResult, 0, len(result.AdaptiveMonitors)),
		CustomMonitors:   make([]CustomMonitorResult, 0, len(result.CustomMonitors)),
	}
	for _, m := range result.AdaptiveMonitors {
		monitors.AdaptiveMonitors = append(monitors.AdaptiveMonitors, AdaptiveMonitorResult{
			MonitorName: m.MonitorName, MonitorType: m.MonitorType, PrimaryColumn: m.PrimaryColumn,
			State: m.State, ObservedValue: m.ObservedValue, ExpectedMin: m.ExpectedMin,
			ExpectedMax: m.ExpectedMax, Tolerance: m.Tolerance, IsSuppressed: m.IsSuppressed,
			Dimensions: m.Dimensions,
		})
	}
	for _, m := range result.CustomMonitors {
		monitors.CustomMonitors = append(monitors.CustomMonitors, CustomMonitorResult{
			MonitorName: m.MonitorName, State: m.State, Score: m.Score,
			BreakingPercentage: m.BreakingPercentage, RowsPassing: m.RowsPassing,
			RowsBreaking: m.RowsBreaking, RowsTotal: m.RowsTotal, Tolerance: m.Tolerance,
			Exception: m.Exception, Dimensions: m.Dimensions,
		})
	}
	monitors.Summary = summarize(monitors)
	return monitors
}

func summarize(monitors MonitorResults) MonitorSummary {
	summary := MonitorSummary{
		Adaptive: len(monitors.AdaptiveMonitors),
		Custom:   len(monitors.CustomMonitors),
	}
	summary.Total = summary.Adaptive + summary.Custom
	for _, m := range monitors.AdaptiveMonitors {
		countState(&summary, m.State)
	}
	for _, m := range monitors.CustomMonitors {
		countState(&summary, m.State)
	}
	return summary
}

func countState(summary *MonitorSummary, state string) {
	switch strings.ToUpper(strings.TrimSpace(state)) {
	case statePassing:
		summary.Passing++
	case stateBreaking:
		summary.Breaking++
	case stateException:
		summary.Exception++
	}
}

func monitorSummaryMessage(monitors MonitorResults, runID string) string {
	summary := monitors.Summary
	message := fmt.Sprintf("Returned %d monitor result(s) for run %q (%d adaptive, %d custom).",
		summary.Total, runID, summary.Adaptive, summary.Custom)
	if summary.Breaking > 0 || summary.Exception > 0 {
		return fmt.Sprintf("%s %d breaking, %d exception.", message, summary.Breaking, summary.Exception)
	}
	return message
}

// noMonitorResults explains a run that returned neither adaptive nor custom results.
func noMonitorResults(runID string) Output {
	return Output{
		Status:  StatusError,
		Message: fmt.Sprintf("Run %q has no monitor results.", runID),
		Guidance: "Monitor results exist only once a run has produced them. Either the run did not complete (check its status with " +
			"dq_get_job_run), or the job has no monitors configured. Confirm the run finished, then retry.",
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
		out.Message = fmt.Sprintf("You do not have permission to view the monitor results for run %q (HTTP 403).", runID)
		out.Guidance = "You need the Data Quality Job > View permission on the job. Ask an administrator for the Data Quality Editor/Manager role, then retry."
	case http.StatusBadRequest:
		out.Message = fmt.Sprintf("The data-quality API rejected the monitor lookup for run %q (HTTP 400): %v", runID, err)
		out.Guidance = "Check that run_id is correct and well-formed, then retry."
	case 0:
		out.Message = fmt.Sprintf("Failed to read the monitor results for run %q: %v", runID, err)
		out.Guidance = "A network/transport error occurred contacting the data-quality API. Retry."
	default:
		out.Message = fmt.Sprintf("Failed to read the monitor results for run %q (HTTP %d): %v", runID, code, err)
		out.Guidance = "This is likely a server-side error. Retry shortly; if it persists, contact your Collibra administrator."
	}
	return out
}
