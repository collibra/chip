// Package delete_dq_job implements the dq_delete_job MCP tool — permanently delete a Collibra
// data-quality job definition, along with every run, rule and result attached to it.
//
// The flow is a two-call converge on the public delete:
//
//	Look up : GET    /rest/dq/1.0/jobs/{jobName} -> on failure, relay a meaningful error;
//	          on success, summarise the job (confirm_required) so the user can review it.
//	Delete  : DELETE /rest/dq/1.0/jobs/{jobName} — only once the caller re-calls with confirm=true.
//
// Deletion is irreversible, so — as in dq_delete_job_run — confirm=false (the default) is strictly
// READ-ONLY: it never reaches the DELETE. Permissions are enforced by the server; 400/401/403/404/409
// and transport failures are surfaced as messages with actionable guidance rather than Go errors.
package delete_dq_job

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
	StatusDeleted         Status = "deleted"
	StatusConfirmRequired Status = "confirm_required"
	StatusNeedsInput      Status = "needs_input"
	StatusError           Status = "error"
)

type Input struct {
	JobName string `json:"jobName" jsonschema:"The name of the data-quality job to delete."`
	Confirm bool   `json:"confirm,omitempty" jsonschema:"Safety checkpoint. false (default) is READ-ONLY: it returns the job's details WITHOUT deleting anything — review them with the user. true performs the irreversible delete."`
}

// JobSummary describes the job that is about to be deleted, so the user can check it is the right one.
type JobSummary struct {
	JobName         string `json:"jobName" jsonschema:"Re-call the tool with this jobName and confirm=true to delete this job."`
	JobType         string `json:"jobType,omitempty" jsonschema:"The job's execution type (PUSHDOWN/PULLUP), if known."`
	EdgeSiteName    string `json:"edgeSiteName,omitempty" jsonschema:"The edge site the job runs on."`
	ConnectionName  string `json:"connectionName,omitempty" jsonschema:"The edge connection the job reads through."`
	SchemaName      string `json:"schemaName,omitempty" jsonschema:"The schema the job reads from."`
	TableName       string `json:"tableName,omitempty" jsonschema:"The table the job reads from."`
	SourceQuery     string `json:"sourceQuery,omitempty" jsonschema:"The SQL the job runs against the source."`
	ScheduleEnabled bool   `json:"scheduleEnabled,omitempty" jsonschema:"Whether the job currently has an active schedule — deleting it also removes the schedule."`
	ScheduleMode    string `json:"scheduleMode,omitempty" jsonschema:"The schedule's mode (HOURLY/DAILY/MONTHLY/...), when scheduled."`
	RunDate         string `json:"runDate,omitempty" jsonschema:"The job's configured run date, if any."`
}

type Output struct {
	Status         Status      `json:"status" jsonschema:"deleted | confirm_required | needs_input | error."`
	Message        string      `json:"message" jsonschema:"Human-readable outcome and what to do next."`
	JobName        string      `json:"jobName,omitempty" jsonschema:"The job that was deleted (or, on confirm_required, the one to confirm)."`
	Job            *JobSummary `json:"job,omitempty" jsonschema:"On confirm_required: the job that will be permanently deleted. Show it to the user before confirming."`
	JobDetailsLink string      `json:"jobDetailsLink,omitempty" jsonschema:"Job Details deep-link path (relative to the Collibra instance URL)."`
	Guidance       string      `json:"guidance,omitempty" jsonschema:"On confirm_required/needs_input/error, what to do next."`
}

func NewTool(collibraClient *http.Client) *chip.Tool[Input, Output] {
	return &chip.Tool[Input, Output]{
		Name:  "dq_delete_job",
		Title: "Delete Data Quality Job",
		Description: "PERMANENTLY DELETES a Collibra data-quality job definition, together with ALL of its " +
			"runs, rules, monitors and results. THIS CANNOT BE UNDONE. Identify the job by its jobName.\n\n" +
			"SAFETY CHECKPOINT: confirm=false (the default) is READ-ONLY — it looks the job up and returns a " +
			"summary (job type, edge site, connection, schema/table, source query, schedule) so you can review " +
			"it with the user, and deletes nothing. Call again with the same jobName and confirm=true to " +
			"actually delete.\n\n" +
			"If the job does not exist, or you lack permission to read or delete it, the tool reports that " +
			"instead of deleting anything. If the job has a run in progress the service may refuse the delete " +
			"— cancel the run first with dq_cancel_job_run, then retry.\n\n" +
			"To delete a single run rather than the whole job, use dq_delete_job_run instead.\n\n" +
			"Example user requests: \"Delete the data quality job sales.orders\"; \"Remove the DQ job for my " +
			"customers table\"; \"Tear down the quality check we set up on public.transactions.\"",
		Handler:     handler(collibraClient),
		Permissions: []string{},
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint:    false,
			DestructiveHint: chip.Ptr(true),
			IdempotentHint:  true,
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
				Message:  "Provide the job to delete.",
				Guidance: "Supply jobName — the name of the data-quality job to delete.",
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
		return previewOrDelete(ctx, collibraClient, jobName, input.Confirm), nil
	}
}

// previewOrDelete looks the job up, then either returns it for confirmation or deletes it.
func previewOrDelete(ctx context.Context, collibraClient *http.Client, jobName string, confirm bool) Output {
	job, code, err := clients.GetDqJob(ctx, collibraClient, jobName)
	if err != nil {
		return lookupError(code, err, jobName)
	}
	if !confirm {
		return confirmRequired(job, jobName)
	}

	if code, err := clients.DeleteDqJob(ctx, collibraClient, jobName); err != nil {
		return deleteError(code, err, jobName)
	}
	return Output{
		Status:  StatusDeleted,
		JobName: jobName,
		Message: fmt.Sprintf("Job %q and all of its runs, rules and results have been permanently deleted.", jobName),
	}
}

// confirmRequired is the safety checkpoint: it describes the job that would be deleted and asks the
// caller to come back with confirm=true.
func confirmRequired(job *clients.DqJobDefinition, jobName string) Output {
	summary := jobSummary(job, jobName)
	return Output{
		Status:         StatusConfirmRequired,
		JobName:        jobName,
		Job:            &summary,
		JobDetailsLink: clients.DqJobDetailsPath(jobName),
		Message:        fmt.Sprintf("About to PERMANENTLY delete job %q and all of its runs, rules and results.", jobName),
		Guidance: "Nothing has been deleted yet. Show the job details to the user and get their explicit approval, then re-call this tool with jobName=" +
			jobName + " and confirm=true. This cannot be undone.",
	}
}

func jobSummary(job *clients.DqJobDefinition, jobName string) JobSummary {
	summary := JobSummary{
		JobName:        jobName,
		JobType:        job.JobType,
		EdgeSiteName:   job.DataLocation.EdgeSiteName,
		ConnectionName: job.DataLocation.EdgeConnectionName,
		SchemaName:     job.DataLocation.SchemaName,
		TableName:      job.DataLocation.TableName,
		SourceQuery:    job.SourceQuery,
		RunDate:        job.RunDateValue(),
	}
	if job.JobName != "" {
		summary.JobName = job.JobName
	}
	if job.SchedulingSettings != nil {
		summary.ScheduleEnabled = job.SchedulingSettings.IsActive
		summary.ScheduleMode = job.SchedulingSettings.SchedulerMode
	}
	return summary
}

func lookupError(code int, err error, jobName string) Output {
	out := Output{Status: StatusError, JobName: jobName}
	switch code {
	case http.StatusNotFound:
		out.Message = fmt.Sprintf("No data-quality job named %q was found (HTTP 404).", jobName)
		out.Guidance = "Verify the job name — it may be misspelled, or the job has already been deleted."
	case http.StatusUnauthorized:
		out.Message = "Not authenticated to the data-quality API (HTTP 401)."
		out.Guidance = "Your Collibra session/token is missing or expired — re-authenticate and retry."
	case http.StatusForbidden:
		out.Message = fmt.Sprintf("You do not have permission to view job %q (HTTP 403).", jobName)
		out.Guidance = "You need the Data Quality Job > View permission on the connection. Ask an administrator for the Data Quality Editor/Manager role, then retry."
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

func deleteError(code int, err error, jobName string) Output {
	out := Output{Status: StatusError, JobName: jobName}
	switch code {
	case http.StatusBadRequest:
		out.Message = fmt.Sprintf("The data-quality API rejected the delete request (HTTP 400): %v", err)
		out.Guidance = "Check that the job name is correct and well-formed, then retry."
	case http.StatusUnauthorized:
		out.Message = "Not authenticated to the data-quality API (HTTP 401)."
		out.Guidance = "Your Collibra session/token is missing or expired — re-authenticate and retry."
	case http.StatusForbidden:
		out.Message = fmt.Sprintf("You do not have permission to delete job %q (HTTP 403).", jobName)
		out.Guidance = "You need the Data Quality Job > Delete permission (or Resource Manage All). Ask an administrator for the Data Quality Manager role, then retry."
	case http.StatusNotFound:
		out.Message = fmt.Sprintf("No data-quality job named %q was found (HTTP 404).", jobName)
		out.Guidance = "The job may have been deleted between the lookup and the delete. Verify the job name."
	case http.StatusConflict:
		out.Message = fmt.Sprintf("Job %q can't be deleted right now (HTTP 409).", jobName)
		out.Guidance = "The job most likely has a run in progress. Cancel it first with dq_cancel_job_run, then delete the job."
	case 0:
		out.Message = fmt.Sprintf("Delete failed: %v", err)
		out.Guidance = "A network/transport error occurred contacting the data-quality API. The job may or may not have been deleted — look it up before retrying."
	default:
		out.Message = fmt.Sprintf("The data-quality service failed to delete job %q (HTTP %d): %v", jobName, code, err)
		out.Guidance = "This is likely a server-side error. Retry shortly; if it persists, contact your Collibra administrator."
	}
	return out
}
