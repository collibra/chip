// Package delete_dq_job_run implements the dq_delete_job_run MCP tool — permanently delete a
// COMPLETED Collibra data-quality job run and all of its per-run results.
//
// It accepts EITHER a jobRunId or a jobName and converges on the public delete call:
//
//	By jobRunId : GET /rest/dq/1.0/jobRuns/{jobRunId} -> if the run is still in progress, refuse;
//	              otherwise show the run (confirm_required) or, with confirm=true, delete it.
//	By jobName  : GET /rest/dq/1.0/jobRuns?nameMatchMode=CONTAINS&status=<terminal...> to find the
//	              job's deletable runs -> none: error; exactly one: show it (confirm_required);
//	              several: return them (needs_input) so the caller re-calls with the chosen jobRunId.
//	Delete      : DELETE /rest/dq/1.0/jobRuns/{jobRunId}.
//
// Deletion is irreversible, so — unlike dq_cancel_job_run — there IS a confirm checkpoint, and the
// delete NEVER fires on a jobName: resolving by name always ends in confirm_required/needs_input, so
// a run can only be deleted once its details have been shown and the caller re-calls with its
// jobRunId and confirm=true. Permissions are enforced by the server — a 403 is surfaced as a
// meaningful error, along with 400/401/404/409/500.
package delete_dq_job_run

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/collibra/chip/pkg/chip"
	"github.com/collibra/chip/pkg/clients"
)

type Status string

const (
	StatusDeleted         Status = "deleted"
	StatusConfirmRequired Status = "confirm_required"
	StatusNeedsInput      Status = "needs_input"
	StatusError           Status = "error"
)

// Input — supply EITHER jobRunId OR jobName (exactly one).
type Input struct {
	JobRunID string `json:"jobRunId,omitempty" jsonschema:"The id of the completed run to delete. Provide this OR jobName."`
	JobName  string `json:"jobName,omitempty" jsonschema:"The job name whose completed run(s) to delete. The tool finds deletable (terminal) runs by name and returns them for selection; it never deletes directly from a name. Provide this OR jobRunId."`
	Confirm  bool   `json:"confirm,omitempty" jsonschema:"Safety checkpoint. false (default) is READ-ONLY: it returns the run's details WITHOUT deleting anything — review them with the user. true (with jobRunId) performs the irreversible delete."`
}

// RunSummary describes one run — either the run about to be deleted, or one of several candidates
// offered when jobName matched more than one.
type RunSummary struct {
	JobRunID  string `json:"jobRunId" jsonschema:"Re-call the tool with this jobRunId and confirm=true to delete this run."`
	JobName   string `json:"jobName,omitempty" jsonschema:"The job this run belongs to."`
	Status    string `json:"status,omitempty" jsonschema:"The run's terminal state (finished/cancelled/failed)."`
	RunDate   string `json:"runDate,omitempty" jsonschema:"The run's date (UTC), if known."`
	StartTime string `json:"startTime,omitempty" jsonschema:"When the run started (UTC), if known."`
	EndTime   string `json:"endTime,omitempty" jsonschema:"When the run finished (UTC), if known."`
}

type Output struct {
	Status         Status       `json:"status" jsonschema:"deleted | confirm_required | needs_input | error."`
	Message        string       `json:"message" jsonschema:"Human-readable outcome and what to do next."`
	JobRunID       string       `json:"jobRunId,omitempty" jsonschema:"The run id that was deleted (or, on confirm_required, the one to confirm)."`
	JobName        string       `json:"jobName,omitempty" jsonschema:"The job the run belongs to, when known."`
	RunState       string       `json:"runState,omitempty" jsonschema:"On an in-progress error: the run's current, non-deletable state."`
	Run            *RunSummary  `json:"run,omitempty" jsonschema:"On confirm_required: the run that will be permanently deleted. Show it to the user before confirming."`
	CandidateRuns  []RunSummary `json:"candidateRuns,omitempty" jsonschema:"When jobName matched multiple deletable runs: pick one and re-call with its jobRunId and confirm=true."`
	JobDetailsLink string       `json:"jobDetailsLink,omitempty" jsonschema:"Job Details deep-link path (relative to the Collibra instance URL), when jobName is known."`
	Guidance       string       `json:"guidance,omitempty" jsonschema:"On confirm_required/needs_input/error, what to do next."`
}

func NewTool(collibraClient *http.Client) *chip.Tool[Input, Output] {
	return &chip.Tool[Input, Output]{
		Name:  "dq_delete_job_run",
		Title: "Delete Data Quality Job Run",
		Description: "PERMANENTLY DELETES a COMPLETED Collibra data-quality job run along with ALL of its " +
			"per-run results (profile, scan, monitor, rule and alert output). THIS CANNOT BE UNDONE. Supply " +
			"EITHER the run's jobRunId OR the jobName of the job whose completed run you want to delete.\n\n" +
			"SAFETY CHECKPOINT: confirm=false (the default) is READ-ONLY — it returns the run's details " +
			"(job name, run id, run date, status) so you can review them with the user, and deletes nothing. " +
			"Call again with that jobRunId and confirm=true to actually delete. A jobName NEVER deletes " +
			"directly: it only resolves candidate runs.\n\n" +
			"BY jobRunId: the tool looks up the run and refuses if it is still in progress, telling you to " +
			"cancel it first with dq_cancel_job_run; otherwise it returns the run for confirmation (or deletes " +
			"it when confirm=true).\n\n" +
			"BY jobName: the tool finds the job's deletable (finished/cancelled/failed) runs. If there are none " +
			"it says so; if exactly one it returns it for confirmation; if several, it returns the candidate " +
			"runs (status=needs_input) so you can pick one and re-call with its jobRunId and confirm=true.\n\n" +
			"Only terminal runs can be deleted — use dq_cancel_job_run to stop an in-progress run first. API " +
			"errors (permission denied, run not found, run in progress, etc.) are surfaced as meaningful " +
			"messages.\n\n" +
			"Example user requests: \"Delete data quality run <id>\"; \"Remove the failed DQ run for " +
			"sales.orders\"; \"Clean up the old quality check results on my customers table.\"",
		Handler:     handler(collibraClient),
		Permissions: []string{},
	}
}

func handler(collibraClient *http.Client) chip.ToolHandlerFunc[Input, Output] {
	return func(ctx context.Context, input Input) (Output, error) {
		jobRunID := strings.TrimSpace(input.JobRunID)
		jobName := strings.TrimSpace(input.JobName)

		// Exactly one of jobRunId / jobName is required.
		switch {
		case jobRunID == "" && jobName == "":
			return Output{
				Status:   StatusNeedsInput,
				Message:  "Provide the run to delete.",
				Guidance: "Supply either jobRunId (the completed run's id) or jobName (the job whose completed run to delete) — exactly one.",
			}, nil
		case jobRunID != "" && jobName != "":
			return Output{
				Status:   StatusNeedsInput,
				Message:  "Provide only one of jobRunId or jobName.",
				Guidance: "Supply either jobRunId or jobName, not both. Use jobRunId to delete a specific run; use jobName to find the job's deletable runs.",
			}, nil
		}

		// By job name: resolve candidates only — never delete straight from a name.
		if jobName != "" {
			return resolveByJobName(ctx, collibraClient, jobName), nil
		}
		return deleteByJobRunID(ctx, collibraClient, jobRunID, input.Confirm), nil
	}
}

// resolveByJobName finds the job's deletable runs and returns either the single run to confirm or
// the candidate list to choose from. It never deletes.
func resolveByJobName(ctx context.Context, collibraClient *http.Client, jobName string) Output {
	runs, code, err := clients.SearchDeletableDqJobRuns(ctx, collibraClient, jobName)
	if err != nil {
		return runLookupError(code, err, fmt.Sprintf("job name %q", jobName))
	}

	switch len(runs) {
	case 0:
		return Output{
			Status:         StatusError,
			JobName:        jobName,
			Message:        fmt.Sprintf("No deletable (completed) runs found for job name %q.", jobName),
			JobDetailsLink: clients.DqJobDetailsPath(jobName),
			Guidance: "Only terminal runs (" + strings.Join(clients.DqTerminalRunStates, "/") + ") can be deleted. " +
				"Check the job name, or the run may still be in progress — cancel it first with dq_cancel_job_run.",
		}
	case 1:
		return confirmRequired(runs[0])
	default:
		candidates := make([]RunSummary, 0, len(runs))
		for i := range runs {
			candidates = append(candidates, runSummary(&runs[i]))
		}
		return Output{
			Status:         StatusNeedsInput,
			JobName:        jobName,
			CandidateRuns:  candidates,
			JobDetailsLink: clients.DqJobDetailsPath(jobName),
			Message:        fmt.Sprintf("Found %d deletable runs matching job name %q.", len(runs), jobName),
			Guidance:       "Show these to the user, then re-call this tool with the chosen run's jobRunId and confirm=true. Deleting a run also deletes all of its results and cannot be undone. Note: dates and times are shown in the format returned by the API and may not match the formatting displayed in the Collibra application's UI.",
		}
	}
}

// deleteByJobRunID checks the run's state, then either returns it for confirmation or deletes it.
func deleteByJobRunID(ctx context.Context, collibraClient *http.Client, jobRunID string, confirm bool) Output {
	run, code, err := clients.GetDqJobRun(ctx, collibraClient, jobRunID)
	if err != nil {
		return runLookupError(code, err, fmt.Sprintf("run %q", jobRunID))
	}
	if !clients.IsDeletableDqRunState(run.Status) {
		return Output{
			Status:   StatusError,
			JobRunID: jobRunID,
			JobName:  run.JobName,
			RunState: run.Status,
			Message:  fmt.Sprintf("Run %q is in state %q and can't be deleted.", jobRunID, run.Status),
			Guidance: "Only completed runs can be deleted; this one is still in progress (" +
				strings.Join(clients.DqCancellableRunStates, "/") + "). Cancel it first with dq_cancel_job_run, then delete it.",
		}
	}
	if !confirm {
		return confirmRequired(*run)
	}

	if code, err := clients.DeleteDqJobRun(ctx, collibraClient, jobRunID); err != nil {
		out := deleteError(code, err, jobRunID)
		out.JobName = run.JobName
		return out
	}

	out := Output{
		Status:   StatusDeleted,
		JobRunID: jobRunID,
		JobName:  run.JobName,
		Message:  fmt.Sprintf("Run %q and all of its results have been permanently deleted.", jobRunID),
	}
	if run.JobName != "" {
		out.JobDetailsLink = clients.DqJobDetailsPath(run.JobName)
		out.Message = fmt.Sprintf("Run %q of job %q and all of its results have been permanently deleted.", jobRunID, run.JobName)
	}
	return out
}

// confirmRequired is the safety checkpoint: it describes the run that would be deleted and asks the
// caller to come back with confirm=true.
func confirmRequired(run clients.DqJobRun) Output {
	summary := runSummary(&run)
	out := Output{
		Status:   StatusConfirmRequired,
		JobRunID: run.JobRunID,
		JobName:  run.JobName,
		Run:      &summary,
		Message:  fmt.Sprintf("About to PERMANENTLY delete run %q (job %q, state %q) and all of its results.", run.JobRunID, run.JobName, run.Status),
		Guidance: "Nothing has been deleted yet. Show the run details to the user and get their explicit approval, then re-call this tool with jobRunId=" +
			run.JobRunID + " and confirm=true. This cannot be undone. Note: dates and times are shown in the format returned by the API and may not match the formatting displayed in the Collibra application's UI.",
	}
	if run.JobName != "" {
		out.JobDetailsLink = clients.DqJobDetailsPath(run.JobName)
	}
	return out
}

func runSummary(run *clients.DqJobRun) RunSummary {
	return RunSummary{
		JobRunID:  run.JobRunID,
		JobName:   run.JobName,
		Status:    run.Status,
		RunDate:   run.RunDateValue(),
		StartTime: run.StartTime,
		EndTime:   run.EndTime,
	}
}

func runLookupError(code int, err error, subject string) Output {
	switch code {
	case http.StatusNotFound:
		return Output{Status: StatusError, Message: fmt.Sprintf("No %s was found (HTTP 404).", subject), Guidance: "Verify the id/name — it may be wrong, or the run has already been deleted."}
	case http.StatusUnauthorized:
		return Output{Status: StatusError, Message: "Not authenticated to the data-quality API (HTTP 401).", Guidance: "Your Collibra session/token is missing or expired — re-authenticate and retry."}
	case http.StatusForbidden:
		return Output{Status: StatusError, Message: fmt.Sprintf("You do not have permission to view %s (HTTP 403).", subject), Guidance: "You need the Data Quality Job > View permission on the connection. Ask an administrator for the Data Quality Editor/Manager role, then retry."}
	case http.StatusBadRequest:
		return Output{Status: StatusError, Message: fmt.Sprintf("The data-quality API rejected the lookup of %s (HTTP 400): %v", subject, err), Guidance: "Check that the jobRunId/jobName is correct and well-formed, then retry."}
	case 0:
		return Output{Status: StatusError, Message: fmt.Sprintf("Failed to look up %s: %v", subject, err), Guidance: "A network/transport error occurred contacting the data-quality API. Retry."}
	default:
		return Output{Status: StatusError, Message: fmt.Sprintf("Failed to look up %s (HTTP %d): %v", subject, code, err), Guidance: "This is likely a server-side error. Retry shortly; if it persists, contact your Collibra administrator."}
	}
}

func deleteError(code int, err error, jobRunID string) Output {
	switch code {
	case http.StatusBadRequest:
		return Output{Status: StatusError, JobRunID: jobRunID, Message: fmt.Sprintf("The data-quality API rejected the delete request (HTTP 400): %v", err), Guidance: "Check that the jobRunId is correct and well-formed, then retry."}
	case http.StatusUnauthorized:
		return Output{Status: StatusError, JobRunID: jobRunID, Message: "Not authenticated to the data-quality API (HTTP 401).", Guidance: "Your Collibra session/token is missing or expired — re-authenticate and retry."}
	case http.StatusForbidden:
		return Output{Status: StatusError, JobRunID: jobRunID, Message: "You do not have permission to delete this data-quality run (HTTP 403).", Guidance: "You need the Data Quality Job > Delete permission (or Resource Manage All). Ask an administrator for the Data Quality Manager role, then retry."}
	case http.StatusNotFound:
		return Output{Status: StatusError, JobRunID: jobRunID, Message: fmt.Sprintf("No run found for jobRunId %q (HTTP 404).", jobRunID), Guidance: "The run id may be wrong, or the run has already been deleted. Verify the run id."}
	case http.StatusConflict:
		return Output{Status: StatusError, JobRunID: jobRunID, Message: fmt.Sprintf("Run %q is in progress and can't be deleted (HTTP 409).", jobRunID), Guidance: "The run started or was still running between the state check and the delete. Cancel it first with dq_cancel_job_run, then delete it."}
	case 0:
		return Output{Status: StatusError, JobRunID: jobRunID, Message: fmt.Sprintf("Delete failed: %v", err), Guidance: "A network/transport error occurred contacting the data-quality API. The run may or may not have been deleted — look it up before retrying."}
	default:
		return Output{Status: StatusError, JobRunID: jobRunID, Message: fmt.Sprintf("The data-quality service failed to delete the run (HTTP %d): %v", code, err), Guidance: "This is likely a server-side error. Retry shortly; if it persists, contact your Collibra administrator."}
	}
}
