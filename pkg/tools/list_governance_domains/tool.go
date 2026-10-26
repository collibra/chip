// Package list_governance_domains implements the list_governance_domains MCP
// tool: it lists the Governance Asset domains, the only domains in which an
// agent contract can be created.
package list_governance_domains

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/collibra/chip/pkg/chip"
	"github.com/collibra/chip/pkg/clients"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// OutputStatus is the outcome of a list_governance_domains call.
type OutputStatus string

const (
	StatusSuccess         OutputStatus = "success"
	StatusValidationError OutputStatus = "validation_error"
	StatusError           OutputStatus = "error"
)

// Input is the tool's typed input.
type Input struct {
	Name  string `json:"name,omitempty" jsonschema:"Optional. Only domains whose name contains this text (case-insensitive)."`
	Limit int    `json:"limit,omitempty" jsonschema:"Optional. Maximum number of domains; default 50, maximum 500."`
}

// Domain is one Governance Asset domain.
type Domain struct {
	ID            string `json:"id" jsonschema:"Domain id (UUID): pass it as domainId to create_agent_contract."`
	Name          string `json:"name" jsonschema:"Domain name."`
	CommunityName string `json:"communityName,omitempty" jsonschema:"Name of the community the domain sits in."`
}

// Output is the typed response.
type Output struct {
	Status     OutputStatus `json:"status" jsonschema:"'success'; 'validation_error' for a bad input; 'error' when Collibra could not answer."`
	Message    string       `json:"message" jsonschema:"Plain-language summary that can be relayed to the customer."`
	Guidance   string       `json:"guidance,omitempty" jsonschema:"What to tell or ask the customer next, on error."`
	ErrorCode  string       `json:"errorCode,omitempty" jsonschema:"Machine-readable error code from Collibra, when it rejected the request."`
	HTTPStatus int          `json:"httpStatus,omitempty" jsonschema:"HTTP status of the rejection, when Collibra answered with one."`
	Domains    []Domain     `json:"domains,omitempty" jsonschema:"The Governance Asset domains the user can see."`
}

// NewTool returns the registered tool.
func NewTool(collibraClient *http.Client) *chip.Tool[Input, Output] {
	return &chip.Tool[Input, Output]{
		Name:  "list_governance_domains",
		Title: "List Governance Domains",
		Description: "List the Governance Asset domains: the only kind of domain an agent contract can be created in. " +
			"Use it to find the domainId that create_agent_contract needs; there is no default domain. If several domains fit, ask the customer which one. " +
			"It only reads, as the signed-in user.",
		Handler:     handler(collibraClient),
		Permissions: []string{},
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, DestructiveHint: chip.Ptr(false), IdempotentHint: true, OpenWorldHint: chip.Ptr(false)},
	}
}

func handler(collibraClient *http.Client) chip.ToolHandlerFunc[Input, Output] {
	return func(ctx context.Context, input Input) (Output, error) {
		if input.Limit < 0 || input.Limit > 500 {
			return Output{Status: StatusValidationError, Message: "limit must be between 1 and 500."}, nil
		}
		domains, err := clients.ListGovernanceDomains(ctx, collibraClient, strings.TrimSpace(input.Name), input.Limit)
		if err != nil {
			message, guidance, code, status := clients.DescribeContractError(err)
			return Output{Status: StatusError, Message: message, Guidance: guidance, ErrorCode: code, HTTPStatus: status}, nil
		}
		out := Output{Status: StatusSuccess}
		for _, d := range domains {
			out.Domains = append(out.Domains, Domain{ID: d.ID, Name: d.Name, CommunityName: d.CommunityName})
		}
		if len(out.Domains) == 0 {
			out.Message = "No Governance Asset domains found for this user."
		} else {
			out.Message = fmt.Sprintf("Found %d Governance Asset domain(s).", len(out.Domains))
		}
		return out, nil
	}
}
