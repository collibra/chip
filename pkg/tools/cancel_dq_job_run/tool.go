// Package cancel_dq_job_run implements the dq_cancel_job_run MCP tool — cancel an in-progress
// Collibra data-quality job run.
//
// It accepts EITHER a jobRunId or a jobName and converges on the public cancel call:
//
//	By jobRunId : GET /rest/dq/1.0/jobRuns/{jobRunId} -> if the run is in a terminal (non-
//	              cancellable) state, refuse; otherwise cancel it.
//	By jobName  : GET /rest/dq/1.0/jobRuns?nameMatchMode=contains&status=<nonterminal...> to find
//	              the job's cancellable runs -> none: error; exactly one: cancel it; several:
//	              return them (needs_input) so the caller re-calls with the chosen jobRunId.
//	Cancel      : POST /rest/dq/1.0/jobRuns/{jobRunId}/cancel.
//
// There is no confirm checkpoint: the terminal-state pre-check (by-id) and the nonterminal search
// filter (by-name) are the safety mechanism. Permissions are enforced by the server — a 403 is
// surfaced as a meaningful error, along with 400/401/404/500.
package cancel_dq_job_run

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
	StatusCanceled   Status = "canceled"
	StatusNeedsInput Status = "needs_input"
	StatusError      Status = "error"
)

// Input — supply EITHER jobRunId OR jobName (exactly one).
type Input struct {
	JobRunID string `json:"jobRunId,omitempty" jsonschema:"The id of the in-progress run to cancel (the jobRunId returned when the run was created/queued). Provide this OR jobName."`
	JobName  string `json:"jobName,omitempty" jsonschema:"The job name whose in-progress run(s) to cancel. The tool finds cancellable (nonterminal) runs by name; if exactly one it cancels it, if several it returns them so you can pick one and re-call with that jobRunId. Provide this OR jobRunId."`
}

// CandidateRun is one cancellable run offered when jobName matched several.
type CandidateRun struct {
	JobRunID  string `json:"jobRunId" jsonschema:"Re-call the tool with this jobRunId to cancel this run."`
	JobName   string `json:"jobName,omitempty" jsonschema:"The job this run belongs to."`
	Status    string `json:"status,omitempty" jsonschema:"The run's current (nonterminal) state."`
	StartedAt string `json:"startedAt,omitempty" jsonschema:"When the run started, if known."`
}

type Output struct {
	Status         Status         `json:"status" jsonschema:"canceled | needs_input | error."`
	Message        string         `json:"message" jsonschema:"Human-readable outcome and what to do next."`
	JobRunID       string         `json:"jobRunId,omitempty" jsonschema:"The run id that was cancelled (or, on needs_input, the one to select)."`
	JobName        string         `json:"jobName,omitempty" jsonschema:"The job the run belongs to, when known."`
	RunState       string         `json:"runState,omitempty" jsonschema:"On a terminal-state error: the run's current, non-cancellable state."`
	CandidateRuns  []CandidateRun `json:"candidateRuns,omitempty" jsonschema:"When jobName matched multiple cancellable runs: pick one and re-call with its jobRunId."`
	JobDetailsLink string         `json:"jobDetailsLink,omitempty" jsonschema:"Job Details deep-link path (relative to the Collibra instance URL), when jobName is known."`
	Guidance       string         `json:"guidance,omitempty" jsonschema:"On needs_input/error, what to do next."`
}

func NewTool(collibraClient *http.Client) *chip.Tool[Input, Output] {
	return &chip.Tool[Input, Output]{
		Name:  "dq_cancel_job_run",
		Title: "Cancel Data Quality Job Run",
		Description: "Cancels an IN-PROGRESS Collibra data-quality job run. Supply EITHER the run's " +
			"jobRunId OR the jobName of the job whose active run you want to cancel.\n\n" +
			"BY jobRunId: the tool looks up the run's status and refuses if it is already in a terminal " +
			"state (finished/failed/cancelled/…) with a clear message; otherwise it cancels it.\n\n" +
			"BY jobName: the tool finds the job's cancellable (in-progress) runs. If there are none it says " +
			"so; if exactly one it cancels it; if several, it returns the candidate runs (status=needs_input) " +
			"so you can pick one and re-call with its jobRunId.\n\n" +
			"This WRITES to Collibra: cancelling aborts the run's in-progress work and is irreversible. On " +
			"success the cancellation is queued. API errors (permission denied, run not found, etc.) are " +
			"surfaced as meaningful messages.\n\n" +
			"Example user requests: \"Cancel data quality run <id>\"; \"Stop the running DQ job for " +
			"sales.orders\"; \"Abort the in-progress quality check on my customers table.\"",
		Handler:     handler(collibraClient),
		Permissions: []string{},
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint:    false,
			DestructiveHint: chip.Ptr(true),
			IdempotentHint:  false,
			OpenWorldHint:   chip.Ptr(false),
		},
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
				Message:  "Provide the run to cancel.",
				Guidance: "Supply either jobRunId (the in-progress run's id) or jobName (the job whose active run to cancel) — exactly one.",
			}, nil
		case jobRunID != "" && jobName != "":
			return Output{
				Status:   StatusNeedsInput,
				Message:  "Provide only one of jobRunId or jobName.",
				Guidance: "Supply either jobRunId or jobName, not both. Use jobRunId to cancel a specific run; use jobName to find and cancel the job's active run.",
			}, nil
		}

		// Resolve the run id to cancel (and its job name, for the message/link).
		if jobRunID != "" {
			// Path A — by run id: check the run's status and refuse terminal (non-cancellable) states.
			run, code, err := clients.GetDqJobRun(ctx, collibraClient, jobRunID)
			if err != nil {
				return runLookupError(code, err, fmt.Sprintf("run %q", jobRunID)), nil
			}
			if !clients.IsCancellableDqRunState(run.Status) {
				return Output{
					Status:   StatusError,
					JobRunID: jobRunID,
					JobName:  run.JobName,
					RunState: run.Status,
					Message:  fmt.Sprintf("Run %q is in state %q and can't be cancelled.", jobRunID, run.Status),
					Guidance: "Only in-progress runs (" + strings.Join(clients.DqCancellableRunStates, "/") + ") can be cancelled; this run has already reached a terminal state.",
				}, nil
			}
			jobName = run.JobName
		} else {
			// Path B — by job name: find cancellable runs and pick / disambiguate.
			runs, code, err := clients.SearchCancellableDqJobRuns(ctx, collibraClient, jobName)
			if err != nil {
				return runLookupError(code, err, fmt.Sprintf("job name %q", jobName)), nil
			}
			switch len(runs) {
			case 0:
				return Output{
					Status:   StatusError,
					JobName:  jobName,
					Message:  fmt.Sprintf("No cancellable (in-progress) runs found for job name %q.", jobName),
					Guidance: "Check the job name, or the run(s) may have already finished. Only in-progress runs can be cancelled.",
				}, nil
			case 1:
				jobRunID = runs[0].JobRunID
				if runs[0].JobName != "" {
					jobName = runs[0].JobName
				}
			default:
				candidates := make([]CandidateRun, 0, len(runs))
				for _, r := range runs {
					candidates = append(candidates, CandidateRun{JobRunID: r.JobRunID, JobName: r.JobName, Status: r.Status, StartedAt: r.StartedAt})
				}
				return Output{
					Status:        StatusNeedsInput,
					JobName:       jobName,
					CandidateRuns: candidates,
					Message:       fmt.Sprintf("Found %d cancellable runs matching job name %q.", len(runs), jobName),
					Guidance:      "Pick one from candidateRuns and re-call this tool with its jobRunId.",
				}, nil
			}
		}

		// Cancel the resolved run (both paths converge here).
		code, err := clients.CancelDqJobRun(ctx, collibraClient, jobRunID)
		if err != nil {
			out := cancelError(code, err, jobRunID)
			out.JobName = jobName
			return out, nil
		}

		out := Output{
			Status:   StatusCanceled,
			JobRunID: jobRunID,
			JobName:  jobName,
			Message:  fmt.Sprintf("Cancellation of run %q has been submitted.", jobRunID),
		}
		if jobName != "" {
			out.JobDetailsLink = clients.DqJobDetailsPath(jobName)
			out.Message = fmt.Sprintf("Cancellation of run %q (job %q) has been submitted.", jobRunID, jobName)
		}
		return out, nil
	}
}

func runLookupError(code int, err error, subject string) Output {
	switch code {
	case http.StatusNotFound:
		return Output{Status: StatusError, Message: fmt.Sprintf("No %s was found (HTTP 404).", subject), Guidance: "Verify the id/name — it may be wrong, or the run has already finished and is no longer available."}
	case http.StatusUnauthorized:
		return Output{Status: StatusError, Message: "Not authenticated to the data-quality API (HTTP 401).", Guidance: "Your Collibra session/token is missing or expired — re-authenticate and retry."}
	case http.StatusForbidden:
		return Output{Status: StatusError, Message: fmt.Sprintf("You do not have permission to view %s (HTTP 403).", subject), Guidance: "You need the Data Quality Job > Run (or Edit) permission on the connection. Ask an administrator for the Data Quality Editor/Manager role, then retry."}
	case http.StatusBadRequest:
		return Output{Status: StatusError, Message: fmt.Sprintf("The data-quality API rejected the lookup of %s (HTTP 400): %v", subject, err), Guidance: "Check that the jobRunId/jobName is correct and well-formed, then retry."}
	case 0:
		return Output{Status: StatusError, Message: fmt.Sprintf("Failed to look up %s: %v", subject, err), Guidance: "A network/transport error occurred contacting the data-quality API. Retry."}
	default:
		return Output{Status: StatusError, Message: fmt.Sprintf("Failed to look up %s (HTTP %d): %v", subject, code, err), Guidance: "This is likely a server-side error. Retry shortly; if it persists, contact your Collibra administrator."}
	}
}

func cancelError(code int, err error, jobRunID string) Output {
	switch code {
	case http.StatusBadRequest:
		return Output{Status: StatusError, JobRunID: jobRunID, Message: fmt.Sprintf("The data-quality API rejected the cancel request (HTTP 400): %v", err), Guidance: "Check that the jobRunId is correct and well-formed, then retry."}
	case http.StatusUnauthorized:
		return Output{Status: StatusError, JobRunID: jobRunID, Message: "Not authenticated to the data-quality API (HTTP 401).", Guidance: "Your Collibra session/token is missing or expired — re-authenticate and retry."}
	case http.StatusForbidden:
		return Output{Status: StatusError, JobRunID: jobRunID, Message: "You do not have permission to cancel this data-quality run (HTTP 403).", Guidance: "You need the Data Quality Job > Run (or Edit) permission on the connection. Ask an administrator for the Data Quality Editor/Manager role, then retry."}
	case http.StatusNotFound:
		return Output{Status: StatusError, JobRunID: jobRunID, Message: fmt.Sprintf("No cancellable run found for jobRunId %q (HTTP 404).", jobRunID), Guidance: "The run id may be wrong, or the run has already finished/been cancelled and is no longer in progress. Verify the run id."}
	case http.StatusConflict:
		return Output{Status: StatusError, JobRunID: jobRunID, Message: fmt.Sprintf("Run %q is no longer in a cancellable state (HTTP 409).", jobRunID), Guidance: "The run likely finished or was already cancelled between the status check and the cancel. No action needed."}
	case 0:
		return Output{Status: StatusError, JobRunID: jobRunID, Message: fmt.Sprintf("Cancel failed: %v", err), Guidance: "A network/transport error occurred contacting the data-quality API. Retry."}
	default:
		return Output{Status: StatusError, JobRunID: jobRunID, Message: fmt.Sprintf("The data-quality service failed to cancel the run (HTTP %d): %v", code, err), Guidance: "This is likely a server-side error. Retry shortly; if it persists, contact your Collibra administrator."}
	}
}
