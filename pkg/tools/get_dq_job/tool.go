// Package get_dq_job implements the dq_get_job MCP tool — read the full definition of a single
// Collibra data-quality job by name.
//
// The flow converges on the public single-job GET, with a fallback for a partial/fuzzy name:
//
//	Exact  : GET /rest/dq/1.0/jobs/{jobName} -> on success, return the job definition.
//	Fuzzy  : on a 404, GET /rest/dq/1.0/jobs?jobName=<name> (searchJobs, %LIKE% match) -> zero
//	         matches: not-found error; exactly one: fetch and return it; several: return the
//	         candidate names (needs_input) so the caller can pick one and re-call with the exact name.
//
// This is a pure read: no confirm checkpoint, no writes. 400/401/403/404/500 and transport failures
// are surfaced as messages with actionable guidance rather than Go errors.
package get_dq_job

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
	StatusSuccess    Status = "success"
	StatusNeedsInput Status = "needs_input"
	StatusError      Status = "error"
)

// Input is the tool's typed input.
type Input struct {
	Name string `json:"name" jsonschema:"Required. The name of the data-quality job to retrieve (a job, also called a 'dataset', is a saved check on one database table), e.g. 'PUBLIC.SAMPLE_DATASET'. An exact match is tried first; if none is found, jobs whose name contains this text are offered as candidates."`
}

// JobDetail is the full job definition returned on success.
type JobDetail struct {
	JobName        string `json:"jobName"`
	JobType        string `json:"jobType,omitempty" jsonschema:"The job's execution type: PUSHDOWN or PULLUP."`
	EdgeSiteName   string `json:"edgeSiteName,omitempty"`
	ConnectionName string `json:"connectionName,omitempty" jsonschema:"The edge connection the job reads through."`
	DataSourceName string `json:"dataSourceName,omitempty" jsonschema:"The database/catalog the job reads from."`
	SchemaName     string `json:"schemaName,omitempty"`
	TableName      string `json:"tableName,omitempty"`
	SourceQuery    string `json:"sourceQuery,omitempty" jsonschema:"The SQL the job runs against the source."`
	RunDate        string `json:"runDate,omitempty" jsonschema:"Start of the job's configured run-date window, if any."`
	RunDateEnd     string `json:"runDateEnd,omitempty" jsonschema:"Exclusive end of the run-date window, if any."`

	AdaptiveMonitors []string `json:"adaptiveMonitors,omitempty" jsonschema:"Enabled built-in adaptive monitor keys (e.g. rowCount, nullValues)."`
	CustomMonitors   []string `json:"customMonitors,omitempty" jsonschema:"Names of the custom DQ rules (monitors) configured on this job. Use get_data_quality_rule for a rule's full definition."`
	NotificationsSet bool     `json:"notificationsSet,omitempty" jsonschema:"Whether the job has any notification configuration."`
	ScheduleEnabled  bool     `json:"scheduleEnabled,omitempty" jsonschema:"Whether the job currently has an active schedule."`
	ScheduleMode     string   `json:"scheduleMode,omitempty" jsonschema:"The schedule's mode (HOURLY/DAILY/MONTHLY), when scheduled."`
	JobDetailsLink   string   `json:"jobDetailsLink,omitempty" jsonschema:"Job Details deep-link path (relative to the Collibra instance URL)."`
}

// Output is the typed response.
type Output struct {
	Status            Status     `json:"status" jsonschema:"'success' when the job was found; 'needs_input' when the name matched none or several jobs; 'error' for downstream DQ failures."`
	Message           string     `json:"message" jsonschema:"Human-readable summary."`
	Job               *JobDetail `json:"job,omitempty" jsonschema:"The job definition, on success."`
	CandidateJobNames []string   `json:"candidateJobNames,omitempty" jsonschema:"When name matched several jobs: exact names to pick from — re-call with one of these."`
	Guidance          string     `json:"guidance,omitempty" jsonschema:"On needs_input/error, what to do next."`
}

// NewTool returns the registered tool.
func NewTool(collibraClient *http.Client) *chip.Tool[Input, Output] {
	return &chip.Tool[Input, Output]{
		Name:  "dq_get_job",
		Title: "Get Data Quality Job",
		Description: "Reads the full definition of a single Collibra data-quality job (a saved check on ONE " +
			"database table; also called a 'dataset') by name. Returns its type (PUSHDOWN/PULLUP), edge site, " +
			"connection, schema/table, source SQL, run-date window, configured monitors (adaptive + custom DQ " +
			"rules), notifications, and schedule.\n\n" +
			"An exact name match is tried first. If none is found, jobs whose name CONTAINS the given text are " +
			"offered as candidates (status=needs_input) so you can pick the exact one and re-call.\n\n" +
			"Note: a job definition has no 'id' or 'status' field in the API — only individual RUNS have a " +
			"status. Use dq_get_job_run (or find/search run tools) for run-level status, score and results.\n\n" +
			"Example user requests: \"Show me the data quality job for sales.orders\"; \"What monitors are " +
			"configured on public.nyse?\"; \"Get the details of the DQ job on my customers table.\"",
		Handler:     handler(collibraClient),
		Permissions: []string{},
	}
}

func handler(collibraClient *http.Client) chip.ToolHandlerFunc[Input, Output] {
	return func(ctx context.Context, input Input) (Output, error) {
		name := strings.TrimSpace(input.Name)
		if name == "" {
			return Output{
				Status:   StatusNeedsInput,
				Message:  "Provide the job to retrieve.",
				Guidance: "Supply name — the name of the data-quality job to retrieve.",
			}, nil
		}

		job, code, err := clients.GetDqJob(ctx, collibraClient, name)
		if err == nil {
			return success(job, name), nil
		}
		if code != http.StatusNotFound {
			return lookupError(code, err, name), nil
		}

		// No exact match — fall back to a fuzzy name search to disambiguate or report not-found.
		names, searchErr := clients.SearchDqJobNames(ctx, collibraClient, name)
		if searchErr != nil {
			return lookupError(0, searchErr, name), nil
		}
		switch len(names) {
		case 0:
			return Output{
				Status:   StatusError,
				Message:  fmt.Sprintf("No data-quality job matching %q was found.", name),
				Guidance: "Verify the job name — it may be misspelled, or the job may not exist.",
			}, nil
		case 1:
			job, code, err := clients.GetDqJob(ctx, collibraClient, names[0])
			if err != nil {
				return lookupError(code, err, names[0]), nil
			}
			return success(job, names[0]), nil
		default:
			return Output{
				Status:            StatusNeedsInput,
				CandidateJobNames: names,
				Message:           fmt.Sprintf("Found %d jobs matching %q.", len(names), name),
				Guidance:          "Pick the exact name from candidateJobNames and re-call this tool with it.",
			}, nil
		}
	}
}

func success(job *clients.DqJobDefinition, jobName string) Output {
	detail := JobDetail{
		JobName:          jobName,
		JobType:          job.JobType,
		EdgeSiteName:     job.DataLocation.EdgeSiteName,
		ConnectionName:   job.DataLocation.EdgeConnectionName,
		DataSourceName:   job.DataLocation.DataSourceName,
		SchemaName:       job.DataLocation.SchemaName,
		TableName:        job.DataLocation.TableName,
		SourceQuery:      job.SourceQuery,
		RunDate:          job.RunDateValue(),
		NotificationsSet: job.Notifications != nil,
		JobDetailsLink:   clients.DqJobDetailsPath(jobName),
	}
	if job.JobName != "" {
		detail.JobName = job.JobName
	}
	if job.RunDateEnd != nil {
		detail.RunDateEnd = job.RunDateEnd.Value
	}
	if job.MonitoringSettings != nil {
		detail.AdaptiveMonitors = clients.EnabledPublicMonitorKeys(job.MonitoringSettings.AdaptiveMonitors)
		for _, m := range job.MonitoringSettings.CustomMonitors {
			detail.CustomMonitors = append(detail.CustomMonitors, m.MonitorName)
		}
	}
	if job.SchedulingSettings != nil {
		detail.ScheduleEnabled = job.SchedulingSettings.IsActive
		detail.ScheduleMode = job.SchedulingSettings.SchedulerMode
	}
	return Output{
		Status:  StatusSuccess,
		Message: fmt.Sprintf("Found job %q.", detail.JobName),
		Job:     &detail,
	}
}

func lookupError(code int, err error, jobName string) Output {
	out := Output{Status: StatusError}
	switch code {
	case http.StatusNotFound:
		out.Message = fmt.Sprintf("No data-quality job named %q was found (HTTP 404).", jobName)
		out.Guidance = "Verify the job name — it may be misspelled, or the job may not exist."
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
