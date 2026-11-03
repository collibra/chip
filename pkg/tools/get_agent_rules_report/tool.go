// Package get_agent_rules_report implements the get_agent_rules_report MCP
// tool: what happened to each agent contract rule in the latest run that
// compiled an AI agent's rules file, read from the Guardian Enforcement API.
// The guard detail of each rule is left out on purpose.
package get_agent_rules_report

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/collibra/chip/pkg/chip"
	"github.com/collibra/chip/pkg/clients"
	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// OutputStatus is the outcome of a get_agent_rules_report call.
type OutputStatus string

const (
	StatusSuccess         OutputStatus = "success"
	StatusNotFound        OutputStatus = "not_found"
	StatusValidationError OutputStatus = "validation_error"
	StatusError           OutputStatus = "error"
)

// Input is the tool's typed input.
type Input struct {
	AIAgentID string `json:"aiAgentId" jsonschema:"Required. UUID of the AI Agent asset (the agent's entry in the Collibra catalog), not the agent's external id. Get it from list_scanned_agents."`
}

// OutcomeCount is how many rules got one outcome.
type OutcomeCount struct {
	Outcome string `json:"outcome" jsonschema:"The outcome, e.g. 'bound'."`
	Count   int    `json:"count" jsonschema:"How many rules got it."`
}

// Output is the typed response.
type Output struct {
	Status         OutputStatus                   `json:"status" jsonschema:"'success'; 'not_found' when there is no report yet (errorCode compilationReportNotFound) or Guardian has no record of the agent (aiAgentNotFound); 'validation_error' for a bad input (nothing called); 'error' when Collibra could not answer."`
	Message        string                         `json:"message" jsonschema:"Plain-language summary that can be relayed to the customer."`
	Guidance       string                         `json:"guidance,omitempty" jsonschema:"What to tell or ask the customer next, on error or not_found."`
	ErrorCode      string                         `json:"errorCode,omitempty" jsonschema:"Machine-readable error code from Collibra (e.g. 'compilationReportNotFound'), when the request was rejected."`
	HTTPStatus     int                            `json:"httpStatus,omitempty" jsonschema:"HTTP status of the rejection, when Collibra answered with one."`
	BundleID       string                         `json:"bundleId,omitempty" jsonschema:"Id of the rules file (enforcement bundle) the run made. Absent when the run made no rules file, for example because no rule could be enforced."`
	AgentVersionID string                         `json:"agentVersionId,omitempty" jsonschema:"The AI Agent Version the run compiled for."`
	StartedAt      string                         `json:"startedAt,omitempty" jsonschema:"When the run began (UTC)."`
	FinishedAt     string                         `json:"finishedAt,omitempty" jsonschema:"When the run finished (UTC)."`
	Rules          []clients.AgentRulesReportRule `json:"rules,omitempty" jsonschema:"Each contract rule the run evaluated: ruleId, agentContractId, agentContractVersionId, agentContractLayer (GLOBAL or SPECIALIZED), statement (the rule text as compiled), outcome and reason (why it got that outcome; empty when none is needed)."`
	OutcomeCounts  []OutcomeCount                 `json:"outcomeCounts,omitempty" jsonschema:"How many rules got each outcome, most frequent first."`
}

// NewTool returns the registered tool.
func NewTool(collibraClient *http.Client) *chip.Tool[Input, Output] {
	return &chip.Tool[Input, Output]{
		Name:  "get_agent_rules_report",
		Title: "Get Agent Rules Report",
		Description: "Read what happened to each rule the last time Collibra compiled an AI agent's agent contracts (sets of natural-language rules) into its rules file (an enforcement bundle, the rules compiled for the agent to apply at runtime). " +
			"For every rule it returns the rule text, the contract and contract version it came from, the contract's layer (GLOBAL or SPECIALIZED), an outcome and the reason, plus a count per outcome. " +
			"Outcomes: bound (enforced), partially_bound (partly enforced), not_applicable (does not apply to this agent), no_enforcement_required, unbindable, ambiguous, inexpressible, unsupported_pattern, needs_classifier, unbound, no_model_answer; new values may appear. " +
			"bundleId is absent when the run made no rules file. " +
			"Use it after a run started by apply_latest_rules has finished (get_agent_guardian_status shows compilation no longer IN_PROGRESS), to explain to the customer which rules are enforced and why others are not. It does not start a run; it does not return the rules file (get_agent_rules_file does). " +
			"aiAgentId is the AI Agent asset id, not the external id: get it from list_scanned_agents. " +
			"A not_found status with errorCode compilationReportNotFound is an answer: the rules were never applied for the agent's newest version, or the latest report is not imported yet. Permission errors (401, 403) are not transient: do not retry them. It only reads. " +
			"Example questions: 'which of my rules are enforced?', 'why wasn't this rule applied to my agent?', 'show me the rules report', 'how did the last compile go?'. " +
			"Note: requires permission to read Guardian enforcement (scope aigovernance.guardian.enforcement-read).",
		Handler:     handler(collibraClient),
		Permissions: []string{},
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, DestructiveHint: chip.Ptr(false), IdempotentHint: true, OpenWorldHint: chip.Ptr(false)},
	}
}

func handler(collibraClient *http.Client) chip.ToolHandlerFunc[Input, Output] {
	return func(ctx context.Context, input Input) (Output, error) {
		aiAgentID := strings.TrimSpace(input.AIAgentID)
		if _, err := uuid.Parse(aiAgentID); err != nil {
			return Output{
				Status:  StatusValidationError,
				Message: fmt.Sprintf("aiAgentId %q is not a valid UUID. Pass the AI Agent asset's UUID (list_scanned_agents returns it), not the agent's external id.", input.AIAgentID),
			}, nil
		}

		report, err := clients.GetAgentRulesReport(ctx, collibraClient, aiAgentID)
		if err != nil {
			return failure(err), nil
		}
		return success(report), nil
	}
}

func success(report *clients.AgentRulesReport) Output {
	counts := countOutcomes(report.Rules)
	out := Output{
		Status:         StatusSuccess,
		BundleID:       report.BundleID,
		AgentVersionID: report.AgentVersionID,
		StartedAt:      report.StartedAt,
		FinishedAt:     report.FinishedAt,
		Rules:          report.Rules,
		OutcomeCounts:  counts,
	}
	parts := make([]string, 0, len(counts))
	for _, c := range counts {
		parts = append(parts, fmt.Sprintf("%d %s", c.Count, c.Outcome))
	}
	out.Message = fmt.Sprintf("The run that finished %s evaluated %d rule(s)", report.FinishedAt, len(report.Rules))
	if len(parts) > 0 {
		out.Message += ": " + strings.Join(parts, ", ")
	}
	if report.BundleID == "" {
		out.Message += ". It made no rules file."
	} else {
		out.Message += "."
	}
	return out
}

// countOutcomes counts the rules per outcome, most frequent first and then by
// name, so the order is stable.
func countOutcomes(rules []clients.AgentRulesReportRule) []OutcomeCount {
	byOutcome := make(map[string]int)
	for _, rule := range rules {
		byOutcome[rule.Outcome]++
	}
	counts := make([]OutcomeCount, 0, len(byOutcome))
	for outcome, count := range byOutcome {
		counts = append(counts, OutcomeCount{Outcome: outcome, Count: count})
	}
	sort.Slice(counts, func(i, j int) bool {
		if counts[i].Count != counts[j].Count {
			return counts[i].Count > counts[j].Count
		}
		return counts[i].Outcome < counts[j].Outcome
	})
	return counts
}

func failure(err error) Output {
	message, guidance, errorCode, httpStatus, ok := clients.DescribeGuardianError(err)
	if !ok {
		return Output{
			Status:   StatusError,
			Message:  fmt.Sprintf("The rules report could not be read from Collibra: %v.", err),
			Guidance: "This looks like a network problem. Tell the customer the lookup did not work; trying once more later is fine.",
		}
	}
	status := StatusError
	if httpStatus == http.StatusNotFound {
		status = StatusNotFound
	}
	return Output{Status: status, Message: message, Guidance: guidance, ErrorCode: errorCode, HTTPStatus: httpStatus}
}
