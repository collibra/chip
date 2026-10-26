// Package create_agent_contract implements the create_agent_contract MCP tool:
// it creates an agent contract and its first version (POST
// /rest/aiGovernance/v1/agentContracts) as the calling user. It never retries.
package create_agent_contract

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

// OutputStatus is the outcome of a create_agent_contract call.
type OutputStatus string

const (
	// StatusPreview means confirm was not set: nothing was sent.
	StatusPreview         OutputStatus = "preview"
	StatusSuccess         OutputStatus = "success"
	StatusValidationError OutputStatus = "validation_error"
	StatusError           OutputStatus = "error"
)

// Input is the tool's typed input.
type Input struct {
	Layer      string   `json:"layer" jsonschema:"Required. GLOBAL (governs every AI agent; at most one may exist) or SPECIALIZED (governs only the agents later linked to it)."`
	Name       string   `json:"name" jsonschema:"Required. Contract name, also the name of its asset. Unique within the domain; at most 2000 characters."`
	DomainID   string   `json:"domainId" jsonschema:"Required. UUID of a Governance Asset domain (find it with list_governance_domains). There is no default."`
	Statements []string `json:"statements" jsonschema:"Required. The ordered plain-language rules of the first version: 1 to 200 statements, each up to 2000 characters, unique once whitespace is cleaned up."`
	Confirm    bool     `json:"confirm,omitempty" jsonschema:"Safety checkpoint. false (default) returns a PREVIEW of exactly what would be created and sends NOTHING. Set true only after the user has clearly agreed to create this exact contract."`
}

// Preview echoes every field that confirm=true will send.
type Preview struct {
	Layer      string   `json:"layer" jsonschema:"The layer that will be sent."`
	Name       string   `json:"name" jsonschema:"The name that will be sent."`
	DomainID   string   `json:"domainId" jsonschema:"The domain that will be sent."`
	Statements []string `json:"statements" jsonschema:"The statements that will be stored, whitespace cleaned up, in order."`
}

// Output is the typed response.
type Output struct {
	Status          OutputStatus `json:"status" jsonschema:"'preview' when confirm was not set (nothing sent; show it to the user, then call again with confirm=true); 'success' when the contract was created; 'validation_error' for bad inputs (nothing sent); 'error' when it was not created."`
	Message         string       `json:"message" jsonschema:"Plain-language summary that can be relayed to the customer."`
	Guidance        string       `json:"guidance,omitempty" jsonschema:"What to tell or ask the customer next, on error."`
	ErrorCode       string       `json:"errorCode,omitempty" jsonschema:"Machine-readable error code from Collibra, when it rejected the request."`
	HTTPStatus      int          `json:"httpStatus,omitempty" jsonschema:"HTTP status of the rejection, when Collibra answered with one."`
	Preview         *Preview     `json:"preview,omitempty" jsonschema:"What would be sent, returned when confirm=false."`
	AgentContractID string       `json:"agentContractId,omitempty" jsonschema:"Id of the created contract, on success."`
	CreatedBy       string       `json:"createdBy,omitempty" jsonschema:"User id (UUID) of the user the contract was created as, on success: the current user. Use it as userId when assigning the Owner role."`
	VersionID       string       `json:"versionId,omitempty" jsonschema:"Id of the first version, on success."`
	VersionNumber   int          `json:"versionNumber,omitempty" jsonschema:"Number of the first version (1), on success."`
	RuleCount       int          `json:"ruleCount,omitempty" jsonschema:"Number of rules stored, on success."`
}

// NewTool returns the registered tool.
func NewTool(collibraClient *http.Client) *chip.Tool[Input, Output] {
	return &chip.Tool[Input, Output]{
		Name:  "create_agent_contract",
		Title: "Create Agent Contract",
		Description: "Create a Guardian agent contract in Collibra, as the current user: a named set of plain-language rules, stored as version 1 together with a new contract asset in the given Governance Asset domain. " +
			"Required: layer (GLOBAL or SPECIALIZED), name, domainId (from list_governance_domains) and statements. " +
			"Only one GLOBAL contract may exist, so check list_agent_contracts first. This does not link the contract to any AI agent: a GLOBAL contract governs every agent, a SPECIALIZED one governs none until agents are linked to it in Collibra. The caller becomes the Owner of the new contract's asset (the agentContractId is the asset id; createdBy is the caller's user id). " +
			"Built around a confirm checkpoint: confirm=false (default) returns a PREVIEW of exactly what would be created and sends nothing: show it to the user and ask; call again with confirm=true only after a clear yes to that exact content. " +
			"Never retry a failed create blindly: on a network failure, check with list_agent_contracts first. Permission and sign-in errors (HTTP 401, 403) are not transient: do not retry, do not look for another route. " +
			"If the Agent Contracts feature is off, the error says it has to be enabled in AI Command Center. " +
			"Requires the global permission to create agent contracts (held by the AI Business User role).",
		Handler:     handler(collibraClient),
		Permissions: []string{},
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: false, DestructiveHint: chip.Ptr(false), IdempotentHint: false, OpenWorldHint: chip.Ptr(false)},
	}
}

func handler(collibraClient *http.Client) chip.ToolHandlerFunc[Input, Output] {
	return func(ctx context.Context, input Input) (Output, error) {
		request, problem := compose(input)
		if problem != "" {
			return Output{Status: StatusValidationError, Message: problem}, nil
		}

		if !input.Confirm {
			return Output{
				Status: StatusPreview,
				Message: fmt.Sprintf("Preview only: nothing was created. confirm=true will create the %s contract %q with %d rule(s) in domain %s. Show the user the rules, ask whether to create it, and call again with confirm=true only after a clear yes.",
					request.Layer, request.Name, len(request.Statements), request.DomainID),
				Preview: &Preview{Layer: request.Layer, Name: request.Name, DomainID: request.DomainID, Statements: request.Statements},
			}, nil
		}

		contract, err := clients.CreateAgentContract(ctx, collibraClient, request)
		if err != nil {
			message, guidance, code, status := clients.DescribeContractError(err)
			return Output{Status: StatusError, Message: message, Guidance: guidance, ErrorCode: code, HTTPStatus: status}, nil
		}
		return Output{
			Status:          StatusSuccess,
			Message:         fmt.Sprintf("Created the %s agent contract %q with %d rule(s).", contract.Layer, contract.Name, contract.RuleCount),
			AgentContractID: contract.ID, CreatedBy: contract.LastModifiedBy, VersionID: contract.ActiveVersionID,
			VersionNumber: contract.ActiveVersionNumber, RuleCount: contract.RuleCount,
		}, nil
	}
}

func compose(input Input) (clients.CreateAgentContractRequest, string) {
	request := clients.CreateAgentContractRequest{
		Layer: strings.ToUpper(strings.TrimSpace(input.Layer)),
		Name:  strings.TrimSpace(input.Name),
	}
	if request.Layer != "GLOBAL" && request.Layer != "SPECIALIZED" {
		return request, fmt.Sprintf("layer %q is not valid. Use GLOBAL or SPECIALIZED.", input.Layer)
	}
	if request.Name == "" || len([]rune(request.Name)) > 2000 {
		return request, "name is required and must be at most 2000 characters."
	}
	request.DomainID = strings.TrimSpace(input.DomainID)
	if _, err := uuid.Parse(request.DomainID); err != nil {
		return request, fmt.Sprintf("domainId %q is not a valid UUID. Find a Governance Asset domain with list_governance_domains.", input.DomainID)
	}
	statements, problem := clients.NormalizeStatements(input.Statements)
	if problem != "" {
		return request, problem
	}
	request.Statements = statements
	return request, ""
}
