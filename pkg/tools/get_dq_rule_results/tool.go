// Package get_dq_rule_results implements the get_dq_rule_results MCP tool: it
// reads a rule's per-run results (scores, breaking-record counts, exceptions)
// after a job run.
package get_dq_rule_results

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/collibra/chip/pkg/chip"
	"github.com/collibra/chip/pkg/clients"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// OutputStatus is the overall outcome of a get_dq_rule_results call.
type OutputStatus string

const (
	// StatusSuccess means the results were returned.
	StatusSuccess OutputStatus = "success"
	// StatusValidationError means the inputs failed validation before any read.
	StatusValidationError OutputStatus = "validation_error"
	// StatusError means the results could not be read due to a downstream error.
	StatusError OutputStatus = "error"
)

const (
	sortOrderAsc  = "ASC"
	sortOrderDesc = "DESC"

	defaultLimit = 10
)

// Input is the tool's typed input.
type Input struct {
	JobName   string `json:"jobName" jsonschema:"Required. Name of the data quality job the rule is attached to (a job, also called a 'dataset', is a saved check on one database table)."`
	RuleName  string `json:"ruleName" jsonschema:"Required. Name of the rule (the check; Collibra calls it a 'monitor') whose results to read."`
	Offset    int    `json:"offset,omitempty" jsonschema:"Optional. Pagination offset (min 0). Defaults to 0."`
	Limit     int    `json:"limit,omitempty" jsonschema:"Optional. Max number of run-result entries to return (min 1). Defaults to 10."`
	SortOrder string `json:"sortOrder,omitempty" jsonschema:"Optional. Order results by run date: 'DESC' (newest first, the default) or 'ASC'."`
}

// ResultEntry is one per-run result for the rule.
type ResultEntry struct {
	RunDate         int64   `json:"runDate" jsonschema:"Run date as epoch milliseconds."`
	RuleStatus      string  `json:"ruleStatus,omitempty" jsonschema:"Outcome for this run, e.g. PASSING, BREAKING, or EXCEPTION."`
	PassFail        bool    `json:"passFail" jsonschema:"Whether the rule passed for this run."`
	Score           int     `json:"score" jsonschema:"Rule score for this run (0-100)."`
	TotalCount      float64 `json:"totalCount" jsonschema:"Total records evaluated."`
	BreakingRecords float64 `json:"breakingRecords" jsonschema:"Number of breaking (failing) records for this run."`
	PassingRecords  float64 `json:"passingRecords" jsonschema:"Number of passing records."`
	BreakMsg        string  `json:"breakMsg,omitempty" jsonschema:"Break message, when the rule broke."`
	Exception       string  `json:"exception,omitempty" jsonschema:"Exception message, when the run errored."`
}

// Output is the typed response.
type Output struct {
	Status   OutputStatus  `json:"status" jsonschema:"'success' when results were returned; 'validation_error' for bad inputs; 'error' for downstream DQ failures."`
	Message  string        `json:"message" jsonschema:"Human-readable summary."`
	RuleName string        `json:"ruleName,omitempty" jsonschema:"The rule name, on success."`
	RuleType string        `json:"ruleType,omitempty" jsonschema:"The rule type, on success."`
	Results  []ResultEntry `json:"results,omitempty" jsonschema:"Per-run result entries, newest first by default."`
	Total    int64         `json:"total" jsonschema:"Total number of result entries available (for pagination)."`
}

// NewTool returns the registered tool.
func NewTool(collibraClient *http.Client) *chip.Tool[Input, Output] {
	return &chip.Tool[Input, Output]{
		Name:  "get_data_quality_rule_results",
		Title: "Get Data Quality Rule Results",
		Description: "Read a data quality rule's (a check on a table's data; Collibra calls it a 'monitor') results for each run of the job — the 0-100 score, the counts of breaking (failing) and passing records, " +
			"pass/fail status and any exception for each run. Paginated (offset/limit), newest first by default.",
		Handler:     handler(collibraClient),
		Permissions: []string{},
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}
}

func handler(collibraClient *http.Client) chip.ToolHandlerFunc[Input, Output] {
	return func(ctx context.Context, input Input) (Output, error) {
		if strings.TrimSpace(input.JobName) == "" {
			return Output{Status: StatusValidationError, Message: "jobName is required."}, nil
		}
		if strings.TrimSpace(input.RuleName) == "" {
			return Output{Status: StatusValidationError, Message: "ruleName is required."}, nil
		}
		sortOrder, out := resolveSortOrder(input.SortOrder)
		if out != nil {
			return *out, nil
		}
		if input.Offset < 0 {
			return Output{Status: StatusValidationError, Message: "offset must be >= 0."}, nil
		}
		limit := input.Limit
		if limit == 0 {
			limit = defaultLimit
		}
		if limit < 1 {
			return Output{Status: StatusValidationError, Message: "limit must be >= 1."}, nil
		}

		res, err := clients.GetDQRuleResults(ctx, collibraClient, strings.TrimSpace(input.JobName), strings.TrimSpace(input.RuleName), input.Offset, limit, sortOrder)
		if err != nil {
			return Output{Status: StatusError, Message: fmt.Sprintf("Could not read rule results: %v", err)}, nil
		}

		entries := make([]ResultEntry, 0, len(res.Results))
		for _, r := range res.Results {
			entries = append(entries, ResultEntry{
				RunDate:         r.RunDate,
				RuleStatus:      r.RuleStatus,
				PassFail:        r.PassFail,
				Score:           r.Score,
				TotalCount:      r.TotalCount,
				BreakingRecords: r.BreakingRecords,
				PassingRecords:  r.PassingRecords,
				BreakMsg:        r.BreakMsg,
				Exception:       r.Exception,
			})
		}

		return Output{
			Status:   StatusSuccess,
			Message:  fmt.Sprintf("Returned %d of %d result(s) for rule %q on job %q.", len(entries), res.Total, res.RuleName, res.Dataset),
			RuleName: res.RuleName,
			RuleType: res.RuleType,
			Results:  entries,
			Total:    res.Total,
		}, nil
	}
}

// resolveSortOrder defaults an empty sortOrder to DESC and rejects anything else.
func resolveSortOrder(sortOrder string) (string, *Output) {
	switch strings.ToUpper(strings.TrimSpace(sortOrder)) {
	case "", sortOrderDesc:
		return sortOrderDesc, nil
	case sortOrderAsc:
		return sortOrderAsc, nil
	default:
		return "", &Output{
			Status:  StatusValidationError,
			Message: fmt.Sprintf("sortOrder %q is invalid. Use %q or %q.", sortOrder, sortOrderAsc, sortOrderDesc),
		}
	}
}
