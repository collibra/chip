// Package get_agent_contract implements the get_agent_contract MCP tool: it
// reads one agent contract (GET /rest/aiGovernance/v1/agentContracts/{id}).
package get_agent_contract

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

// OutputStatus is the outcome of a get_agent_contract call.
type OutputStatus string

const (
	StatusSuccess         OutputStatus = "success"
	StatusNotFound        OutputStatus = "not_found"
	StatusValidationError OutputStatus = "validation_error"
	StatusError           OutputStatus = "error"
)

// Input is the tool's typed input.
type Input struct {
	AgentContractID string `json:"agentContractId" jsonschema:"Required. UUID of the agent contract (from list_agent_contracts or create_agent_contract)."`
}

// Output is the typed response.
type Output struct {
	Status              OutputStatus `json:"status" jsonschema:"'success'; 'not_found' when the contract does not exist, was deleted or is not visible; 'validation_error' for a bad input; 'error' when Collibra could not answer."`
	Message             string       `json:"message" jsonschema:"Plain-language summary that can be relayed to the customer."`
	Guidance            string       `json:"guidance,omitempty" jsonschema:"What to tell or ask the customer next, on error."`
	ErrorCode           string       `json:"errorCode,omitempty" jsonschema:"Machine-readable error code from Collibra, when it rejected the request."`
	HTTPStatus          int          `json:"httpStatus,omitempty" jsonschema:"HTTP status of the rejection, when Collibra answered with one."`
	ID                  string       `json:"id,omitempty" jsonschema:"Agent contract id."`
	Name                string       `json:"name,omitempty" jsonschema:"Contract name."`
	Layer               string       `json:"layer,omitempty" jsonschema:"GLOBAL or SPECIALIZED."`
	ActiveVersionID     string       `json:"activeVersionId,omitempty" jsonschema:"Id of the active version."`
	ActiveVersionNumber int          `json:"activeVersionNumber,omitempty" jsonschema:"Number of the active version."`
	RuleCount           int          `json:"ruleCount,omitempty" jsonschema:"Rules in the active version. Read them with get_agent_contract_rules."`
	GovernedAgentCount  *int         `json:"governedAgentCount,omitempty" jsonschema:"AI agents the contract governs (only those the user can view)."`
	CreatedOn           string       `json:"createdOn,omitempty" jsonschema:"When the contract was created (UTC)."`
	LastModifiedOn      string       `json:"lastModifiedOn,omitempty" jsonschema:"When the active version was created (UTC)."`
	LastModifiedBy      string       `json:"lastModifiedBy,omitempty" jsonschema:"User id (UUID) who created the active version."`
}

// NewTool returns the registered tool.
func NewTool(collibraClient *http.Client) *chip.Tool[Input, Output] {
	return &chip.Tool[Input, Output]{
		Name:  "get_agent_contract",
		Title: "Get Agent Contract",
		Description: "Read one Guardian agent contract: its name, layer (GLOBAL or SPECIALIZED), active version and number of rules and governed agents. " +
			"It does not return the rules themselves; use get_agent_contract_rules for those. " +
			"A not_found status is an answer (the contract does not exist, was deleted or is not visible to the user). It only reads, as the signed-in user.",
		Handler:     handler(collibraClient),
		Permissions: []string{},
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, DestructiveHint: chip.Ptr(false), IdempotentHint: true, OpenWorldHint: chip.Ptr(false)},
	}
}

func handler(collibraClient *http.Client) chip.ToolHandlerFunc[Input, Output] {
	return func(ctx context.Context, input Input) (Output, error) {
		id := strings.TrimSpace(input.AgentContractID)
		if _, err := uuid.Parse(id); err != nil {
			return Output{Status: StatusValidationError, Message: fmt.Sprintf("agentContractId %q is not a valid UUID. list_agent_contracts returns it.", input.AgentContractID)}, nil
		}
		c, err := clients.GetAgentContract(ctx, collibraClient, id)
		if err != nil {
			message, guidance, code, status := clients.DescribeContractError(err)
			out := Output{Status: StatusError, Message: message, Guidance: guidance, ErrorCode: code, HTTPStatus: status}
			if status == http.StatusNotFound {
				out.Status = StatusNotFound
			}
			return out, nil
		}
		return Output{
			Status:  StatusSuccess,
			Message: fmt.Sprintf("Agent contract %q (%s), version %d with %d rule(s).", c.Name, c.Layer, c.ActiveVersionNumber, c.RuleCount),
			ID:      c.ID, Name: c.Name, Layer: c.Layer,
			ActiveVersionID: c.ActiveVersionID, ActiveVersionNumber: c.ActiveVersionNumber,
			RuleCount: c.RuleCount, GovernedAgentCount: c.GovernedAgentCount,
			CreatedOn: c.CreatedOn, LastModifiedOn: c.LastModifiedOn, LastModifiedBy: c.LastModifiedBy,
		}, nil
	}
}
