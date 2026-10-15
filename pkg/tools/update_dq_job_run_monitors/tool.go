// Package update_dq_job_run_monitors implements the update_data_quality_job_run_monitors MCP
// tool — suppress one or more monitor results of a Collibra data-quality job run in a single
// bulk request.
//
// The flow is two public calls around a confirm checkpoint:
//
//	GET /rest/dq/1.0/jobRuns/{jobRunId}/monitors -> resolve every requested monitor on the run
//	                                                (preview, and again as the read-back)
//	PUT /rest/dq/1.0/jobRuns/{jobRunId}/monitors -> apply all operations in one request
//
// The PUT is all-or-nothing on the service side: every operation is validated before anything is
// written, and the writes share one transaction. A batch therefore never partially applies, so the
// tool resolves the monitors up front — an unknown run or monitor is reported by name before the
// write rather than as the service's single opaque 404 — and reads the run back afterwards to
// confirm each monitor's resulting state.
//
// The public API also offers PASS (adaptive monitors only). It is out of this tool's scope and
// rejected during validation, before any call.
package update_dq_job_run_monitors

import (
	"context"
	"fmt"
	"net/http"
	"regexp"
	"strings"

	"github.com/collibra/chip/pkg/chip"
	"github.com/collibra/chip/pkg/clients"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Status is the overall outcome of a call.
type Status string

const (
	// StatusPreview means nothing was written: the confirm checkpoint is closed.
	StatusPreview Status = "preview"
	// StatusSuccess means the bulk update was applied.
	StatusSuccess Status = "success"
	// StatusNeedsInput means the inputs failed validation before any call.
	StatusNeedsInput Status = "needs_input"
	// StatusError means the run or a monitor could not be resolved, or the update was rejected.
	StatusError Status = "error"
)

// ActionSuppress is the only action this tool applies.
const ActionSuppress = "SUPPRESS"

const (
	kindAdaptive     = "adaptive"
	kindCustom       = "custom"
	stateSuppressed  = "SUPPRESSED"
	maxOperations    = 500
	maxMonitorName   = 255
	maxListedNames   = 50
	maxErrMessage    = 200
	monitorNameRegex = `^[a-zA-Z0-9_-]+( [a-zA-Z0-9_-]+)*$`
)

// monitorNamePattern is the public API's own constraint on monitorName, enforced here so a
// malformed name is caught before any call rather than as a downstream 400.
var monitorNamePattern = regexp.MustCompile(monitorNameRegex)

// Input is the tool's typed input.
type Input struct {
	RunID      string      `json:"run_id" jsonschema:"Required. The id (jobRunId) of the job run whose monitor results to update, e.g. '9c1d1e3a-2b4f-4a6c-8d90-111213141516'. Comes from dq_search_job_runs."`
	Operations []Operation `json:"operations" jsonschema:"Required. The operations to apply, 1 to 500, all on this one run and all sent in a single request. Each monitor may appear only once."`
	Confirm    bool        `json:"confirm,omitempty" jsonschema:"Safety checkpoint. false (default) resolves every monitor on the run and returns a PREVIEW of exactly what would change, writing NOTHING; set true to apply the operations after the user has approved."`
}

// Operation is one monitor override requested by the caller.
type Operation struct {
	MonitorName string `json:"monitor_name" jsonschema:"Required. Name of the monitor (a single data-quality check on the table) exactly as get_data_quality_job_run_monitors reports it for this run, e.g. 'ROW_COUNT', 'amount__NULL' or 'amount_is_positive'. Case-sensitive; letters, digits, '_' and '-', with single spaces between words; at most 255 characters."`
	Action      string `json:"action" jsonschema:"Required. The override to apply. Only SUPPRESS is accepted: the monitor result is excluded from the run's quality score (state SUPPRESSED), and an adaptive monitor is also retrained so its learned expected range absorbs this run's behaviour."`
}

// OperationResult describes one operation, previewed or applied.
type OperationResult struct {
	MonitorName string `json:"monitorName" jsonschema:"The monitor the operation targets."`
	MonitorKind string `json:"monitorKind" jsonschema:"'adaptive' for a monitor Collibra learns and maintains itself; 'custom' for a user-authored data-quality rule. Suppressing an adaptive monitor also retrains it; a custom one is not retrained."`
	Action      string `json:"action" jsonschema:"The operation sent for this monitor. Always SUPPRESS."`
	StateBefore string `json:"stateBefore,omitempty" jsonschema:"The monitor's state on this run before the update, e.g. BREAKING or PASSING. SUPPRESSED here means it was already suppressed and the operation changes nothing visible."`
	StateAfter  string `json:"stateAfter,omitempty" jsonschema:"The monitor's state read back after the update. Absent in a preview, and absent when the read-back failed — which does not undo the update."`
	Applied     bool   `json:"applied" jsonschema:"True once the service accepted the bulk request containing this operation; false in a preview or when the request was rejected. The service applies a batch whole or not at all, so this is the same for every operation in one call."`
	Verified    bool   `json:"verified" jsonschema:"True when the read-back shows the monitor suppressed. false with applied=true means the service accepted the update but the read-back did not reflect it — re-check with get_data_quality_job_run_monitors."`
}

// Output is the typed response.
type Output struct {
	Status     Status            `json:"status" jsonschema:"'preview' when confirm was false and nothing was written; 'success' when the update was applied; 'needs_input' for inputs that failed validation (nothing was called); 'error' when the run or a monitor was not found or the service rejected the update (nothing was applied)."`
	Message    string            `json:"message" jsonschema:"Human-readable summary of what would happen, or what happened."`
	JobRunID   string            `json:"jobRunId,omitempty" jsonschema:"The run the operations target."`
	Operations []OperationResult `json:"operations,omitempty" jsonschema:"One entry per requested operation, in request order: what would change in a preview, what changed on success."`
	NotFound   []string          `json:"notFound,omitempty" jsonschema:"Requested monitor names that do not exist on this run. When present, nothing was written."`
	Guidance   string            `json:"guidance,omitempty" jsonschema:"What to do next: on preview, how to confirm; on needs_input/error, how to correct the call."`
}

// NewTool returns the registered tool.
func NewTool(collibraClient *http.Client) *chip.Tool[Input, Output] {
	return &chip.Tool[Input, Output]{
		Name:  "update_data_quality_job_run_monitors",
		Title: "Update Data Quality Job Run Monitors",
		Description: "Suppresses one or more monitor results of a single Collibra data-quality job run, in one bulk request. " +
			"A data-quality job (Collibra also calls it a \"dataset\") is a saved set of checks over ONE database table, and a " +
			"job run is one execution of it. A \"monitor\" is a single data-quality check on that table's data, with one " +
			"result per run. Suppressing a monitor's result overrides it for that run: its state becomes SUPPRESSED and it no " +
			"longer counts towards the run's quality score. Use it to silence a known, accepted failure — a breaking monitor " +
			"the user has judged to be a false alarm or an expected change — without editing the monitor itself.\n\n" +
			"Each operation names a monitor and an action; SUPPRESS is the only action. Suppressing an adaptive monitor (one " +
			"Collibra learns and maintains itself) also retrains it, so its learned expected range absorbs this run's " +
			"behaviour; suppressing a custom monitor (a user-authored data-quality rule) does not retrain anything. Up to 500 " +
			"operations, each monitor at most once.\n\n" +
			"Prerequisites: run_id comes from dq_search_job_runs, and monitor names come from " +
			"get_data_quality_job_run_monitors for that same run — names must match exactly.\n\n" +
			"All-or-nothing: the service applies the whole batch or none of it. Before writing, the tool checks that the run " +
			"exists and that every named monitor is on it, and reports any it cannot find by name. After writing, it reads the " +
			"run back and returns each monitor's state before and after.\n\n" +
			"Confirm checkpoint: confirm=false (the default) resolves the monitors and returns a preview of exactly what would " +
			"change, writing nothing. Call again with confirm=true to apply. The tool never asks the user anything itself; " +
			"getting the user's approval between the two calls is the caller's job.\n\n" +
			"Not to be confused with: get_data_quality_job_run_monitors, which only reads monitor results; " +
			"dq_update_job, which changes a job's configuration (including which adaptive monitors it runs) for all future " +
			"runs rather than overriding one run's results; and create_data_quality_rule, which defines new monitors.\n\n" +
			"Writes: it changes the run's monitor states and score. Requires the Data Quality Job > Edit permission on the " +
			"job the run belongs to.\n\n" +
			"Example user requests: \"Suppress the ROW_COUNT monitor on run <id>\"; \"Suppress every breaking monitor in " +
			"that run\"; \"That null-check failure is expected, silence it\"; \"Mute these alerts for last night's run\"; " +
			"\"Ignore the failing checks on this run\"",
		Handler:     handler(collibraClient),
		Permissions: []string{},
		// Destructive: it overwrites existing monitor states and retrains adaptive
		// baselines, neither of which this tool can undo. Not idempotent: a repeat
		// call retrains adaptive monitors again.
		Annotations: &mcp.ToolAnnotations{DestructiveHint: chip.Ptr(true), OpenWorldHint: chip.Ptr(false)},
	}
}

func handler(collibraClient *http.Client) chip.ToolHandlerFunc[Input, Output] {
	return func(ctx context.Context, input Input) (Output, error) {
		runID := strings.TrimSpace(input.RunID)
		operations, invalid := validate(runID, input.Operations)
		if invalid != nil {
			return *invalid, nil
		}

		before, code, err := clients.GetDqJobRunMonitors(ctx, collibraClient, runID)
		if err != nil {
			return lookupError(code, err, runID), nil
		}
		resolved, notFound := resolve(operations, before)
		if len(notFound) > 0 {
			return monitorsNotFound(runID, notFound, before), nil
		}

		if !input.Confirm {
			return preview(runID, resolved), nil
		}

		code, err = clients.UpdateDqJobRunMonitors(ctx, collibraClient, runID, requestOperations(resolved))
		if err != nil {
			return updateError(code, err, runID), nil
		}
		return applied(ctx, collibraClient, runID, resolved), nil
	}
}

// validate enforces every documented input constraint before any network call and
// returns the operations with names trimmed and actions upper-cased.
func validate(runID string, operations []Operation) ([]Operation, *Output) {
	if runID == "" {
		return nil, needsInput("run_id is required.",
			"Supply run_id — the id (jobRunId) of the job run whose monitors to suppress. dq_search_job_runs lists a job's runs with their ids.")
	}
	if len(operations) == 0 {
		return nil, needsInput("operations is required and must contain at least one operation.",
			"Supply operations as a list of {monitor_name, action} objects, e.g. [{\"monitor_name\": \"ROW_COUNT\", \"action\": \"SUPPRESS\"}]. "+
				"get_data_quality_job_run_monitors lists the monitor names on the run.")
	}
	if len(operations) > maxOperations {
		return nil, needsInput(fmt.Sprintf("operations has %d entries; at most %d are allowed in one request.", len(operations), maxOperations),
			fmt.Sprintf("Split the operations into batches of %d or fewer and call once per batch.", maxOperations))
	}

	normalized := make([]Operation, 0, len(operations))
	seen := make(map[string]int, len(operations))
	for i, op := range operations {
		name := strings.TrimSpace(op.MonitorName)
		action := strings.ToUpper(strings.TrimSpace(op.Action))
		if problem := validateOperation(i, name, action); problem != nil {
			return nil, problem
		}
		if first, dup := seen[name]; dup {
			return nil, needsInput(fmt.Sprintf("operations[%d] repeats monitor_name %q, already given at operations[%d].", i, name, first),
				"Each monitor may appear only once per request. Remove the duplicate.")
		}
		seen[name] = i
		normalized = append(normalized, Operation{MonitorName: name, Action: action})
	}
	return normalized, nil
}

func validateOperation(i int, name, action string) *Output {
	switch {
	case name == "":
		return needsInput(fmt.Sprintf("operations[%d].monitor_name is required.", i),
			"Use the monitor's name exactly as get_data_quality_job_run_monitors reports it for this run.")
	case len(name) > maxMonitorName:
		return needsInput(fmt.Sprintf("operations[%d].monitor_name is %d characters; at most %d are allowed.", i, len(name), maxMonitorName),
			"Use the monitor's name exactly as get_data_quality_job_run_monitors reports it for this run.")
	case !monitorNamePattern.MatchString(name):
		return needsInput(fmt.Sprintf("operations[%d].monitor_name %q is not a valid monitor name.", i, name),
			"Monitor names contain only letters, digits, '_' and '-', with single spaces between words (e.g. 'ROW_COUNT', "+
				"'amount__NULL'). Copy the name exactly as get_data_quality_job_run_monitors reports it.")
	case action == "":
		return needsInput(fmt.Sprintf("operations[%d].action is required.", i),
			fmt.Sprintf("Set action to %q — the only action this tool applies.", ActionSuppress))
	case action != ActionSuppress:
		return needsInput(fmt.Sprintf("operations[%d].action %q is not supported. Use %q.", i, action, ActionSuppress),
			fmt.Sprintf("%q is the only action this tool applies. Nothing was sent.", ActionSuppress))
	}
	return nil
}

func needsInput(message, guidance string) *Output {
	return &Output{Status: StatusNeedsInput, Message: message, Guidance: guidance}
}

// resolve matches each operation to the run's monitor of that exact name. The
// public GET renders names in the same form the PUT accepts, so an exact match
// here is what the service will find.
func resolve(operations []Operation, run *clients.DqJobRunMonitorsResult) ([]OperationResult, []string) {
	index := monitorIndex(run)
	resolved := make([]OperationResult, 0, len(operations))
	var notFound []string
	for _, op := range operations {
		m, ok := index[op.MonitorName]
		if !ok {
			notFound = append(notFound, op.MonitorName)
			continue
		}
		resolved = append(resolved, OperationResult{
			MonitorName: op.MonitorName,
			MonitorKind: m.kind,
			Action:      op.Action,
			StateBefore: m.state,
		})
	}
	return resolved, notFound
}

type runMonitor struct {
	kind  string
	state string
}

func monitorIndex(run *clients.DqJobRunMonitorsResult) map[string]runMonitor {
	index := make(map[string]runMonitor, len(run.AdaptiveMonitors)+len(run.CustomMonitors))
	for _, m := range run.AdaptiveMonitors {
		index[m.MonitorName] = runMonitor{kind: kindAdaptive, state: adaptiveState(m)}
	}
	for _, m := range run.CustomMonitors {
		index[m.MonitorName] = runMonitor{kind: kindCustom, state: m.State}
	}
	return index
}

// adaptiveState reports SUPPRESSED for an adaptive monitor flagged isSuppressed,
// whatever its state field says, so a suppression is visible either way.
func adaptiveState(m clients.DqAdaptiveMonitorResult) string {
	if m.IsSuppressed {
		return stateSuppressed
	}
	return m.State
}

func monitorsNotFound(runID string, notFound []string, run *clients.DqJobRunMonitorsResult) Output {
	return Output{
		Status:   StatusError,
		Message:  fmt.Sprintf("%d monitor(s) not found on run %q: %s. Nothing was changed.", len(notFound), runID, strings.Join(notFound, ", ")),
		JobRunID: runID,
		NotFound: notFound,
		Guidance: availableMonitorsGuidance(run),
	}
}

func availableMonitorsGuidance(run *clients.DqJobRunMonitorsResult) string {
	names := make([]string, 0, len(run.AdaptiveMonitors)+len(run.CustomMonitors))
	for _, m := range run.AdaptiveMonitors {
		names = append(names, m.MonitorName)
	}
	for _, m := range run.CustomMonitors {
		names = append(names, m.MonitorName)
	}
	if len(names) == 0 {
		return "This run has no monitor results at all, so there is nothing to suppress. Check the run finished with dq_get_job_run."
	}
	listed := names
	suffix := ""
	if len(names) > maxListedNames {
		listed = names[:maxListedNames]
		suffix = fmt.Sprintf(" … and %d more (see get_data_quality_job_run_monitors)", len(names)-maxListedNames)
	}
	return fmt.Sprintf("Names are case-sensitive and must match exactly. Monitors on this run: %s%s. "+
		"Correct the names and retry; the whole batch is sent only once every name resolves.", strings.Join(listed, ", "), suffix)
}

func preview(runID string, resolved []OperationResult) Output {
	return Output{
		Status:     StatusPreview,
		Message:    fmt.Sprintf("Would suppress %d monitor(s) on run %q. Nothing has changed yet.", len(resolved), runID),
		JobRunID:   runID,
		Operations: resolved,
		Guidance: "Suppressed monitors stop counting towards this run's quality score; adaptive monitors are also retrained on " +
			"this run, which shifts their learned expected range and cannot be reversed by this tool. Review the list with the " +
			"user, then call again with the same inputs and confirm=true to apply.",
	}
}

func requestOperations(resolved []OperationResult) []clients.DqMonitorOperation {
	ops := make([]clients.DqMonitorOperation, 0, len(resolved))
	for _, r := range resolved {
		ops = append(ops, clients.DqMonitorOperation{MonitorName: r.MonitorName, Operation: clients.DqMonitorOperationSuppress})
	}
	return ops
}

// applied reports a completed update. The read-back is best-effort: the write has
// already committed, so a failure to re-read must not be reported as though the
// update failed.
func applied(ctx context.Context, client *http.Client, runID string, resolved []OperationResult) Output {
	for i := range resolved {
		resolved[i].Applied = true
	}
	message := fmt.Sprintf("Suppressed %d monitor(s) on run %q.", len(resolved), runID)

	after, _, err := clients.GetDqJobRunMonitors(ctx, client, runID)
	if err != nil {
		return Output{
			Status:     StatusSuccess,
			Message:    message,
			JobRunID:   runID,
			Operations: resolved,
			Guidance: "The update was accepted. Reading the run back afterwards failed, so the resulting states are not " +
				"included — use get_data_quality_job_run_monitors to see them. This does not affect the update.",
		}
	}

	index := monitorIndex(after)
	var unverified []string
	for i := range resolved {
		m, ok := index[resolved[i].MonitorName]
		if ok {
			resolved[i].StateAfter = m.state
		}
		resolved[i].Verified = ok && m.state == stateSuppressed
		if !resolved[i].Verified {
			unverified = append(unverified, resolved[i].MonitorName)
		}
	}

	out := Output{Status: StatusSuccess, Message: message, JobRunID: runID, Operations: resolved}
	if len(unverified) > 0 {
		out.Guidance = fmt.Sprintf("The service accepted the update, but the read-back does not show these monitor(s) as "+
			"SUPPRESSED: %s. Re-check them with get_data_quality_job_run_monitors before retrying.", strings.Join(unverified, ", "))
	}
	return out
}

// lookupError maps a failure of the up-front monitor read. Nothing has been
// written at this point.
func lookupError(code int, err error, runID string) Output {
	out := Output{Status: StatusError, JobRunID: runID}
	switch code {
	case http.StatusNotFound:
		out.Message = fmt.Sprintf("No job run with id %q was found (HTTP 404). Nothing was changed.", runID)
		out.Guidance = "Verify the run_id — it may be wrong, or the run has since been deleted. dq_search_job_runs lists a job's runs with their ids."
	case http.StatusUnauthorized:
		out.Message = "Not authenticated to the data-quality API (HTTP 401). Nothing was changed."
		out.Guidance = "Your Collibra session/token is missing or expired — re-authenticate and retry."
	case http.StatusForbidden:
		out.Message = fmt.Sprintf("You do not have permission to view the monitor results for run %q (HTTP 403). Nothing was changed.", runID)
		out.Guidance = "Suppressing needs the Data Quality Job > Edit permission on the job the run belongs to. Ask an administrator for it, then retry."
	case http.StatusBadRequest:
		out.Message = fmt.Sprintf("The data-quality API rejected the monitor lookup for run %q (HTTP 400): %s", runID, safeErr(err))
		out.Guidance = "Check that run_id is correct and well-formed, then retry."
	case http.StatusUnprocessableEntity:
		out.Message = fmt.Sprintf("The data-quality API could not process the monitor lookup for run %q (HTTP 422): %s", runID, safeErr(err))
		out.Guidance = "The request was well-formed but rejected as invalid. Correct the inputs; retrying unchanged will fail identically."
	case http.StatusOK:
		// The client returns code 200 with an error when the body fails to parse.
		out.Message = fmt.Sprintf("The data-quality API returned monitor results for run %q that could not be read: %s", runID, safeErr(err))
		out.Guidance = "The response was not in the expected format. This is a service-side problem — retrying is unlikely to help; contact your Collibra administrator if it persists."
	case 0:
		out.Message = fmt.Sprintf("Failed to read the monitor results for run %q: %s", runID, safeErr(err))
		out.Guidance = "A network/transport error occurred contacting the data-quality API. Nothing was changed. Retry."
	default:
		out.Message = fmt.Sprintf("Failed to read the monitor results for run %q (HTTP %d): %s", runID, code, safeErr(err))
		out.Guidance = "This is likely a server-side error. Nothing was changed. Retry shortly; if it persists, contact your Collibra administrator."
	}
	return out
}

// updateError maps a rejected bulk update. The service validates the whole batch
// before writing, so a 4xx means none of it was applied.
func updateError(code int, err error, runID string) Output {
	out := Output{Status: StatusError, JobRunID: runID}
	switch code {
	case http.StatusNotFound:
		out.Message = fmt.Sprintf("The run or one of the monitors was not found when applying the update to run %q (HTTP 404): %s. Nothing was changed.", runID, safeErr(err))
		out.Guidance = "The run or a monitor changed between the preview and the update. Re-run with confirm=false to resolve the monitors again."
	case http.StatusUnauthorized:
		out.Message = "Not authenticated to the data-quality API (HTTP 401). Nothing was changed."
		out.Guidance = "Your Collibra session/token is missing or expired — re-authenticate and retry."
	case http.StatusForbidden:
		out.Message = fmt.Sprintf("You do not have permission to update the monitors of run %q (HTTP 403). Nothing was changed.", runID)
		out.Guidance = "Suppressing needs the Data Quality Job > Edit permission on the job the run belongs to. Ask an administrator for it, then retry."
	case http.StatusBadRequest:
		out.Message = fmt.Sprintf("The data-quality API rejected the update to run %q (HTTP 400): %s. Nothing was changed.", runID, safeErr(err))
		out.Guidance = "The batch is rejected whole when any operation is invalid. Correct the operation the message names and retry."
	case http.StatusUnprocessableEntity:
		out.Message = fmt.Sprintf("The data-quality API could not process the update to run %q (HTTP 422): %s. Nothing was changed.", runID, safeErr(err))
		out.Guidance = "The request was well-formed but rejected as invalid. Correct the inputs; retrying unchanged will fail identically."
	default:
		out.Message = fmt.Sprintf("Failed to apply the update to run %q: %s", runID, statusText(code, err))
		out.Guidance = "This is a server-side or transport error, so it is not known whether the update was applied. Check the " +
			"monitors with get_data_quality_job_run_monitors before retrying."
	}
	return out
}

func statusText(code int, err error) string {
	if code == 0 {
		return safeErr(err)
	}
	return fmt.Sprintf("(HTTP %d) %s", code, safeErr(err))
}

// safeErr renders a downstream error for the model under a cap. The DQ client
// wraps the whole non-2xx body into the error and engine failures echo the
// offending value, so an unbounded pass-through is a channel for customer data.
func safeErr(err error) string {
	if err == nil {
		return "no additional detail"
	}
	msg := strings.TrimSpace(err.Error())
	if msg == "" {
		return "no additional detail"
	}
	if len(msg) > maxErrMessage {
		return msg[:maxErrMessage] + "… (truncated)"
	}
	return msg
}
