// Package find_dq_rules implements the find_dq_rules MCP tool: it searches
// existing data quality rules (monitors) across jobs, primarily to detect rules
// already present on a target column before creating a new one.
package find_dq_rules

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/collibra/chip/pkg/chip"
	"github.com/collibra/chip/pkg/clients"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// OutputStatus is the overall outcome of a find_dq_rules call.
type OutputStatus string

const (
	// StatusSuccess means the search ran.
	StatusSuccess OutputStatus = "success"
	// StatusValidationError means the inputs failed validation before any read.
	StatusValidationError OutputStatus = "validation_error"
	// StatusError means the search failed due to a downstream DQ error.
	StatusError OutputStatus = "error"
)

const defaultLimit = 25

// DQ monitor filterable fields and operators used by this tool.
const (
	fieldJobName     = "JOB_NAME"
	fieldColumnName  = "COLUMN_NAME"
	fieldMonitorName = "MONITOR_NAME"
	opEquals         = "EQUALS"
	opContains       = "CONTAINS"
)

// Input is the tool's typed input. At least one filter should be provided; for
// duplicate detection, set jobName and columnName.
type Input struct {
	JobName      string `json:"jobName,omitempty" jsonschema:"Optional. Exact job (dataset) name to scope the search to."`
	ColumnName   string `json:"columnName,omitempty" jsonschema:"Optional. Exact column name — combine with jobName to find existing rules on a specific column (duplicate detection)."`
	NameContains string `json:"nameContains,omitempty" jsonschema:"Optional. Substring match on the rule (monitor) name."`
	Offset       int    `json:"offset,omitempty" jsonschema:"Optional. Pagination offset (min 0). Defaults to 0."`
	Limit        int    `json:"limit,omitempty" jsonschema:"Optional. Max rules to return (1-100). Defaults to 25."`
}

// Rule is one matching rule (monitor).
type Rule struct {
	MonitorName   string   `json:"monitorName"`
	JobName       string   `json:"jobName"`
	ColumnName    string   `json:"columnName,omitempty"`
	MonitorType   string   `json:"monitorType,omitempty"`
	MonitorStatus string   `json:"monitorStatus,omitempty"`
	Dimensions    []string `json:"dimensions,omitempty"`
	RuleQuery     string   `json:"ruleQuery,omitempty"`
	FilterQuery   string   `json:"filterQuery,omitempty"`
}

// Output is the typed response.
type Output struct {
	Status  OutputStatus `json:"status" jsonschema:"'success' when the search ran; 'validation_error' for bad inputs; 'error' for downstream DQ failures."`
	Message string       `json:"message" jsonschema:"Human-readable summary."`
	Rules   []Rule       `json:"rules,omitempty" jsonschema:"Matching rules."`
	Total   int64        `json:"total" jsonschema:"Total number of matching rules (for pagination)."`
}

// NewTool returns the registered tool.
func NewTool(collibraClient *http.Client) *chip.Tool[Input, Output] {
	return &chip.Tool[Input, Output]{
		Name:  "find_dq_rules",
		Title: "Find Data Quality Rules",
		Description: "Search existing data quality rules (monitors) across jobs. Filter by exact jobName and/or " +
			"columnName (combine both to detect rules already on a target column before creating a new one), or by " +
			"a rule-name substring. Returns each rule's job, column, type, status and SQL. Paginated (offset/limit).",
		Handler:     handler(collibraClient),
		Permissions: []string{},
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}
}

func handler(collibraClient *http.Client) chip.ToolHandlerFunc[Input, Output] {
	return func(ctx context.Context, input Input) (Output, error) {
		if input.Offset < 0 {
			return Output{Status: StatusValidationError, Message: "offset must be >= 0."}, nil
		}
		limit := input.Limit
		if limit == 0 {
			limit = defaultLimit
		}
		if limit < 1 || limit > 100 {
			return Output{Status: StatusValidationError, Message: "limit must be between 1 and 100."}, nil
		}

		var filters []clients.DQMonitorFilter
		if s := strings.TrimSpace(input.JobName); s != "" {
			filters = append(filters, clients.DQMonitorFilter{Field: fieldJobName, Operator: opEquals, Values: []string{s}})
		}
		if s := strings.TrimSpace(input.ColumnName); s != "" {
			filters = append(filters, clients.DQMonitorFilter{Field: fieldColumnName, Operator: opEquals, Values: []string{s}})
		}
		if s := strings.TrimSpace(input.NameContains); s != "" {
			filters = append(filters, clients.DQMonitorFilter{Field: fieldMonitorName, Operator: opContains, Values: []string{s}})
		}

		res, err := clients.FindDQRules(ctx, collibraClient, filters, input.Offset, limit)
		if err != nil {
			return Output{Status: StatusError, Message: fmt.Sprintf("Could not search rules: %v", err)}, nil
		}

		rules := make([]Rule, 0, len(res.Results))
		for _, m := range res.Results {
			rules = append(rules, Rule{
				MonitorName:   m.MonitorName,
				JobName:       m.JobName,
				ColumnName:    m.ColumnName,
				MonitorType:   m.MonitorType,
				MonitorStatus: m.MonitorStatus,
				Dimensions:    m.Dimensions,
				RuleQuery:     m.RuleQuery,
				FilterQuery:   m.FilterQuery,
			})
		}

		return Output{
			Status:  StatusSuccess,
			Message: fmt.Sprintf("Found %d of %d matching rule(s).", len(rules), res.Total),
			Rules:   rules,
			Total:   res.Total,
		}, nil
	}
}
