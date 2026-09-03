// Package get_dq_job_run_profile implements the dq_get_job_run_profile MCP tool —
// read the column-level profiling statistics a Collibra data-quality job run
// produced, by run id.
//
// The flow is a single public GET:
//
//	GET /rest/dq/1.0/jobRuns/{jobRunId}/profile -> one page of per-column profiles
//	                                               (types, counts, min/max/mean,
//	                                               quartiles, top shapes).
//
// This is a pure read: no confirm checkpoint, no writes. Profiling results only
// exist once a run has produced them, so a run that never completed — or a job with
// profiling switched off — comes back with an empty page; that is reported as an
// error explaining why rather than as an empty success. 400/401/403/404/500 and
// transport failures are surfaced as messages with actionable guidance rather than
// Go errors.
package get_dq_job_run_profile

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

// The public profile endpoint pages at 100 columns and refuses more than 500.
const (
	defaultLimit = 100
	maxLimit     = 500
)

// Input is the tool's typed input.
type Input struct {
	RunID  string `json:"run_id" jsonschema:"Required. The id (jobRunId) of the job run whose profile to retrieve."`
	Limit  int    `json:"limit,omitempty" jsonschema:"Optional. Maximum number of column profiles to return (1-500). Defaults to 100."`
	Offset int    `json:"offset,omitempty" jsonschema:"Optional. Index of the first column profile to return (min 0). Defaults to 0. Use with limit to page through a wide table."`
}

// ColumnShape is one observed value shape for a column.
type ColumnShape struct {
	Pattern    string  `json:"pattern" jsonschema:"The shape pattern, using a character-class encoding: '#' for a digit, 'A' for a letter (e.g. '######' for six digits)."`
	Count      int64   `json:"count" jsonschema:"Number of values matching this shape."`
	Percentage float64 `json:"percentage" jsonschema:"Percentage of non-null values matching this shape."`
}

// ColumnProfile is one column's profiling statistics for the run.
type ColumnProfile struct {
	ColumnName   string        `json:"columnName" jsonschema:"Source column name."`
	DefinedType  string        `json:"definedType,omitempty" jsonschema:"Type declared by the source schema (e.g. BIGINT, VARCHAR(255), DECIMAL)."`
	InferredType string        `json:"inferredType,omitempty" jsonschema:"Type inferred from the observed values. A comma-separated list when the column holds a mix of types (e.g. 'String, Int'), which is itself a signal of type inconsistency."`
	ValueCount   int64         `json:"valueCount" jsonschema:"Number of non-null, non-empty values observed."`
	NullCount    int64         `json:"nullCount" jsonschema:"Number of NULL values observed."`
	NullPercent  float64       `json:"nullPercent" jsonschema:"NULL values as a percentage of all rows observed in the column."`
	EmptyCount   int64         `json:"emptyCount" jsonschema:"Number of empty values (e.g. empty strings) observed."`
	EmptyPercent float64       `json:"emptyPercent" jsonschema:"Empty values as a percentage of all rows observed in the column."`
	UniqueCount  int64         `json:"uniqueCount" jsonschema:"Number of distinct values observed (exact or approximate depending on the engine)."`
	Min          string        `json:"min,omitempty" jsonschema:"Minimum value observed. Absent when no values were observed."`
	Max          string        `json:"max,omitempty" jsonschema:"Maximum value observed. Absent when no values were observed."`
	Mean         string        `json:"mean,omitempty" jsonschema:"Arithmetic mean. Numeric columns only."`
	Median       string        `json:"median,omitempty" jsonschema:"Median (50th percentile). Numeric columns only."`
	Q1           string        `json:"q1,omitempty" jsonschema:"First quartile (25th percentile). Numeric columns only."`
	Q3           string        `json:"q3,omitempty" jsonschema:"Third quartile (75th percentile). Numeric columns only."`
	TopShapes    []ColumnShape `json:"topShapes,omitempty" jsonschema:"Top observed value shapes, ordered by descending frequency. Absent when shape analysis did not run for the column."`
}

// ProfileDetail is the page of profiling results returned on success.
type ProfileDetail struct {
	JobRunID   string          `json:"jobRunId"`
	JobName    string          `json:"jobName,omitempty"`
	RunDate    string          `json:"runDate,omitempty"`
	Offset     int64           `json:"offset" jsonschema:"Index of the first column profile in this page."`
	Limit      int64           `json:"limit" jsonschema:"Maximum number of column profiles in this page."`
	Total      *int64          `json:"total,omitempty" jsonschema:"Total number of profiled columns for this run, across all pages."`
	HasMore    bool            `json:"hasMore" jsonschema:"Whether more column profiles remain beyond this page; page through with offset."`
	Columns    []ColumnProfile `json:"columns" jsonschema:"One entry per profiled column, ascending by column name."`
	JobDetails string          `json:"jobDetailsLink,omitempty" jsonschema:"Job Details deep-link path (relative to the Collibra instance URL), when jobName is known."`
}

// Output is the typed response.
type Output struct {
	Status   Status         `json:"status" jsonschema:"'success' when profiling results were returned; 'needs_input' for bad inputs; 'error' for downstream DQ failures or when the run has no profiling results."`
	Message  string         `json:"message" jsonschema:"Human-readable summary."`
	Profile  *ProfileDetail `json:"profile,omitempty" jsonschema:"The run's column-level profiling results, on success."`
	Guidance string         `json:"guidance,omitempty" jsonschema:"On needs_input/error, what to do next."`
}

// NewTool returns the registered tool.
func NewTool(collibraClient *http.Client) *chip.Tool[Input, Output] {
	return &chip.Tool[Input, Output]{
		Name:  "dq_get_job_run_profile",
		Title: "Get Data Quality Job Run Profile",
		Description: "Reads the column-level profiling statistics produced by a single Collibra data-quality job run, " +
			"by its run_id (jobRunId). Per column: the type declared by the source schema and the type actually inferred from the " +
			"values, counts of values/nulls/empties/distinct values (with null and empty also as percentages), min/max/mean, " +
			"quartiles for numeric columns, and the top observed value shapes.\n\n" +
			"Use this to understand a run's data distribution, type inconsistencies (inferredType listing more than one type) and " +
			"format anomalies. Profiling results exist only for runs that produced them, so a run that never completed — or a job " +
			"with profiling switched off — returns an error saying so.\n\n" +
			"Paginated: 100 columns per page by default (max 500); page through a wide table with offset.\n\n" +
			"Example user requests: \"Show me the profile for DQ run <id>\"; \"Which columns in run <id> have nulls?\"; " +
			"\"What formats do the values in run <id> take?\"",
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
				Message:  "Provide the run whose profile to retrieve.",
				Guidance: "Supply run_id — the id (jobRunId) of the job run whose profile to retrieve.",
			}, nil
		}
		limit, out := resolveLimit(input.Limit)
		if out != nil {
			return *out, nil
		}
		if input.Offset < 0 {
			return Output{
				Status:   StatusNeedsInput,
				Message:  "offset must be 0 or greater.",
				Guidance: "Pass offset as the index of the first column profile to return, starting at 0.",
			}, nil
		}

		// Ask for the total so the caller can tell whether paging is needed.
		page, code, err := clients.GetDqJobRunProfile(ctx, collibraClient, runID, limit, input.Offset, true)
		if err != nil {
			return lookupError(code, err, runID), nil
		}
		if len(page.Results) == 0 {
			return noProfileResults(runID, input.Offset), nil
		}

		detail := profileDetail(page, runID, limit)
		return Output{
			Status:  StatusSuccess,
			Message: profileSummary(detail, runID),
			Profile: &detail,
		}, nil
	}
}

// resolveLimit defaults an unset limit and rejects one the API would refuse.
func resolveLimit(limit int) (int, *Output) {
	switch {
	case limit == 0:
		return defaultLimit, nil
	case limit < 0 || limit > maxLimit:
		return 0, &Output{
			Status:   StatusNeedsInput,
			Message:  fmt.Sprintf("limit must be between 1 and %d.", maxLimit),
			Guidance: fmt.Sprintf("Pass limit as the number of column profiles to return per page (1-%d), or omit it for %d.", maxLimit, defaultLimit),
		}
	default:
		return limit, nil
	}
}

func profileDetail(page *clients.DqJobRunProfileResults, runID string, requestedLimit int) ProfileDetail {
	detail := ProfileDetail{
		JobRunID: firstNonEmpty(page.JobRunID, runID),
		JobName:  page.JobName,
		RunDate:  page.RunDateValue(),
		Offset:   page.Offset,
		Limit:    page.Limit,
		Total:    page.Total,
		Columns:  make([]ColumnProfile, 0, len(page.Results)),
	}
	if detail.Limit == 0 {
		detail.Limit = int64(requestedLimit)
	}
	for _, column := range page.Results {
		detail.Columns = append(detail.Columns, columnProfile(column))
	}
	detail.HasMore = hasMore(page, len(page.Results))
	if detail.JobName != "" {
		detail.JobDetails = clients.DqJobDetailsPath(detail.JobName)
	}
	return detail
}

func columnProfile(column clients.DqColumnProfile) ColumnProfile {
	observed := column.ValueCount + column.NullCount + column.EmptyCount
	profile := ColumnProfile{
		ColumnName:   column.ColumnName,
		DefinedType:  column.DefinedType,
		InferredType: column.InferredType,
		ValueCount:   column.ValueCount,
		NullCount:    column.NullCount,
		NullPercent:  percentOf(column.NullCount, observed),
		EmptyCount:   column.EmptyCount,
		EmptyPercent: percentOf(column.EmptyCount, observed),
		UniqueCount:  column.UniqueCount,
		Min:          column.Min,
		Max:          column.Max,
		Mean:         column.Mean,
		Median:       column.Median,
		Q1:           column.Q1,
		Q3:           column.Q3,
	}
	for _, shape := range column.TopShapes {
		profile.TopShapes = append(profile.TopShapes, ColumnShape{
			Pattern: shape.Pattern, Count: shape.Count, Percentage: shape.Percentage,
		})
	}
	return profile
}

// hasMore reports whether columns remain past this page, using total when the API
// returned it and falling back to a full page as the signal when it did not.
func hasMore(page *clients.DqJobRunProfileResults, returned int) bool {
	if page.Total != nil {
		return page.Offset+int64(returned) < *page.Total
	}
	return page.Limit > 0 && int64(returned) >= page.Limit
}

func profileSummary(detail ProfileDetail, runID string) string {
	subject := fmt.Sprintf("run %q", runID)
	if detail.JobName != "" {
		subject = fmt.Sprintf("run %q of job %q", runID, detail.JobName)
	}
	if detail.Total != nil {
		return fmt.Sprintf("Returned profiling statistics for %d of %d column(s) from %s.", len(detail.Columns), *detail.Total, subject)
	}
	return fmt.Sprintf("Returned profiling statistics for %d column(s) from %s.", len(detail.Columns), subject)
}

// noProfileResults explains an empty page: past the end when paging, otherwise a run
// that produced no profile at all.
func noProfileResults(runID string, offset int) Output {
	if offset > 0 {
		return Output{
			Status:   StatusError,
			Message:  fmt.Sprintf("No column profiles for run %q at offset %d.", runID, offset),
			Guidance: "The offset is past the last profiled column. Retry with a smaller offset, or omit it to start from the first column.",
		}
	}
	return Output{
		Status:  StatusError,
		Message: fmt.Sprintf("Run %q has no profiling results.", runID),
		Guidance: "Profiling results exist only once a run has produced them. Either the run did not complete (check its status with " +
			"dq_get_job_run), or profiling is not configured for the job. Confirm the run finished, then retry.",
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
		out.Message = fmt.Sprintf("You do not have permission to view the profile for run %q (HTTP 403).", runID)
		out.Guidance = "You need the Data Quality Job > View permission on the job. Ask an administrator for the Data Quality Editor/Manager role, then retry."
	case http.StatusBadRequest:
		out.Message = fmt.Sprintf("The data-quality API rejected the profile lookup for run %q (HTTP 400): %v", runID, err)
		out.Guidance = fmt.Sprintf("Check that run_id is correct and well-formed and that limit is within 1-%d, then retry.", maxLimit)
	case 0:
		out.Message = fmt.Sprintf("Failed to read the profile for run %q: %v", runID, err)
		out.Guidance = "A network/transport error occurred contacting the data-quality API. Retry."
	default:
		out.Message = fmt.Sprintf("Failed to read the profile for run %q (HTTP %d): %v", runID, code, err)
		out.Guidance = "This is likely a server-side error. Retry shortly; if it persists, contact your Collibra administrator."
	}
	return out
}

func percentOf(count, total int64) float64 {
	if total <= 0 {
		return 0
	}
	return float64(count) / float64(total) * 100
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
