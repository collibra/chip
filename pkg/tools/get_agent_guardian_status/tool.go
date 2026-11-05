// Package get_agent_guardian_status implements the get_agent_guardian_status
// MCP tool: one answer to "where is this agent in its Guardian setup?", read
// from the Guardian Overview API and passed through with its field names.
package get_agent_guardian_status

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/collibra/chip/pkg/chip"
	"github.com/collibra/chip/pkg/clients"
	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// OutputStatus is the outcome of a get_agent_guardian_status call.
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

// Output is the typed response. The blocks are Collibra's, unchanged.
type Output struct {
	Status       OutputStatus                         `json:"status" jsonschema:"'success'; 'not_found' when Guardian has no record of the agent (errorCode aiAgentNotFound); 'validation_error' for a bad input (nothing was called); 'error' when Collibra could not answer."`
	Message      string                               `json:"message" jsonschema:"Plain-language summary that can be relayed to the customer."`
	Guidance     string                               `json:"guidance,omitempty" jsonschema:"What to tell or ask the customer next, on error."`
	ErrorCode    string                               `json:"errorCode,omitempty" jsonschema:"Machine-readable error code from Collibra (e.g. 'aiAgentNotFound', 'accessDenied', 'agentContractsUnavailable'), when the request was rejected."`
	HTTPStatus   int                                  `json:"httpStatus,omitempty" jsonschema:"HTTP status of the rejection, when Collibra answered with one."`
	AIAgent      *clients.GuardianAgentRef            `json:"aiAgent,omitempty" jsonschema:"The agent: id and name."`
	AgentScan    *clients.GuardianAgentScanOverview   `json:"scan,omitempty" jsonschema:"The agent's latest scan (the newest scanned state of the agent) and its import into the catalog. status: IN_PROGRESS (still importing), COMPLETED (aiAgentVersion is the newest version), FAILED (failureMessage says why). lastReceivedOn is when the agent was last scanned."`
	Contracts    *[]clients.GuardianGoverningContract `json:"contracts,omitempty" jsonschema:"The agent contracts (sets of natural-language rules) that govern the agent, global first, each with its layer (GLOBAL or SPECIALIZED) and activeVersionId (its latest published version). Empty when no contract governs the agent."`
	Compilation  *clients.GuardianCompilationOverview `json:"compilation,omitempty" jsonschema:"Whether the agent's rules file (enforcement bundle, the rules compiled for the agent to apply at runtime) is current. status: NOT_COMPILED, IN_PROGRESS, FAILED, STALE (staleBecause says why: CONTRACT_VERSION_CHANGED, GOVERNING_SET_CHANGED, SCAN_CHANGED) or UP_TO_DATE. latestJob is the latest run (state PENDING, RUNNING, COMPLETED or FAILED, with failureMessage on failure; bundleId when it made a rules file); latestBundle is the newest rules file."`
	Enforcement  *clients.GuardianEnforcementOverview `json:"enforcement,omitempty" jsonschema:"What the agent's activity shows it runs. status: NOT_OBSERVED (no activity seen yet), UP_TO_DATE, or STALE (the deployment does not run the newest rules file). observedBundle is the rules file seen in the latest issue."`
	Collection   *clients.GuardianCollectionOverview  `json:"collection,omitempty" jsonschema:"Whether Collibra reads the agent's activity. status: OK, NOT_COLLECTED (reason: NO_CONNECTION, SOURCE_NOT_CONNECTED or AGENT_NOT_FOUND), FAILING or LAGGING. Absent for an agent with no rules file yet, or whose platform's activity Collibra does not collect."`
	Links        *Links                               `json:"links,omitempty" jsonschema:"Ready-made links to the agent's pages in Collibra, to give the customer as clickable links. Use them as they are; never build a link yourself. Absent when the Collibra address is not known."`
	AbsentBlocks []string                             `json:"absentBlocks,omitempty" jsonschema:"The blocks Collibra did not return. An older Collibra may not send every block; collection is also absent for an agent that has no rules file yet."`
}

// NewTool returns the registered tool.
func NewTool(collibraClient *http.Client) *chip.Tool[Input, Output] {
	return &chip.Tool[Input, Output]{
		Name:  "get_agent_guardian_status",
		Title: "Get Agent Guardian Status",
		Description: "Read where one AI agent stands in its Guardian setup, in one call. Guardian is Collibra's runtime governance for AI agents: an agent is scanned (a description of its sub-agents, tools and models), agent contracts (sets of natural-language rules) govern it, Collibra compiles those rules into a rules file (an enforcement bundle) for the agent's newest version, the agent runs with that file, and Collibra reads the agent's activity. " +
			"This tool returns each of those stages as Collibra reports it: aiAgent, scan (import status and newest version), contracts (governing contracts with their active versions), compilation (NOT_COMPILED, IN_PROGRESS, FAILED, STALE with staleBecause, or UP_TO_DATE, plus the latest run and rules file), enforcement (whether the deployment runs the newest rules file) and collection (whether Collibra reads the agent's activity, and if not, why). " +
			"Use it to decide the next setup step and to follow a run started by apply_latest_rules until compilation leaves IN_PROGRESS. Use get_agent_rules_report for what happened to each rule, and get_agent_rules_file to download the rules file. " +
			"aiAgentId is the AI Agent asset id, not the external id: get it from list_scanned_agents. A block an older Collibra does not send is left out and named in absentBlocks. " +
			"A not_found status (errorCode aiAgentNotFound) means Guardian has no record of the agent: its scan may still be importing, or the id is wrong. A 503 (agentContractsUnavailable) is temporary. Permission errors (401, 403) are not: do not retry them. It only reads. " +
			"Example questions: 'Is Guardian set up for my agent?', 'Did the rules finish applying?', 'Is Collibra reading my agent's activity?', 'what's the status of my agent?'. " +
			"Note: requires permission to view the agent in Collibra (scope aigovernance.guardian.overview-read).",
		Handler:     handler(collibraClient),
		Permissions: []string{},
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, DestructiveHint: chip.Ptr(false), IdempotentHint: true, OpenWorldHint: chip.Ptr(false)},
	}
}

// Links are the pages in Collibra the customer can open for this agent.
type Links struct {
	Agent          string         `json:"agent,omitempty" jsonschema:"The AI Agent's page."`
	AgentVersion   string         `json:"agentVersion,omitempty" jsonschema:"The page of the agent's newest version."`
	ContractTab    string         `json:"contractTab,omitempty" jsonschema:"The agent's Contract tab: the rules that apply and the latest rules file."`
	GuardianTab    string         `json:"guardianTab,omitempty" jsonschema:"The agent's Guardian tab: the issues Guardian recorded."`
	Contracts      []ContractLink `json:"contracts,omitempty" jsonschema:"One link per agent contract that governs the agent."`
	AllContracts   string         `json:"allContracts,omitempty" jsonschema:"The page listing every agent contract."`
	GuardianAgents string         `json:"guardianAgents,omitempty" jsonschema:"The page listing every agent Guardian governs."`
}

// ContractLink is the page of one agent contract.
type ContractLink struct {
	Name string `json:"name"`
	URL  string `json:"url"`
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

		overview, err := clients.GetAgentGuardianOverview(ctx, collibraClient, aiAgentID)
		if err != nil {
			return failure(err), nil
		}
		out := success(overview)
		if host, ok := chip.GetCollibraHost(ctx); ok && strings.TrimSpace(host) != "" {
			out.Links = links(strings.TrimSuffix(strings.TrimSpace(host), "/"), aiAgentID, overview)
		}
		return out, nil
	}
}

func success(overview *clients.AgentGuardianOverview) Output {
	out := Output{
		Status:       StatusSuccess,
		AIAgent:      overview.AIAgent,
		AgentScan:    overview.AgentScan,
		Contracts:    overview.Contracts,
		Compilation:  compilation(overview.Compilation),
		Enforcement:  overview.Enforcement,
		Collection:   overview.Collection,
		AbsentBlocks: absentBlocks(overview),
	}
	out.Message = summary(overview)
	if len(out.AbsentBlocks) > 0 {
		out.Message += fmt.Sprintf(" Collibra did not return: %s.", strings.Join(out.AbsentBlocks, ", "))
	}
	return out
}

// compilation names the scan reason in the tool's own words, so the reason a
// rules file is out of date reads the same as the rest of the output.
func compilation(c *clients.GuardianCompilationOverview) *clients.GuardianCompilationOverview {
	if c == nil || len(c.StaleBecause) == 0 {
		return c
	}
	out := *c
	out.StaleBecause = make([]string, len(c.StaleBecause))
	for i, reason := range c.StaleBecause {
		if reason == "FINGERPRINT_CHANGED" {
			reason = "SCAN_CHANGED"
		}
		out.StaleBecause[i] = reason
	}
	return &out
}

func links(host, aiAgentID string, overview *clients.AgentGuardianOverview) *Links {
	l := &Links{
		Agent:          host + "/asset/" + aiAgentID,
		ContractTab:    host + "/asset/" + aiAgentID + "?tabbar=Contract",
		GuardianTab:    host + "/asset/" + aiAgentID + "?tabbar=Guardian",
		AllContracts:   host + "/ai-governance/agent-contracts",
		GuardianAgents: host + "/ai-governance/guardian-agents",
	}
	if overview.AgentScan != nil && overview.AgentScan.AIAgentVersion != nil && overview.AgentScan.AIAgentVersion.ID != "" {
		l.AgentVersion = host + "/asset/" + overview.AgentScan.AIAgentVersion.ID
	}
	if overview.Contracts != nil {
		for _, c := range *overview.Contracts {
			l.Contracts = append(l.Contracts, ContractLink{Name: c.Name, URL: host + "/ai-governance/agent-contracts/" + c.ID})
		}
	}
	return l
}

func absentBlocks(overview *clients.AgentGuardianOverview) []string {
	var absent []string
	for _, block := range []struct {
		name    string
		present bool
	}{
		{"aiAgent", overview.AIAgent != nil},
		{"scan", overview.AgentScan != nil},
		{"contracts", overview.Contracts != nil},
		{"compilation", overview.Compilation != nil},
		{"enforcement", overview.Enforcement != nil},
		{"collection", overview.Collection != nil},
	} {
		if !block.present {
			absent = append(absent, block.name)
		}
	}
	return absent
}

func summary(overview *clients.AgentGuardianOverview) string {
	var parts []string
	if overview.AgentScan != nil {
		parts = append(parts, "scan "+overview.AgentScan.Status)
	}
	if overview.Contracts != nil {
		parts = append(parts, fmt.Sprintf("%d governing contract(s)", len(*overview.Contracts)))
	}
	if overview.Compilation != nil {
		parts = append(parts, "compilation "+overview.Compilation.Status)
	}
	if overview.Enforcement != nil {
		parts = append(parts, "enforcement "+overview.Enforcement.Status)
	}
	if overview.Collection != nil {
		parts = append(parts, "collection "+overview.Collection.Status)
	}
	if len(parts) == 0 {
		return "Collibra returned no Guardian status for this agent."
	}
	return "Guardian status of the agent: " + strings.Join(parts, ", ") + "."
}

func failure(err error) Output {
	message, guidance, errorCode, httpStatus, ok := clients.DescribeGuardianError(err)
	if !ok {
		return Output{
			Status:   StatusError,
			Message:  fmt.Sprintf("The agent's Guardian status could not be read from Collibra: %v.", err),
			Guidance: "This looks like a network problem. Tell the customer the lookup did not work; trying once more later is fine.",
		}
	}
	status := StatusError
	if httpStatus == http.StatusNotFound {
		status = StatusNotFound
	}
	return Output{Status: status, Message: message, Guidance: guidance, ErrorCode: errorCode, HTTPStatus: httpStatus}
}
