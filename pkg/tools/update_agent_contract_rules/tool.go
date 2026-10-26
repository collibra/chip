// Package update_agent_contract_rules implements the update_agent_contract_rules
// MCP tool: it replaces the rules of an agent contract by pushing a new version
// (PUT /rest/aiGovernance/v1/agentContracts/{id}/rules) as the calling user.
package update_agent_contract_rules

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/collibra/chip/pkg/chip"
	"github.com/collibra/chip/pkg/clients"
	"github.com/collibra/chip/pkg/tools/get_agent_contract_rules"
	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// OutputStatus is the outcome of an update_agent_contract_rules call.
type OutputStatus string

const (
	// StatusPreview means confirm was not set: nothing was sent.
	StatusPreview         OutputStatus = "preview"
	StatusSuccess         OutputStatus = "success"
	StatusNotFound        OutputStatus = "not_found"
	StatusValidationError OutputStatus = "validation_error"
	StatusError           OutputStatus = "error"
)

// Input is the tool's typed input.
type Input struct {
	AgentContractID string   `json:"agentContractId" jsonschema:"Required. UUID of the agent contract to update."`
	Statements      []string `json:"statements" jsonschema:"Required. The FULL ordered set of rules the new version should have (1 to 200, each up to 2000 characters, unique once whitespace is cleaned up). Anything left out is dropped from the active version; unchanged statements keep their rule id. Start from get_agent_contract_rules and change only what the customer asked."`
	Confirm         bool     `json:"confirm,omitempty" jsonschema:"Safety checkpoint. false (default) returns a PREVIEW with the changes against the current active version and sends NOTHING. Set true only after the user has clearly agreed to push exactly these rules."`
}

// Preview shows what a push would change.
type Preview struct {
	CurrentVersionNumber int      `json:"currentVersionNumber" jsonschema:"Active version number right now. If it is not the version the user reviewed, someone else changed the contract: show the difference before pushing."`
	NoChange             bool     `json:"noChange" jsonschema:"true when the statements equal the current active version: pushing would create no new version."`
	Added                []string `json:"added,omitempty" jsonschema:"Statements that are not in the current version."`
	Removed              []string `json:"removed,omitempty" jsonschema:"Statements in the current version that the new set drops."`
	UnchangedCount       int      `json:"unchangedCount" jsonschema:"Statements kept as they are."`
	Statements           []string `json:"statements" jsonschema:"The full ordered set that will be pushed, whitespace cleaned up."`
}

// Output is the typed response.
type Output struct {
	Status        OutputStatus                    `json:"status" jsonschema:"'preview' when confirm was not set (nothing sent); 'success' when Collibra holds the pushed rules (read versionNumber to see whether a new version was created); 'not_found'; 'validation_error' for bad inputs (nothing sent); 'error' when the push was not applied."`
	Message       string                          `json:"message" jsonschema:"Plain-language summary that can be relayed to the customer."`
	Guidance      string                          `json:"guidance,omitempty" jsonschema:"What to tell or ask the customer next, on error."`
	ErrorCode     string                          `json:"errorCode,omitempty" jsonschema:"Machine-readable error code from Collibra, when it rejected the request."`
	HTTPStatus    int                             `json:"httpStatus,omitempty" jsonschema:"HTTP status of the rejection, when Collibra answered with one."`
	Preview       *Preview                        `json:"preview,omitempty" jsonschema:"What would change, returned when confirm=false."`
	NewVersion    *bool                           `json:"newVersionCreated,omitempty" jsonschema:"On success: true when a new version was created, false when the pushed rules matched the active version."`
	ContractID    string                          `json:"agentContractId,omitempty" jsonschema:"The contract, on success."`
	VersionID     string                          `json:"versionId,omitempty" jsonschema:"Id of the active version after the push, on success."`
	VersionNumber int                             `json:"versionNumber,omitempty" jsonschema:"Number of the active version after the push, on success."`
	Rules         []get_agent_contract_rules.Rule `json:"rules,omitempty" jsonschema:"The rules of the active version after the push, with their ids, on success. Compare with what you pushed."`
}

// NewTool returns the registered tool.
func NewTool(collibraClient *http.Client) *chip.Tool[Input, Output] {
	return &chip.Tool[Input, Output]{
		Name:  "update_agent_contract_rules",
		Title: "Update Agent Contract Rules",
		Description: "Replace the rules of an existing Guardian agent contract, as the current user, by pushing a new version. The call carries the FULL ordered set of statements, not a patch: add, reword, reorder or drop rules by sending the complete list you want. " +
			"Workflow: read the current rules with get_agent_contract_rules, change only what the customer asked, then call this with confirm=false to get a PREVIEW of exactly what is added, removed and kept against the current version, show it to the user, and call again with confirm=true only after a clear yes to that exact set. " +
			"Pushing the exact same set as the active version creates no new version (safe to repeat). If someone else changed the contract since you read it, the preview shows a different currentVersionNumber: tell the user and re-confirm. A concurrentAgentContractUpdate error means another push won a race: read again, do not just resend. " +
			"The linked agents never change on a push. Permission and sign-in errors (HTTP 401, 403) are not transient: do not retry, do not look for another route. If the Agent Contracts feature is off, the error says it has to be enabled in AI Command Center. " +
			"Requires update permission on the contract (held by its Owner by default).",
		Handler:     handler(collibraClient),
		Permissions: []string{},
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: false, DestructiveHint: chip.Ptr(false), IdempotentHint: true, OpenWorldHint: chip.Ptr(false)},
	}
}

func handler(collibraClient *http.Client) chip.ToolHandlerFunc[Input, Output] {
	return func(ctx context.Context, input Input) (Output, error) {
		contractID := strings.TrimSpace(input.AgentContractID)
		if _, err := uuid.Parse(contractID); err != nil {
			return validationError(fmt.Sprintf("agentContractId %q is not a valid UUID. list_agent_contracts returns it.", input.AgentContractID)), nil
		}
		statements, problem := clients.NormalizeStatements(input.Statements)
		if problem != "" {
			return validationError(problem), nil
		}

		if !input.Confirm {
			current, err := clients.GetAgentContractRules(ctx, collibraClient, contractID)
			if err != nil {
				return failure(err), nil
			}
			preview := diff(current, statements)
			message := fmt.Sprintf("Preview only: nothing was changed. confirm=true will push %d rule(s) on top of version %d: %d added, %d removed, %d kept. Show the user the changes, ask whether to push, and call again with confirm=true only after a clear yes.",
				len(statements), preview.CurrentVersionNumber, len(preview.Added), len(preview.Removed), preview.UnchangedCount)
			if preview.NoChange {
				message = fmt.Sprintf("Preview only: these rules are identical to version %d, so pushing would create no new version.", preview.CurrentVersionNumber)
			}
			return Output{Status: StatusPreview, Message: message, Preview: preview}, nil
		}

		// Read the active version first so the result can say whether a new version was created.
		before, err := clients.GetAgentContractRules(ctx, collibraClient, contractID)
		if err != nil {
			return failure(err), nil
		}
		after, err := clients.ReplaceAgentContractRules(ctx, collibraClient, contractID, statements)
		if err != nil {
			return failure(err), nil
		}
		created := after.AgentContractVersion.ID != before.AgentContractVersion.ID
		out := get_agent_contract_rules.FromList(after, "")
		message := fmt.Sprintf("Pushed. The contract is now at version %d with %d rule(s).", after.AgentContractVersion.VersionNumber, len(after.Results))
		if !created {
			message = fmt.Sprintf("The pushed rules matched version %d, so no new version was created.", after.AgentContractVersion.VersionNumber)
		}
		return Output{
			Status: StatusSuccess, Message: message, NewVersion: &created,
			ContractID: out.ContractID, VersionID: out.VersionID, VersionNumber: out.VersionNumber, Rules: out.Rules,
		}, nil
	}
}

// diff compares the pushed set with the active version by statement text.
func diff(current *clients.AgentContractRuleList, statements []string) *Preview {
	have := make(map[string]bool, len(current.Results))
	for _, r := range current.Results {
		have[r.Statement] = true
	}
	want := make(map[string]bool, len(statements))
	preview := &Preview{CurrentVersionNumber: current.AgentContractVersion.VersionNumber, Statements: statements}
	for _, s := range statements {
		want[s] = true
		if have[s] {
			preview.UnchangedCount++
		} else {
			preview.Added = append(preview.Added, s)
		}
	}
	for _, r := range current.Results {
		if !want[r.Statement] {
			preview.Removed = append(preview.Removed, r.Statement)
		}
	}
	sameOrder := len(current.Results) == len(statements)
	for i := 0; sameOrder && i < len(statements); i++ {
		sameOrder = current.Results[i].Statement == statements[i]
	}
	preview.NoChange = sameOrder
	return preview
}

func failure(err error) Output {
	message, guidance, code, status := clients.DescribeContractError(err)
	out := Output{Status: StatusError, Message: message, Guidance: guidance, ErrorCode: code, HTTPStatus: status}
	if status == http.StatusNotFound {
		out.Status = StatusNotFound
	}
	return out
}

func validationError(message string) Output {
	return Output{Status: StatusValidationError, Message: message}
}
