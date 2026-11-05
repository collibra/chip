// Package apply_latest_rules implements the apply_latest_rules MCP tool: it
// asks Collibra to compile the agent contracts that govern an AI agent into a
// rules file (an enforcement bundle) for the agent's newest version (POST
// /rest/guardianEnforcement/v1/bundles/by/aiAgent/{aiAgentId}/compile). It is
// the same action as Apply latest rules on the agent's Contract tab, behind a
// confirm checkpoint, and it never retries: every request starts a run.
package apply_latest_rules

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

// OutputStatus is the outcome of an apply_latest_rules call.
type OutputStatus string

const (
	// StatusPreview means confirm was not set: nothing was started.
	StatusPreview         OutputStatus = "preview"
	StatusSuccess         OutputStatus = "success"
	StatusNotFound        OutputStatus = "not_found"
	StatusValidationError OutputStatus = "validation_error"
	StatusError           OutputStatus = "error"
)

// Input is the tool's typed input.
type Input struct {
	AIAgentID string `json:"aiAgentId" jsonschema:"Required. UUID of the AI Agent asset (the agent's entry in the Collibra catalog), not the agent's external id. Get it from list_scanned_agents."`
	Confirm   bool   `json:"confirm,omitempty" jsonschema:"Safety checkpoint. false (default) returns a PREVIEW of the agent, its newest version, the governing contracts and the current compilation status, and starts NOTHING. Set true only after the user has clearly agreed to apply the latest rules to this agent."`
}

// Preview is what confirm=true would act on. The only field sent is aiAgentId.
type Preview struct {
	AIAgentID         string                              `json:"aiAgentId" jsonschema:"The agent id that will be sent; the request has no body."`
	AIAgent           *clients.GuardianAgentRef           `json:"aiAgent,omitempty" jsonschema:"The agent: id and name."`
	NewestVersion     *clients.GuardianAgentRef           `json:"newestVersion,omitempty" jsonschema:"The agent's newest version (from its latest imported scan), which the rules will be compiled for. Absent while the agent's first scan is still importing or when its import failed."`
	ScanStatus        string                              `json:"scanStatus,omitempty" jsonschema:"Import status of the agent's latest scan: IN_PROGRESS, COMPLETED or FAILED."`
	Contracts         []clients.GuardianGoverningContract `json:"contracts" jsonschema:"The agent contracts that govern the agent, global first, with the active version each run will use. Empty when no contract governs the agent: a run then finishes without a rules file."`
	CompilationStatus string                              `json:"compilationStatus,omitempty" jsonschema:"Current compilation status: NOT_COMPILED, IN_PROGRESS, FAILED, STALE or UP_TO_DATE."`
	StaleBecause      []string                            `json:"staleBecause,omitempty" jsonschema:"Why the current rules file is out of date, when it is."`
	LatestRunState    string                              `json:"latestRunState,omitempty" jsonschema:"State of the latest run, when there was one: PENDING, RUNNING, COMPLETED or FAILED."`
}

// Output is the typed response.
type Output struct {
	Status     OutputStatus `json:"status" jsonschema:"'preview' when confirm was not set (nothing started; ask the user, then call again with confirm=true); 'success' when Collibra accepted the run; 'not_found' when Guardian has no record of the agent; 'validation_error' for a bad input (nothing called); 'error' when the run was not started or the preview could not be read."`
	Message    string       `json:"message" jsonschema:"Plain-language summary that can be relayed to the customer."`
	Guidance   string       `json:"guidance,omitempty" jsonschema:"What to tell or ask the customer next, on error."`
	ErrorCode  string       `json:"errorCode,omitempty" jsonschema:"Machine-readable error code from Collibra (e.g. 'aiAgentNotFound'), when the request was rejected."`
	HTTPStatus int          `json:"httpStatus,omitempty" jsonschema:"HTTP status of the rejection, when Collibra answered with one."`
	Preview    *Preview     `json:"preview,omitempty" jsonschema:"What the run would act on, returned when confirm=false."`
	State      string       `json:"state,omitempty" jsonschema:"State of the accepted run, on success: usually PENDING. Follow it with get_agent_guardian_status."`
	CreatedOn  string       `json:"createdOn,omitempty" jsonschema:"When Collibra accepted the run (UTC, RFC 3339), on success."`
}

// NewTool returns the registered tool.
func NewTool(collibraClient *http.Client) *chip.Tool[Input, Output] {
	return &chip.Tool[Input, Output]{
		Name:  "apply_latest_rules",
		Title: "Apply Latest Rules",
		Description: "Apply the latest rules to an AI agent: ask Collibra to turn the agent contracts (sets of natural-language rules) that govern the agent into a rules file (an enforcement bundle, the rules compiled for the agent to apply at runtime) for the agent's newest version, together with a report on each rule. It is the same action as 'Apply latest rules' on the agent's Contract tab in Collibra. " +
			"Use it once the agent has been scanned and imported and its contracts are in place, or when get_agent_guardian_status reports the compilation as STALE or FAILED. It does not create or change contracts, and it does not download the rules file (get_agent_rules_file does). " +
			"aiAgentId is the AI Agent asset id, not the external id: get it from list_scanned_agents. " +
			"Built around a confirm checkpoint: confirm=false (default) starts nothing and returns a PREVIEW read from Collibra: the agent, its newest version, the governing contracts with their active versions, and the current compilation status. Show it to the user and ask; call again with confirm=true only after a clear yes. " +
			"With confirm=true it starts one run and returns its state and createdOn. The run takes minutes: follow it with get_agent_guardian_status until compilation leaves IN_PROGRESS, then read get_agent_rules_report. Every confirmed call starts a new run, so do not repeat it to check progress. A run with no usable rules finishes without a rules file; get_agent_guardian_status and get_agent_rules_report show that. " +
			"A not_found status (errorCode aiAgentNotFound) means the agent was never scanned, its scan is not imported yet, or the id is wrong. Permission errors (401, 403) are not transient: do not retry them. " +
			"Example requests: 'apply the latest rules to my agent', 'compile the contracts for this agent', 'update my agent's rules file'. " +
			"Note: requires permission to start compilations (scope aigovernance.guardian.enforcement-write).",
		Handler:     handler(collibraClient),
		Permissions: []string{},
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: false, DestructiveHint: chip.Ptr(false), IdempotentHint: false, OpenWorldHint: chip.Ptr(false)},
	}
}

func handler(collibraClient *http.Client) chip.ToolHandlerFunc[Input, Output] {
	return func(ctx context.Context, input Input) (Output, error) {
		aiAgentID := strings.TrimSpace(input.AIAgentID)
		if _, err := uuid.Parse(aiAgentID); err != nil {
			return Output{
				Status:  StatusValidationError,
				Message: fmt.Sprintf("aiAgentId %q is not a valid UUID. Pass the AI Agent asset's UUID (list_scanned_agents returns it), not the agent's external id. Nothing was started.", input.AIAgentID),
			}, nil
		}

		if !input.Confirm {
			overview, err := clients.GetAgentGuardianOverview(ctx, collibraClient, aiAgentID)
			if err != nil {
				return failure(err, "The preview could not be read from Collibra", false), nil
			}
			return previewOutput(aiAgentID, overview), nil
		}

		run, err := clients.StartAgentRulesCompilation(ctx, collibraClient, aiAgentID)
		if err != nil {
			return failure(err, "The rules could not be applied", true), nil
		}
		return Output{
			Status: StatusSuccess,
			Message: fmt.Sprintf("Collibra accepted the run to apply the latest rules (state %s, accepted %s). It takes minutes; "+
				"follow it with get_agent_guardian_status until compilation leaves IN_PROGRESS, then read get_agent_rules_report.", run.State, run.CreatedOn),
			State:     run.State,
			CreatedOn: run.CreatedOn,
		}, nil
	}
}

func previewOutput(aiAgentID string, overview *clients.AgentGuardianOverview) Output {
	preview := &Preview{
		AIAgentID: aiAgentID,
		AIAgent:   overview.AIAgent,
		Contracts: []clients.GuardianGoverningContract{},
	}
	if overview.AgentScan != nil {
		preview.ScanStatus = overview.AgentScan.Status
		preview.NewestVersion = overview.AgentScan.AIAgentVersion
	}
	if overview.Contracts != nil {
		preview.Contracts = *overview.Contracts
	}
	if overview.Compilation != nil {
		preview.CompilationStatus = overview.Compilation.Status
		preview.StaleBecause = overview.Compilation.StaleBecause
		if overview.Compilation.LatestJob != nil {
			preview.LatestRunState = overview.Compilation.LatestJob.State
		}
	}
	return Output{
		Status: StatusPreview,
		Message: fmt.Sprintf("Preview only: nothing was started. confirm=true will ask Collibra to compile the %d governing contract(s) shown here into a rules file for %s (compilation is %s now). "+
			"Ask the user whether to apply the latest rules, and call again with confirm=true only after a clear yes.",
			len(preview.Contracts), versionClause(preview.NewestVersion), orUnknown(preview.CompilationStatus)),
		Preview: preview,
	}
}

func versionClause(version *clients.GuardianAgentRef) string {
	switch {
	case version == nil:
		return "the agent's newest version (not imported yet)"
	case version.Name != "":
		return fmt.Sprintf("the agent's newest version %q", version.Name)
	}
	return fmt.Sprintf("the agent's newest version %s", version.ID)
}

func orUnknown(s string) string {
	if s == "" {
		return "unknown"
	}
	return s
}

func failure(err error, what string, write bool) Output {
	message, guidance, errorCode, httpStatus, ok := clients.DescribeGuardianError(err)
	if !ok {
		out := Output{Status: StatusError, Message: fmt.Sprintf("%s: %v.", what, err)}
		if write {
			out.Guidance = "This looks like a network problem. It is not known whether a run started: check get_agent_guardian_status before offering to try again, so a second run is not started by accident."
		} else {
			out.Guidance = "This looks like a network problem; nothing was started. Trying once more later is fine."
		}
		return out
	}
	status := StatusError
	if httpStatus == http.StatusNotFound {
		status = StatusNotFound
	}
	if !write || httpStatus < http.StatusInternalServerError {
		message += " Nothing was started."
	}
	return Output{Status: status, Message: message, Guidance: guidance, ErrorCode: errorCode, HTTPStatus: httpStatus}
}
