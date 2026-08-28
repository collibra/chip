// Package run_dq_job implements the dq_run_job MCP tool — trigger a run of an existing Collibra
// data-quality job as-is, with no changes to its definition.
//
// The flow resolves the job, then converges on the public run behind a confirm checkpoint:
//
//	Resolve : GET  /rest/dq/1.0/jobs/{jobName} -> exact match wins.
//	          On 404, GET /rest/dq/1.0/jobs?jobName=<name> (searchJobs, %LIKE% match) -> zero
//	          matches: not-found error; exactly one: use it; several: return the candidate names
//	          (needs_input) so the caller can pick one and re-call with the exact name.
//	Preview : confirm=false (the default) returns the composed run plan and queues NOTHING.
//	Run     : confirm=true POSTs /rest/dq/1.0/jobs/{jobName}/run -> queues a run, returns jobRunId.
//	          runDate/runDateEnd/backrun are optional; omitted means "use the job's current
//	          settings, run now" (the "no edits" case).
//
// The confirm checkpoint is required by docs/TOOL_CONTRIBUTION_STANDARDS.md §5: it applies to any
// tool that writes to Collibra or a downstream system, and queuing a run writes — it consumes
// compute on the job's edge site, and a backrun can queue many runs at once. Permissions are
// enforced by the DQ service; 400/401/403/404/500 and transport failures are surfaced as messages
// with actionable guidance rather than Go errors.
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

type Status string

const (
	StatusPreview    Status = "preview"
	StatusRunning    Status = "running"
	StatusNeedsInput Status = "needs_input"
	StatusError      Status = "error"
)

type Input struct {
	JobName string `json:"jobName" jsonschema:"Required. Name of the existing data-quality job to run (a job, also called a 'dataset', is a saved data-quality check on ONE database table that scans the table and runs its rules), e.g. 'PUBLIC.SAMPLE_DATASET'. An exact match is tried first; if none is found, jobs whose name contains this text are offered as candidates."`

	Confirm bool `json:"confirm,omitempty" jsonschema:"Safety checkpoint. false (the default) is READ-ONLY: it returns a PREVIEW of the run that would be queued and queues NOTHING — review it with the user. true queues the run. Always preview first unless the user has already approved this exact run."`

	// --- Optional run-window override. Omit all of these to run the job with its current settings. ---
	RunDateKind     string `json:"runDateKind,omitempty" jsonschema:"Optional. Format of runDateValue: 'DATE' (yyyy-MM-dd) or 'TIMESTAMP' (RFC3339). Required only when runDateValue is set; supply both or neither."`
	RunDateValue    string `json:"runDateValue,omitempty" jsonschema:"Optional. Overrides the start of the run's time slice (the job's ${rd} variable), e.g. '2025-10-22' with runDateKind='DATE'. Default: omit to use the job's current run date (now), matching a normal scheduled run."`
	RunDateEndKind  string `json:"runDateEndKind,omitempty" jsonschema:"Optional. Format of runDateEndValue: 'DATE' (yyyy-MM-dd) or 'TIMESTAMP' (RFC3339). Required only when runDateEndValue is set; supply both or neither."`
	RunDateEndValue string `json:"runDateEndValue,omitempty" jsonschema:"Optional. Overrides the exclusive end of the run's time slice (the job's ${rdEnd} variable), e.g. '2025-10-23' with runDateEndKind='DATE'. Default: omit to use the job's current run-date end. Set this together with runDateValue to run over a date RANGE rather than a single date."`
	BackrunTimeBin  string `json:"backrunTimeBin,omitempty" jsonschema:"Optional. Size of each backfill period: 'DAY', 'MONTH' or 'YEAR'. Set together with backrunBinValue to also queue runs for PRIOR periods. Default: omit for no backfill."`
	BackrunBinValue int    `json:"backrunBinValue,omitempty" jsonschema:"Optional. How many prior periods to backfill, e.g. 10 with backrunTimeBin='DAY' queues the last 10 days in addition to the current run. Must be 1 or greater. Set together with backrunTimeBin. Default: omit for no backfill."`
}

// RunPlan echoes every field that will be sent on the run, so a preview shows exactly what
// confirm=true would queue (TOOL_CONTRIBUTION_STANDARDS.md §5.2).
type RunPlan struct {
	JobName         string `json:"jobName" jsonschema:"The exact name of the job that will be run."`
	RunDate         string `json:"runDate,omitempty" jsonschema:"The run-date override that will be sent, as 'value (KIND)'. Absent when the job's current run date is used."`
	RunDateEnd      string `json:"runDateEnd,omitempty" jsonschema:"The run-date-end override that will be sent, as 'value (KIND)'. Absent when the job's current run-date end is used."`
	Backrun         string `json:"backrun,omitempty" jsonschema:"The backfill that will be queued, e.g. '10 prior DAY period(s)'. Absent when no backfill was requested."`
	UsesJobDefaults bool   `json:"usesJobDefaults" jsonschema:"true when no overrides were supplied and the job runs exactly as scheduled would run it."`
}

type Output struct {
	Status            Status   `json:"status" jsonschema:"'preview' when confirm was false and nothing was queued; 'running' when the run was queued; 'needs_input' when the job name matched none or several jobs, or inputs were incomplete; 'error' for downstream DQ failures."`
	Message           string   `json:"message" jsonschema:"Human-readable outcome and what to do next."`
	JobName           string   `json:"jobName,omitempty" jsonschema:"The job that was previewed or run."`
	RunPlan           *RunPlan `json:"runPlan,omitempty" jsonschema:"On preview and on success: exactly what was (or would be) submitted."`
	JobRunID          string   `json:"jobRunId,omitempty" jsonschema:"The id of the newly queued run, returned only when confirm was true. Use it with dq_get_job_run to check status, or dq_cancel_job_run to stop the run."`
	JobDetailsLink    string   `json:"jobDetailsLink,omitempty" jsonschema:"Job Details deep-link path (relative to the Collibra instance URL)."`
	CandidateJobNames []string `json:"candidateJobNames,omitempty" jsonschema:"When the job name matched several jobs: exact names to pick from — re-call with one of these."`
	Guidance          string   `json:"guidance,omitempty" jsonschema:"On preview/needs_input/error, what to do next."`
}

func NewTool(collibraClient *http.Client) *chip.Tool[Input, Output] {
	return &chip.Tool[Input, Output]{
		Name:  "dq_run_job",
		Title: "Run Data Quality Job",
		Description: "Triggers a run of an existing Collibra data-quality job (a job, also called a 'dataset', is " +
			"a saved data-quality check on ONE database table that scans the table and runs its rules) AS-IS, " +
			"with no changes to the job's definition. Identify the job by name: an exact match is tried first, " +
			"and if none is found, jobs whose name contains the given text are returned as candidates " +
			"(status=needs_input) so you can pick the exact one and re-call.\n\n" +
			"The job must already exist — this tool does not create one. By default the run uses the job's " +
			"current settings (current run date, no backfill), which is the plain \"just run it\" case. " +
			"Optionally override the run's time slice with runDateKind/runDateValue and " +
			"runDateEndKind/runDateEndValue ('DATE' is yyyy-MM-dd, 'TIMESTAMP' is RFC3339), or additionally " +
			"queue runs for prior periods with backrunTimeBin ('DAY'/'MONTH'/'YEAR') plus backrunBinValue.\n\n" +
			"This WRITES to Collibra: it queues a new run, which consumes compute on the job's edge site, and a " +
			"backfill can queue many runs at once. It is therefore built around a confirm checkpoint: " +
			"confirm=false (the default) returns a PREVIEW of exactly what would be queued and queues nothing — " +
			"review it with the user; confirm=true queues the run and returns its jobRunId.\n\n" +
			"To check on the run afterwards, read its status and score with dq_get_job_run, or stop it with " +
			"dq_cancel_job_run — both take the returned jobRunId. To CHANGE the job's definition (its query, " +
			"schedule or monitors) rather than just run it, use dq_update_job instead; to read the job's " +
			"configuration without running it, use dq_get_job.\n\n" +
			"Note: requires the Data Quality Admin or Data Quality User global role. The data-quality service " +
			"rejects the run with HTTP 403 if the user lacks the DATA_QUALITY_JOB_RUN permission on the job.\n\n" +
			"Example user requests: \"Run the data quality job sales.orders now\"; \"Kick off a DQ run for my " +
			"customers table\"; \"Trigger public.transactions without changing anything\"; \"Re-run the orders " +
			"quality check for last Tuesday\"; \"Backfill data quality for the past 10 days on public.nyse.\"",
		Handler:     handler(collibraClient),
		Permissions: []string{},
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint:    false,
			DestructiveHint: chip.Ptr(false),
			IdempotentHint:  false,
			OpenWorldHint:   chip.Ptr(false),
		},
	}
}

func handler(collibraClient *http.Client) chip.ToolHandlerFunc[Input, Output] {
	return func(ctx context.Context, input Input) (Output, error) {
		jobName := strings.TrimSpace(input.JobName)
		if jobName == "" {
			return Output{
				Status:   StatusNeedsInput,
				Message:  "Provide the job to run.",
				Guidance: "Supply jobName — the name of the data-quality job to run.",
			}, nil
		}
		if !clients.IsValidDqJobName(jobName) {
			return Output{
				Status:   StatusNeedsInput,
				JobName:  jobName,
				Message:  fmt.Sprintf("%q is not a valid data-quality job name.", jobName),
				Guidance: "Job names contain only letters, digits, '.', '-' and '_', and cannot start with '-'. Check the name and retry.",
			}, nil
		}

		// Validate the run window before any network call, so a preventable mistake never surfaces
		// as a downstream 400.
		runRequest, needsInput := buildRunRequest(input)
		if needsInput != nil {
			needsInput.JobName = jobName
			return *needsInput, nil
		}

		resolved, resolveOut := resolveJobName(ctx, collibraClient, jobName)
		if resolveOut != nil {
			return *resolveOut, nil
		}

		plan := runPlan(resolved, input)

		// Confirm checkpoint: queue nothing until the caller has seen what would be queued.
		if !input.Confirm {
			return Output{
				Status:         StatusPreview,
				JobName:        resolved,
				RunPlan:        plan,
				JobDetailsLink: clients.DqJobDetailsPath(resolved),
				Message:        previewMessage(resolved, plan),
				Guidance:       "Nothing has been queued. Review the runPlan with the user, then re-call with confirm=true to start the run.",
			}, nil
		}

		submission, code, err := clients.RunDqJob(ctx, collibraClient, resolved, runRequest)
		if err != nil {
			return runError(code, err, resolved), nil
		}

		return Output{
			Status:         StatusRunning,
			JobName:        resolved,
			RunPlan:        plan,
			JobRunID:       submission.JobRunID,
			JobDetailsLink: clients.DqJobDetailsPath(resolved),
			Message:        fmt.Sprintf("Run %q of job %q has been queued.", submission.JobRunID, resolved),
			Guidance:       "Check progress with dq_get_job_run using this jobRunId, or stop the run with dq_cancel_job_run.",
		}, nil
	}
}

// resolveJobName converges a caller-supplied name on exactly one existing job: exact match first,
// then a fuzzy name search on 404. It returns either the resolved name or the Output to return.
func resolveJobName(ctx context.Context, collibraClient *http.Client, jobName string) (string, *Output) {
	_, code, err := clients.GetDqJob(ctx, collibraClient, jobName)
	if err == nil {
		return jobName, nil
	}
	if code != http.StatusNotFound {
		out := lookupError(code, err, jobName)
		return "", &out
	}

	// No exact match — fall back to a fuzzy name search to disambiguate or report not-found.
	names, searchErr := clients.SearchDqJobNames(ctx, collibraClient, jobName)
	if searchErr != nil {
		out := lookupError(0, searchErr, jobName)
		return "", &out
	}
	switch len(names) {
	case 0:
		return "", &Output{
			Status:   StatusError,
			JobName:  jobName,
			Message:  fmt.Sprintf("No data-quality job matching %q was found.", jobName),
			Guidance: "Verify the job name — it may be misspelled, or the job may not exist. Create it first if needed.",
		}
	case 1:
		return names[0], nil
	default:
		return "", &Output{
			Status:            StatusNeedsInput,
			JobName:           jobName,
			CandidateJobNames: names,
			Message:           fmt.Sprintf("Found %d jobs matching %q.", len(names), jobName),
			Guidance:          "Pick the exact name from candidateJobNames and re-call this tool with it.",
		}
	}
}

// runPlan renders the request that will be submitted, in the same terms the user approved.
func runPlan(jobName string, input Input) *RunPlan {
	plan := &RunPlan{JobName: jobName}
	if input.RunDateValue != "" {
		plan.RunDate = fmt.Sprintf("%s (%s)", input.RunDateValue, input.RunDateKind)
	}
	if input.RunDateEndValue != "" {
		plan.RunDateEnd = fmt.Sprintf("%s (%s)", input.RunDateEndValue, input.RunDateEndKind)
	}
	if input.BackrunTimeBin != "" && input.BackrunBinValue > 0 {
		plan.Backrun = fmt.Sprintf("%d prior %s period(s)", input.BackrunBinValue, input.BackrunTimeBin)
	}
	plan.UsesJobDefaults = plan.RunDate == "" && plan.RunDateEnd == "" && plan.Backrun == ""
	return plan
}

func previewMessage(jobName string, plan *RunPlan) string {
	if plan.UsesJobDefaults {
		return fmt.Sprintf("Preview: job %q would run now with its current settings. Nothing has been queued yet.", jobName)
	}
	if plan.Backrun != "" {
		return fmt.Sprintf("Preview: job %q would run with the overrides below, AND backfill %s — this can queue a large number of runs. Nothing has been queued yet.", jobName, plan.Backrun)
	}
	return fmt.Sprintf("Preview: job %q would run with the overrides below. Nothing has been queued yet.", jobName)
}

// buildRunRequest assembles the optional run-window override from the input, or returns a
// needs_input Output when a kind/value pair or the backrun pair is only half-supplied.
func buildRunRequest(input Input) (clients.RunDqJobRequest, *Output) {
	var req clients.RunDqJobRequest

	runDate, err := runDateOverride("runDate", input.RunDateKind, input.RunDateValue)
	if err != nil {
		return req, err
	}
	req.RunDate = runDate

	runDateEnd, err := runDateOverride("runDateEnd", input.RunDateEndKind, input.RunDateEndValue)
	if err != nil {
		return req, err
	}
	req.RunDateEnd = runDateEnd

	switch {
	case input.BackrunTimeBin == "" && input.BackrunBinValue == 0:
		// no backfill requested
	case input.BackrunTimeBin == "" || input.BackrunBinValue <= 0:
		return req, &Output{
			Status:   StatusNeedsInput,
			Message:  "backrunTimeBin and backrunBinValue must be supplied together, with backrunBinValue >= 1.",
			Guidance: "Set both backrunTimeBin ('DAY', 'MONTH' or 'YEAR') and backrunBinValue (1 or greater), or omit both to run without backfilling.",
		}
	default:
		req.Backrun = &clients.DqPublicBackrun{TimeBin: input.BackrunTimeBin, BinValue: input.BackrunBinValue}
	}

	return req, nil
}

func runDateOverride(field, kind, value string) (*clients.DqPublicRunDate, *Output) {
	switch {
	case kind == "" && value == "":
		return nil, nil
	case kind == "" || value == "":
		return nil, &Output{
			Status:   StatusNeedsInput,
			Message:  fmt.Sprintf("%sKind and %sValue must be supplied together.", field, field),
			Guidance: fmt.Sprintf("Set both %sKind ('DATE' for yyyy-MM-dd, or 'TIMESTAMP' for RFC3339) and %sValue, or omit both to use the job's current %s.", field, field, field),
		}
	}
	return &clients.DqPublicRunDate{Kind: kind, Value: value}, nil
}

func lookupError(code int, err error, jobName string) Output {
	out := Output{Status: StatusError, JobName: jobName}
	switch code {
	case http.StatusNotFound:
		out.Message = fmt.Sprintf("No data-quality job named %q was found (HTTP 404).", jobName)
		out.Guidance = "Verify the job name — it may be misspelled, or the job has been deleted."
	case http.StatusUnauthorized:
		out.Message = "Not authenticated to the data-quality API (HTTP 401)."
		out.Guidance = "Your Collibra session/token is missing or expired — re-authenticate and retry."
	case http.StatusForbidden:
		out.Message = fmt.Sprintf("You do not have permission to view job %q (HTTP 403).", jobName)
		out.Guidance = "You need the DATA_QUALITY permission on the job. Ask an administrator for the Data Quality Admin or Data Quality User global role, then retry."
	case http.StatusBadRequest:
		out.Message = fmt.Sprintf("The data-quality API rejected the lookup of job %q (HTTP 400): %v", jobName, err)
		out.Guidance = "Check that the job name is correct and well-formed, then retry."
	case 0:
		out.Message = fmt.Sprintf("Failed to look up job %q: %v", jobName, err)
		out.Guidance = "A network/transport error occurred contacting the data-quality API. Retry."
	default:
		out.Message = fmt.Sprintf("Failed to look up job %q (HTTP %d): %v", jobName, code, err)
		out.Guidance = "This is likely a server-side error. Retry shortly; if it persists, contact your Collibra administrator."
	}
	return out
}

func runError(code int, err error, jobName string) Output {
	out := Output{Status: StatusError, JobName: jobName}
	switch code {
	case http.StatusBadRequest:
		out.Message = fmt.Sprintf("The data-quality API rejected the run request (HTTP 400): %v", err)
		out.Guidance = "Check runDateKind/runDateValue ('DATE' is yyyy-MM-dd, 'TIMESTAMP' is RFC3339), runDateEndKind/runDateEndValue, and backrunTimeBin/backrunBinValue, then retry."
	case http.StatusUnauthorized:
		out.Message = "Not authenticated to the data-quality API (HTTP 401)."
		out.Guidance = "Your Collibra session/token is missing or expired — re-authenticate and retry."
	case http.StatusForbidden:
		out.Message = fmt.Sprintf("You do not have permission to run job %q (HTTP 403).", jobName)
		out.Guidance = "You need the DATA_QUALITY_JOB_RUN permission on the job. Ask an administrator for the Data Quality Admin or Data Quality User global role, then retry."
	case http.StatusNotFound:
		out.Message = fmt.Sprintf("No data-quality job named %q was found (HTTP 404).", jobName)
		out.Guidance = "The job may have been deleted between the lookup and the run. Verify the job name."
	case 0:
		out.Message = fmt.Sprintf("Run request failed: %v", err)
		out.Guidance = "A network/transport error occurred contacting the data-quality API. The run may or may not have been queued — check with dq_get_job_run or the DQ UI before retrying."
	default:
		out.Message = fmt.Sprintf("The data-quality service failed to run job %q (HTTP %d): %v", jobName, code, err)
		out.Guidance = "This is likely a server-side error. Retry shortly; if it persists, contact your Collibra administrator."
	}
	return out
}
