// Package search_dq_job_runs implements the dq_search_job_runs MCP tool — search and list Collibra
// data-quality job run history with filters and pagination.
//
// This is a thin wrapper over the PUBLIC GET /rest/dq/1.0/jobRuns (searchJobRuns) endpoint: it does
// not try to disambiguate — it just returns whatever page of runs matches the given filters, which
// may be empty.
//
// This is a pure read: no confirm checkpoint, no writes. 400/401/403/500 and transport failures are
// surfaced as messages with actionable guidance rather than Go errors.
package search_dq_job_runs

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/collibra/chip/pkg/chip"
	"github.com/collibra/chip/pkg/clients"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// OutputStatus is the overall outcome of a dq_search_job_runs call.
type OutputStatus string

const (
	StatusSuccess         OutputStatus = "success"
	StatusValidationError OutputStatus = "validation_error"
	StatusError           OutputStatus = "error"
)

const (
	defaultPageSize = 25
	maxPageSize     = 100
)

// Input is the tool's typed input. All filters are optional; omitting all of them returns every
// accessible job run, paginated.
type Input struct {
	JobName  string `json:"job_name,omitempty" jsonschema:"Optional. Fuzzy, case-insensitive job-name filter (substring match)."`
	Status   string `json:"status,omitempty" jsonschema:"Optional. Filter by run status: WAITING, DISPATCHED, SETUP, RUNNING, SENDING, FINISHED, CANCELLED, FAILED, or UNKNOWN."`
	JobType  string `json:"job_type,omitempty" jsonschema:"Optional. Filter by job execution type: PUSHDOWN or PULLUP. If omitted, both types are returned."`
	Page     int    `json:"page,omitempty" jsonschema:"Optional. 1-based page number. Defaults to 1."`
	PageSize int    `json:"page_size,omitempty" jsonschema:"Optional. Results per page (1-100). Defaults to 25."`
}

// RunSummary is one matching job run.
type RunSummary struct {
	JobRunID  string `json:"jobRunId"`
	JobName   string `json:"jobName,omitempty"`
	JobType   string `json:"jobType,omitempty" jsonschema:"PUSHDOWN or PULLUP, when the API returns it for this run."`
	Status    string `json:"status,omitempty" jsonschema:"WAITING | DISPATCHED | SETUP | RUNNING | SENDING | FINISHED | CANCELLED | FAILED | UNKNOWN."`
	StartTime string `json:"startTime,omitempty"`
	EndTime   string `json:"endTime,omitempty" jsonschema:"When the run reached a terminal state; may be empty while in progress or if the search endpoint does not report it — use dq_get_job_run for the authoritative value."`
}

// Output is the typed response.
type Output struct {
	Status   OutputStatus `json:"status" jsonschema:"'success' when the search ran; 'validation_error' for bad inputs; 'error' for downstream DQ failures."`
	Message  string       `json:"message" jsonschema:"Human-readable summary."`
	Runs     []RunSummary `json:"runs,omitempty" jsonschema:"Matching job runs on this page."`
	Total    int64        `json:"total" jsonschema:"Total number of matching runs across all pages."`
	Page     int          `json:"page" jsonschema:"The 1-based page number returned."`
	PageSize int          `json:"pageSize" jsonschema:"The page size used."`
}

// NewTool returns the registered tool.
func NewTool(collibraClient *http.Client) *chip.Tool[Input, Output] {
	return &chip.Tool[Input, Output]{
		Name:  "dq_search_job_runs",
		Title: "Search Data Quality Job Runs",
		Description: "Searches and lists Collibra data-quality job run history, with optional filters and " +
			"pagination. Filter by a fuzzy job-name substring, exact run status, and/or job type " +
			"(PUSHDOWN/PULLUP). Omitting all filters returns every accessible run, one page at a time. Each " +
			"result includes the run id, job name, status, and start time.\n\n" +
			"For the full details of one run — including score, row count, and per-monitor results once it's " +
			"terminal — use dq_get_job_run with the returned jobRunId.\n\n" +
			"Example user requests: \"Show me the recent failed DQ runs\"; \"What runs are currently in " +
			"progress?\"; \"List the run history for the sales.orders job.\"",
		Handler:     handler(collibraClient),
		Permissions: []string{},
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, DestructiveHint: chip.Ptr(false), IdempotentHint: true, OpenWorldHint: chip.Ptr(false)},
	}
}

func handler(collibraClient *http.Client) chip.ToolHandlerFunc[Input, Output] {
	return func(ctx context.Context, input Input) (Output, error) {
		page := input.Page
		if page <= 0 {
			page = 1
		}
		pageSize := input.PageSize
		if pageSize == 0 {
			pageSize = defaultPageSize
		}
		if pageSize < 1 || pageSize > maxPageSize {
			return Output{Status: StatusValidationError, Message: fmt.Sprintf("page_size must be between 1 and %d.", maxPageSize)}, nil
		}

		jobName := strings.TrimSpace(input.JobName)
		jobType := strings.TrimSpace(input.JobType)
		var statuses []string
		if s := strings.TrimSpace(input.Status); s != "" {
			statuses = []string{s}
		}
		offset := (page - 1) * pageSize

		result, code, err := clients.SearchDqJobRuns(ctx, collibraClient, jobName, statuses, jobType, offset, pageSize)
		if err != nil {
			return lookupError(code, err), nil
		}

		runs := make([]RunSummary, 0, len(result.Results))
		for _, r := range result.Results {
			runs = append(runs, RunSummary{
				JobRunID:  r.JobRunID,
				JobName:   r.JobName,
				JobType:   r.JobType,
				Status:    r.Status,
				StartTime: r.StartTime,
				EndTime:   r.EndTime,
			})
		}

		return Output{
			Status:   StatusSuccess,
			Message:  fmt.Sprintf("Found %d of %d matching job run(s).", len(runs), result.Total),
			Runs:     runs,
			Total:    result.Total,
			Page:     page,
			PageSize: pageSize,
		}, nil
	}
}

func lookupError(code int, err error) Output {
	out := Output{Status: StatusError}
	switch code {
	case http.StatusUnauthorized:
		out.Message = "Not authenticated to the data-quality API (HTTP 401)."
	case http.StatusForbidden:
		out.Message = "You do not have permission to search data-quality job runs (HTTP 403)."
	case http.StatusBadRequest:
		out.Message = fmt.Sprintf("The data-quality API rejected the search (HTTP 400): %v", err)
	case 0:
		out.Message = fmt.Sprintf("Failed to search job runs: %v", err)
	default:
		out.Message = fmt.Sprintf("Failed to search job runs (HTTP %d): %v", code, err)
	}
	return out
}
