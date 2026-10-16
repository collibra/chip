// Package get_dq_job_run_monitors implements the get_data_quality_job_run_monitors MCP tool —
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
	MonitorType   string   `json:"monitorType,omitempty" jsonschema:"What the monitor watches. Values are reported by the service exactly as shown and are not uniformly separated — some use underscores and some a space: NULL, EMPTY, UNIQUENESS, DATA_TYPE, ROW_COUNT, SCHEMA_CHANGE, \"MIN VALUE\", \"MAX VALUE\", \"MEAN VALUE\". Echo the string back as received rather than normalising it."`
	PrimaryColumn string   `json:"primaryColumn,omitempty" jsonschema:"Column the monitor watches; absent for monitors that span the whole table."`
	State         string   `json:"state,omitempty" jsonschema:"Outcome of this monitor for this run. PASSING = the check held. BREAKING = it did not; this is a failure. EXCEPTION = the check could not run (see exception); treat as a failure to investigate, not a pass. LEARNING = an adaptive monitor still building its baseline, so it is not yet scored and is NOT a failure. SUPPRESSED = kept on the job but excluded from scoring, also not a failure. USER_PASSED = a human overrode a breaking result to passing. STALE = carried over from an earlier run because this run did not re-evaluate it. SKIPPED = not evaluated this run. Only BREAKING and EXCEPTION mean something is wrong."`
	ObservedValue string   `json:"observedValue,omitempty" jsonschema:"The value this run actually observed."`
	ExpectedMin   string   `json:"expectedMin,omitempty" jsonschema:"Lower bound of the learned expected range the observed value was judged against."`
	ExpectedMax   string   `json:"expectedMax,omitempty" jsonschema:"Upper bound of the learned expected range the observed value was judged against."`
	Tolerance     string   `json:"tolerance,omitempty" jsonschema:"Sensitivity tier of the learned threshold (e.g. NARROW, NEUTRAL, WIDE)."`
	IsSuppressed  bool     `json:"isSuppressed,omitempty" jsonschema:"Whether the monitor is suppressed, meaning it is kept but not scored."`
	Dimensions    []string `json:"dimensions,omitempty" jsonschema:"Data quality dimensions this monitor contributes to — the quality category the check counts towards when the job is scored. Commonly Accuracy, Completeness, Consistency, Conformity, Integrity, Timeliness, Validity and Uniqueness, though an environment may define its own."`
}

// CustomMonitorResult is one custom monitor's (user-authored DQ rule's) outcome for
// this run. Its threshold is the rule's tolerance: how many breaking rows are allowed
// before the rule fails.
type CustomMonitorResult struct {
	MonitorName        string   `json:"monitorName"`
	State              string   `json:"state,omitempty" jsonschema:"Outcome of this monitor for this run. PASSING = the check held. BREAKING = it did not; this is a failure. EXCEPTION = the check could not run (see exception); treat as a failure to investigate, not a pass. LEARNING = an adaptive monitor still building its baseline, so it is not yet scored and is NOT a failure. SUPPRESSED = kept on the job but excluded from scoring, also not a failure. USER_PASSED = a human overrode a breaking result to passing. STALE = carried over from an earlier run because this run did not re-evaluate it. SKIPPED = not evaluated this run. Only BREAKING and EXCEPTION mean something is wrong."`
	Score              float64  `json:"score" jsonschema:"Quality score for this rule on this run, 0-100, where 100 is perfect and 0 is worst. The service deducts points per breaking row, so more failures means a lower score."`
	BreakingPercentage float64  `json:"breakingPercentage,omitempty" jsonschema:"Breaking rows as a percentage of rows evaluated."`
	RowsPassing        int64    `json:"rowsPassing,omitempty"`
	RowsBreaking       int64    `json:"rowsBreaking,omitempty" jsonschema:"The observed value for a custom rule: how many rows failed the rule this run."`
	RowsTotal          int64    `json:"rowsTotal,omitempty"`
	Tolerance          int      `json:"tolerance,omitempty" jsonschema:"The rule's threshold: count of breaking rows allowed before the rule is judged as failing."`
	Exception          string   `json:"exception,omitempty" jsonschema:"Failure message, only set when the monitor errored (state EXCEPTION)."`
	Dimensions         []string `json:"dimensions,omitempty" jsonschema:"Data quality dimensions this monitor contributes to — the quality category the check counts towards when the job is scored. Commonly Accuracy, Completeness, Consistency, Conformity, Integrity, Timeliness, Validity and Uniqueness, though an environment may define its own."`
}

// MonitorSummary counts the run's monitors so a caller can triage without walking
// both lists.
type MonitorSummary struct {
	Total     int `json:"total" jsonschema:"Total monitor results for the run, equal to adaptive + custom. Note that passing + breaking + exception does NOT generally add up to this: the remainder are monitors in the other states (LEARNING, SUPPRESSED, USER_PASSED, STALE, SKIPPED), which are counted in total but not broken out here. Do not infer a state count by subtraction."`
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
		Name:  "get_data_quality_job_run_monitors",
		Title: "Get Data Quality Job Run Monitors",
		Description: "Reads the per-monitor results of a single Collibra data-quality job run by its run_id. " +
			"A data-quality job (Collibra also calls it a \"dataset\") is a saved set of checks over ONE database table, and a " +
			"job run is one execution of it. A \"monitor\" is a single data-quality check on that table's data — one rule, with " +
			"one outcome per run.\n\n" +
			"Two kinds are returned. Adaptive monitors are the ones Collibra creates and maintains itself by learning a column's " +
			"normal behaviour (nulls, empties, uniqueness, smallest/largest/mean, row count, data type, schema change); they report " +
			"the observed value against a learned expected range and the sensitivity tier of that range. Custom monitors are the " +
			"job's user-authored rules; they report a score, breaking/passing row counts, and a tolerance — how many breaking rows " +
			"are allowed before the rule fails. A summary counts monitors by state so a failing run can be triaged at a glance.\n\n" +
			"Monitor results exist only for runs that produced them, so a run that never completed returns an error saying so.\n\n" +
			"Prerequisite: run_id comes from dq_search_job_runs, which lists a job's runs with their ids and status. " +
			"You cannot derive it from a job name.\n\n" +
			"Not to be confused with two tools taking the same run_id: dq_get_job_run returns this same monitor breakdown plus the " +
			"run's lifecycle details (status, timing, overall score) — prefer this tool when only the monitors are wanted or when " +
			"tolerances are needed. get_data_quality_job_run_profile describes the shape of the data the run saw (per-column counts " +
			"and distributions) rather than which checks passed.\n\n" +
			"Read-only: it never changes a job, a run, a monitor or any data. Requires the Data Quality Job > View permission on the " +
			"job the run belongs to; without it the call returns a permission error.\n\n" +
			"Example user requests: \"Which monitors failed in DQ run <id>?\"; \"Show me the monitor results for run <id>\"; " +
			"\"What was the expected range for the monitor that broke in run <id>?\"; \"Why did this quality run fail?\"; " +
			"\"Is that run healthy?\"",
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
			Exception: truncateMessage(m.Exception), Dimensions: m.Dimensions,
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

// maxErrMessage caps how much downstream text is forwarded to the model.
const maxErrMessage = 200

// truncateMessage bounds a message from the DQ engine. Engine and JDBC failures
// routinely echo the offending cell value ("invalid input syntax for integer:
// ..."), so an unbounded pass-through is a channel for customer data rather than
// metadata. Truncating keeps the leading text, where the error class lives.
// An empty message stays empty - a healthy monitor must not gain an exception.
func truncateMessage(msg string) string {
	trimmed := strings.TrimSpace(msg)
	if len(trimmed) > maxErrMessage {
		return trimmed[:maxErrMessage] + "… (truncated)"
	}
	return trimmed
}

// safeErr renders a downstream error for the model under the same cap.
func safeErr(err error) string {
	if err == nil {
		return "no additional detail"
	}
	if msg := truncateMessage(err.Error()); msg != "" {
		return msg
	}
	return "no additional detail"
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
		out.Message = fmt.Sprintf("The data-quality API rejected the monitor lookup for run %q (HTTP 400): %s", runID, safeErr(err))
		out.Guidance = "Check that run_id is correct and well-formed, then retry."
	case http.StatusUnprocessableEntity:
		out.Message = fmt.Sprintf("The data-quality API could not process the monitor lookup for run %q (HTTP 422): %s", runID, safeErr(err))
		out.Guidance = "The request was well-formed but rejected as invalid — most often a run that exists but has no monitor results. Correct the inputs; retrying unchanged will fail identically."
	case http.StatusOK:
		// The client returns code 200 with an error when the body fails to parse.
		// Printing "(HTTP 200)" tells the agent nothing it can act on.
		out.Message = fmt.Sprintf("The data-quality API returned monitor results for run %q that could not be read: %s", runID, safeErr(err))
		out.Guidance = "The response was not in the expected format. This is a service-side problem — retrying is unlikely to help; contact your Collibra administrator if it persists."
	case 0:
		out.Message = fmt.Sprintf("Failed to read the monitor results for run %q: %s", runID, safeErr(err))
		out.Guidance = "A network/transport error occurred contacting the data-quality API. Retry."
	default:
		out.Message = fmt.Sprintf("Failed to read the monitor results for run %q (HTTP %d): %s", runID, code, safeErr(err))
		out.Guidance = "This is likely a server-side error. Retry shortly; if it persists, contact your Collibra administrator."
	}
	return out
}
