// Package list_agent_contracts implements the list_agent_contracts MCP tool:
// it lists agent contracts from the Agent Contracts API (GET
// /rest/aiGovernance/v1/agentContracts) as the calling user.
package list_agent_contracts

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

// OutputStatus is the outcome of a list_agent_contracts call.
type OutputStatus string

const (
	StatusSuccess         OutputStatus = "success"
	StatusValidationError OutputStatus = "validation_error"
	StatusError           OutputStatus = "error"
)

// Input is the tool's typed input.
type Input struct {
	Name         string `json:"name,omitempty" jsonschema:"Optional. Only contracts whose name contains this text (case-insensitive)."`
	AgentID      string `json:"agentId,omitempty" jsonschema:"Optional. UUID of an AI Agent asset. Returns every contract that governs that agent (the global contract first, then the specialised ones linked to it); paging and sorting are then ignored. Fails with 403 if the user cannot view one of them."`
	SortField    string `json:"sortField,omitempty" jsonschema:"Optional. NAME (default), RULE_COUNT or LAST_MODIFIED_ON. The global contract always comes first."`
	SortOrder    string `json:"sortOrder,omitempty" jsonschema:"Optional. ASC (default) or DESC."`
	Offset       int    `json:"offset,omitempty" jsonschema:"Optional. Index of the first result; default 0."`
	Limit        int    `json:"limit,omitempty" jsonschema:"Optional. Page size; default 100, maximum 500."`
	IncludeTotal bool   `json:"includeTotal,omitempty" jsonschema:"Optional. true also returns the total number of matches."`
}

// Contract is one agent contract in the answer.
type Contract struct {
	ID                  string `json:"id" jsonschema:"Agent contract id (UUID)."`
	Name                string `json:"name" jsonschema:"Contract name (also its asset name)."`
	Layer               string `json:"layer" jsonschema:"GLOBAL (governs every agent, at most one) or SPECIALIZED (governs only the agents linked to it)."`
	ActiveVersionID     string `json:"activeVersionId" jsonschema:"Id of the active (latest) version; pass it to get_agent_contract_rules as versionId to read that exact version."`
	ActiveVersionNumber int    `json:"activeVersionNumber" jsonschema:"Number of the active version."`
	RuleCount           int    `json:"ruleCount" jsonschema:"Number of rules in the active version."`
	GovernedAgentCount  *int   `json:"governedAgentCount,omitempty" jsonschema:"Number of AI agents the contract governs (only those the user can view)."`
	LastModifiedOn      string `json:"lastModifiedOn" jsonschema:"When the active version was created (UTC)."`
	LastModifiedBy      string `json:"lastModifiedBy" jsonschema:"User id (UUID) who created the active version."`
}

// Output is the typed response.
type Output struct {
	Status     OutputStatus `json:"status" jsonschema:"'success'; 'validation_error' for a bad input (nothing was called); 'error' when Collibra could not answer."`
	Message    string       `json:"message" jsonschema:"Plain-language summary that can be relayed to the customer."`
	Guidance   string       `json:"guidance,omitempty" jsonschema:"What to tell or ask the customer next, on error."`
	ErrorCode  string       `json:"errorCode,omitempty" jsonschema:"Machine-readable error code from Collibra, when it rejected the request."`
	HTTPStatus int          `json:"httpStatus,omitempty" jsonschema:"HTTP status of the rejection, when Collibra answered with one."`
	Total      *int         `json:"total,omitempty" jsonschema:"Total matches, when includeTotal was true and no agentId was given."`
	Contracts  []Contract   `json:"contracts,omitempty" jsonschema:"The contracts. Empty with agentId means no contract governs that agent."`
}

// NewTool returns the registered tool.
func NewTool(collibraClient *http.Client) *chip.Tool[Input, Output] {
	return &chip.Tool[Input, Output]{
		Name:  "list_agent_contracts",
		Title: "List Agent Contracts",
		Description: "List Guardian agent contracts: the named sets of plain-language rules that govern AI agents. " +
			"Use it to see which contracts exist (for example before creating one, since only one GLOBAL contract is allowed) or, with agentId, which contracts govern one AI agent. " +
			"Each entry has the contract id, layer (GLOBAL or SPECIALIZED), active version and rule count; read the rules with get_agent_contract_rules. " +
			"It only reads, as the signed-in user, and only returns contracts that user can view. " +
			"If the Agent Contracts feature is off, the answer says it has to be enabled in AI Command Center. " +
			"Example requests: 'which agent contracts do we have', 'which contracts apply to this agent'.",
		Handler:     handler(collibraClient),
		Permissions: []string{},
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, DestructiveHint: chip.Ptr(false), IdempotentHint: true, OpenWorldHint: chip.Ptr(false)},
	}
}

func handler(collibraClient *http.Client) chip.ToolHandlerFunc[Input, Output] {
	return func(ctx context.Context, input Input) (Output, error) {
		params := clients.ListAgentContractsParams{
			Name:         strings.TrimSpace(input.Name),
			SortField:    strings.ToUpper(strings.TrimSpace(input.SortField)),
			SortOrder:    strings.ToUpper(strings.TrimSpace(input.SortOrder)),
			Offset:       input.Offset,
			Limit:        input.Limit,
			IncludeTotal: input.IncludeTotal,
		}
		if id := strings.TrimSpace(input.AgentID); id != "" {
			if _, err := uuid.Parse(id); err != nil {
				return validationError(fmt.Sprintf("agentId %q is not a valid UUID. Pass the AI Agent asset's UUID.", input.AgentID)), nil
			}
			params.AgentID = id
		}
		switch params.SortField {
		case "", "NAME", "RULE_COUNT", "LAST_MODIFIED_ON":
		default:
			return validationError(fmt.Sprintf("sortField %q is not valid. Use NAME, RULE_COUNT or LAST_MODIFIED_ON.", input.SortField)), nil
		}
		switch params.SortOrder {
		case "", "ASC", "DESC":
		default:
			return validationError(fmt.Sprintf("sortOrder %q is not valid. Use ASC or DESC.", input.SortOrder)), nil
		}
		if params.Offset < 0 || params.Limit < 0 || params.Limit > 500 {
			return validationError("offset must be 0 or more and limit between 1 and 500."), nil
		}

		page, err := clients.ListAgentContracts(ctx, collibraClient, params)
		if err != nil {
			message, guidance, code, status := clients.DescribeContractError(err)
			return Output{Status: StatusError, Message: message, Guidance: guidance, ErrorCode: code, HTTPStatus: status}, nil
		}

		out := Output{Status: StatusSuccess, Total: page.Total}
		for _, c := range page.Results {
			out.Contracts = append(out.Contracts, Contract{
				ID: c.ID, Name: c.Name, Layer: c.Layer,
				ActiveVersionID: c.ActiveVersionID, ActiveVersionNumber: c.ActiveVersionNumber,
				RuleCount: c.RuleCount, GovernedAgentCount: c.GovernedAgentCount,
				LastModifiedOn: c.LastModifiedOn, LastModifiedBy: c.LastModifiedBy,
			})
		}
		switch {
		case len(out.Contracts) == 0 && params.AgentID != "":
			out.Message = "No agent contract governs this AI agent."
		case len(out.Contracts) == 0:
			out.Message = "No agent contracts found."
		default:
			out.Message = fmt.Sprintf("Found %d agent contract(s).", len(out.Contracts))
		}
		return out, nil
	}
}

func validationError(message string) Output {
	return Output{Status: StatusValidationError, Message: message}
}
