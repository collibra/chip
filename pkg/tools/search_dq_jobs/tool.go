// Package search_dq_jobs implements the dq_search_jobs MCP tool — search and list Collibra
// data-quality job definitions with filters and pagination.
//
// This is a thin wrapper over the PUBLIC GET /rest/dq/1.0/jobs (searchJobs) endpoint: it does not
// try to disambiguate or fall back like dq_get_job does — it just returns whatever page of jobs
// matches the given filters, which may be empty.
//
// This is a pure read: no confirm checkpoint, no writes. 400/401/403/500 and transport failures are
// surfaced as messages with actionable guidance rather than Go errors.
package search_dq_jobs

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/collibra/chip/pkg/chip"
	"github.com/collibra/chip/pkg/clients"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// OutputStatus is the overall outcome of a dq_search_jobs call.
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
// accessible job, paginated.
type Input struct {
	Name     string `json:"name,omitempty" jsonschema:"Optional. Fuzzy, case-insensitive job-name filter (substring match). A job, also called a 'dataset', is a saved check on one database table."`
	Schema   string `json:"schema,omitempty" jsonschema:"Optional. Exact schema name to filter by."`
	Table    string `json:"table,omitempty" jsonschema:"Optional. Exact table or view name to filter by."`
	EdgeSite string `json:"edge_site,omitempty" jsonschema:"Optional. Exact edge site name to filter by."`
	JobType  string `json:"job_type,omitempty" jsonschema:"Optional. Filter by job execution type: PUSHDOWN or PULLUP. If omitted, both types are returned."`
	Page     int    `json:"page,omitempty" jsonschema:"Optional. 1-based page number. Defaults to 1."`
	PageSize int    `json:"page_size,omitempty" jsonschema:"Optional. Results per page (1-100). Defaults to 25."`
}

// JobSummary is one matching job.
//
// Note: a job definition has no separate 'id' field in the API — jobName is the unique identifier.
// It also has no 'status' field; only individual RUNS have a status (see dq_search_job_runs).
type JobSummary struct {
	JobName      string `json:"jobName" jsonschema:"The job's unique name — use this to identify the job (dq_get_job, dq_search_job_runs, etc.)."`
	JobType      string `json:"jobType,omitempty" jsonschema:"PUSHDOWN or PULLUP."`
	SchemaName   string `json:"schemaName,omitempty"`
	TableName    string `json:"tableName,omitempty"`
	EdgeSiteName string `json:"edgeSiteName,omitempty"`
}

// Output is the typed response.
type Output struct {
	Status   OutputStatus `json:"status" jsonschema:"'success' when the search ran; 'validation_error' for bad inputs; 'error' for downstream DQ failures."`
	Message  string       `json:"message" jsonschema:"Human-readable summary."`
	Jobs     []JobSummary `json:"jobs,omitempty" jsonschema:"Matching jobs on this page."`
	Total    int64        `json:"total" jsonschema:"Total number of matching jobs across all pages."`
	Page     int          `json:"page" jsonschema:"The 1-based page number returned."`
	PageSize int          `json:"pageSize" jsonschema:"The page size used."`
}

// NewTool returns the registered tool.
func NewTool(collibraClient *http.Client) *chip.Tool[Input, Output] {
	return &chip.Tool[Input, Output]{
		Name:  "dq_search_jobs",
		Title: "Search Data Quality Jobs",
		Description: "Searches and lists Collibra data-quality job definitions (a job, also called a 'dataset', " +
			"is a saved check on ONE database table), with optional filters and pagination. Filter by a fuzzy " +
			"job-name substring, exact schema, table, edge site, and/or job type (PUSHDOWN/PULLUP). Omitting all " +
			"filters returns every accessible job, one page at a time. Each result includes the job's name, type, " +
			"schema, table, and edge site.\n\n" +
			"Note: job definitions have no 'status' field — only individual runs do. Use dq_search_job_runs to " +
			"find runs and their status/history for a job.\n\n" +
			"Example user requests: \"List all PULLUP jobs\"; \"What DQ jobs exist on the sales schema?\"; " +
			"\"Search for jobs on the nyse table.\"",
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

		filters := clients.DqJobSearchFilters{
			JobName:      strings.TrimSpace(input.Name),
			SchemaName:   strings.TrimSpace(input.Schema),
			TableName:    strings.TrimSpace(input.Table),
			EdgeSiteName: strings.TrimSpace(input.EdgeSite),
			JobType:      strings.TrimSpace(input.JobType),
		}
		offset := (page - 1) * pageSize

		result, code, err := clients.SearchDqJobs(ctx, collibraClient, filters, offset, pageSize)
		if err != nil {
			return lookupError(code, err), nil
		}

		jobs := make([]JobSummary, 0, len(result.Results))
		for _, j := range result.Results {
			jobs = append(jobs, JobSummary{
				JobName:      j.JobName,
				JobType:      j.JobType,
				SchemaName:   j.DataLocation.SchemaName,
				TableName:    j.DataLocation.TableName,
				EdgeSiteName: j.DataLocation.EdgeSiteName,
			})
		}

		return Output{
			Status:   StatusSuccess,
			Message:  fmt.Sprintf("Found %d of %d matching job(s).", len(jobs), result.Total),
			Jobs:     jobs,
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
		out.Message = "You do not have permission to search data-quality jobs (HTTP 403)."
	case http.StatusBadRequest:
		out.Message = fmt.Sprintf("The data-quality API rejected the search (HTTP 400): %v", err)
	case 0:
		out.Message = fmt.Sprintf("Failed to search jobs: %v", err)
	default:
		out.Message = fmt.Sprintf("Failed to search jobs (HTTP %d): %v", code, err)
	}
	return out
}
