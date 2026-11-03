// Package edit_domain implements the edit_domain MCP tool: a single entry
// point for renaming a domain, changing its description, moving it to another
// community, and managing its role assignments (responsibilities) via a typed
// list of operations. Each operation is validated independently and applied on
// its own, so one failing operation never blocks its valid siblings.
package edit_domain

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/collibra/chip/pkg/chip"
	"github.com/collibra/chip/pkg/clients"
	"github.com/collibra/chip/pkg/tools/resolve"
	"github.com/collibra/chip/pkg/tools/responsibility"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// OperationType enumerates the edits edit_domain can perform.
type OperationType string

const (
	OpSetName              OperationType = "set_name"
	OpSetDescription       OperationType = "set_description"
	OpSetCommunity         OperationType = "set_community"
	OpSetResponsibility    OperationType = "set_responsibility"
	OpRemoveResponsibility OperationType = "remove_responsibility"
)

// Input is the tool's typed input.
type Input struct {
	Domain     string      `json:"domain" jsonschema:"Required. The domain to edit (the folder inside a community that holds assets) — its UUID or its name (an exact, case-insensitive match; an ambiguous name returns the candidates with their UUIDs), e.g. 'Employee Wellness'."`
	Operations []Operation `json:"operations" jsonschema:"Required. Non-empty list of operations to apply. Each operation is validated and applied independently. Each operation's type selects which additional fields are used (see Operation)."`
}

// Operation is a discriminated union: the 'type' field selects which other
// fields are interpreted. Unused fields are ignored.
type Operation struct {
	Type OperationType `json:"type" jsonschema:"Required. One of: set_name, set_description, set_community, set_responsibility, remove_responsibility."`

	Value string `json:"value,omitempty" jsonschema:"For set_name: the new domain name (must not be empty). For set_description: the new description (an empty string clears it)."`

	Community string `json:"community,omitempty" jsonschema:"For set_community: the community to move the domain into, given as its UUID or name."`

	Role   string `json:"role,omitempty" jsonschema:"For set_responsibility and remove_responsibility: the resource role name (a named duty a person or group holds on the resource, matched case-insensitively), e.g. 'Steward' or 'Owner'; an unknown name returns the available roles."`
	UserID string `json:"userId,omitempty" jsonschema:"For set_responsibility and remove_responsibility: the user or group, given as a UUID, email address, username, or full name such as 'Jane Smith'. A full name shared by several users returns a per-operation error listing the candidates."`
}

// OutputStatus summarises the result of the call.
type OutputStatus string

const (
	StatusSuccess        OutputStatus = "success"
	StatusPartialSuccess OutputStatus = "partial_success"
	StatusError          OutputStatus = "error"
)

// Output is the tool's typed output.
type Output struct {
	Status  OutputStatus      `json:"status" jsonschema:"Overall status: success if every operation applied, partial_success if some succeeded and some failed, error if every operation failed."`
	Results []OperationResult `json:"results" jsonschema:"Per-operation outcomes, in the same order as the input operations."`
	Domain  *DomainSummary    `json:"domain,omitempty" jsonschema:"The domain's state after the edits. Present when the domain could be re-read."`
}

// DomainSummary is the post-edit snapshot of the domain.
type DomainSummary struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	CommunityID string `json:"communityId,omitempty"`
	Community   string `json:"community,omitempty"`
}

// OperationResult is the outcome of a single operation.
type OperationResult struct {
	Operation     OperationType `json:"operation"`
	Status        string        `json:"status" jsonschema:"'success' or 'error'."`
	PreviousValue string        `json:"previousValue,omitempty"`
	NewValue      string        `json:"newValue,omitempty"`
	Error         string        `json:"error,omitempty"`
}

// NewTool returns the registered tool.
func NewTool(collibraClient *http.Client) *chip.Tool[Input, Output] {
	return &chip.Tool[Input, Output]{
		Name:  "edit_domain",
		Title: "Edit Domain",
		Description: "Edit an existing Collibra domain (a domain is the folder inside a community that holds assets such as business terms or tables; communities are the top-level folders that group domains) identified by its UUID or name. " +
			"Use this to rename a domain, change its description, move it to a different community, or manage who is responsible for it. " +
			"To edit the assets inside a domain use edit_asset; to edit a community itself use edit_community; this tool cannot create or delete domains. " +
			"The domain must already exist; look up its UUID or exact name first if you only have a vague reference. " +
			"Submit a list of typed operations: " +
			"set_name (rename the domain); " +
			"set_description (change the description; an empty value clears it); " +
			"set_community (move the domain into a different community, given as a UUID or name); " +
			"set_responsibility (assign a user or group to a resource role, a named duty such as 'Steward' or 'Owner'; the user can be a UUID, username, email address, or full name such as 'Jane Smith'); " +
			"remove_responsibility (unassign a user or group from a role; removes only a responsibility assigned directly on the domain, not one inherited from its community). " +
			"Each operation is validated independently and applied on its own: invalid or failing operations return per-operation errors while valid siblings still apply, yielding status=success, partial_success, or error. " +
			"set_community verifies the result, so a request Collibra accepts without moving the domain is reported as an error. " +
			"On completion the response includes the domain's state after the edits. " +
			"This tool writes to Collibra immediately and requires permission to edit the domain. " +
			"Example requests: 'Rename the Employee Wellness domain to Staff Wellness', 'Move the Churn Metrics domain into the Marketing community', 'Make Jane Smith the Steward of the HR Glossary domain', 'Update the description of the Customer Data domain'.",
		Handler:     handler(collibraClient),
		Permissions: []string{},
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: false, DestructiveHint: chip.Ptr(true), IdempotentHint: false, OpenWorldHint: chip.Ptr(false)},
	}
}

func handler(collibraClient *http.Client) chip.ToolHandlerFunc[Input, Output] {
	return func(ctx context.Context, input Input) (Output, error) {
		if len(input.Operations) == 0 {
			return Output{}, fmt.Errorf("operations must not be empty")
		}
		domain, err := resolve.Domain(ctx, collibraClient, input.Domain, resolve.Hints{})
		if err != nil {
			return Output{}, err
		}

		// Validate every operation before any write, then apply the valid ones.
		results := make([]OperationResult, len(input.Operations))
		for i, op := range input.Operations {
			if verr := validateOperation(op); verr != nil {
				results[i] = errorResult(op, verr)
				continue
			}
			results[i] = executeOperation(ctx, collibraClient, domain, op)
		}

		out := Output{Status: overallStatus(results), Results: results}
		out.Domain = readDomain(ctx, collibraClient, domain.ID)
		return out, nil
	}
}

// validateOperation runs the checks that need no request.
func validateOperation(op Operation) error {
	switch op.Type {
	case OpSetName:
		if strings.TrimSpace(op.Value) == "" {
			return fmt.Errorf("value is required for set_name and must not be empty")
		}
	case OpSetDescription:
		// An empty value is a valid way to clear the description.
	case OpSetCommunity:
		if strings.TrimSpace(op.Community) == "" {
			return fmt.Errorf("community is required for set_community (a UUID or name)")
		}
	case OpSetResponsibility, OpRemoveResponsibility:
		return responsibility.Check(op.Role, op.UserID)
	default:
		return fmt.Errorf("unknown operation type %q; expected one of set_name, set_description, set_community, set_responsibility, remove_responsibility", op.Type)
	}
	return nil
}

func executeOperation(ctx context.Context, client *http.Client, domain clients.DomainDetails, op Operation) OperationResult {
	switch op.Type {
	case OpSetName:
		return patchField(ctx, client, domain, op, domain.Name, op.Value, clients.DomainPatch{Name: chip.Ptr(op.Value)})
	case OpSetDescription:
		return patchField(ctx, client, domain, op, domain.Description, op.Value, clients.DomainPatch{Description: chip.Ptr(op.Value)})
	case OpSetCommunity:
		return executeSetCommunity(ctx, client, domain, op)
	case OpSetResponsibility:
		return executeSetResponsibility(ctx, client, domain, op)
	default:
		return executeRemoveResponsibility(ctx, client, domain, op)
	}
}

func patchField(ctx context.Context, client *http.Client, domain clients.DomainDetails, op Operation, previous, next string, patch clients.DomainPatch) OperationResult {
	if _, err := clients.PatchDomain(ctx, client, domain.ID, patch); err != nil {
		return errorResult(op, err)
	}
	return OperationResult{Operation: op.Type, Status: "success", PreviousValue: previous, NewValue: next}
}

func executeSetCommunity(ctx context.Context, client *http.Client, domain clients.DomainDetails, op Operation) OperationResult {
	community, err := resolve.Community(ctx, client, op.Community, resolve.Hints{})
	if err != nil {
		return errorResult(op, err)
	}
	updated, err := clients.PatchDomain(ctx, client, domain.ID, clients.DomainPatch{CommunityID: chip.Ptr(community.ID)})
	if err != nil {
		return errorResult(op, err)
	}
	// Trust the response, not the status code: a no-op must not report success.
	if updated.Community == nil || updated.Community.ID != community.ID {
		return errorResult(op, fmt.Errorf("the request succeeded but the domain is not in community %q afterwards", community.Name))
	}
	res := OperationResult{Operation: op.Type, Status: "success", NewValue: community.Name}
	if domain.Community != nil {
		res.PreviousValue = domain.Community.Name
	}
	return res
}

func executeSetResponsibility(ctx context.Context, client *http.Client, domain clients.DomainDetails, op Operation) OperationResult {
	id, err := responsibility.Set(ctx, client, target(domain), op.Role, op.UserID)
	if err != nil {
		return errorResult(op, err)
	}
	return OperationResult{Operation: op.Type, Status: "success", NewValue: id}
}

func executeRemoveResponsibility(ctx context.Context, client *http.Client, domain clients.DomainDetails, op Operation) OperationResult {
	id, err := responsibility.Remove(ctx, client, target(domain), op.Role, op.UserID)
	if err != nil {
		return errorResult(op, err)
	}
	return OperationResult{Operation: op.Type, Status: "success", PreviousValue: id}
}

func target(domain clients.DomainDetails) responsibility.Target {
	return responsibility.Target{Type: responsibility.ResourceDomain, ID: domain.ID}
}

func errorResult(op Operation, err error) OperationResult {
	return OperationResult{Operation: op.Type, Status: "error", Error: err.Error()}
}

func overallStatus(results []OperationResult) OutputStatus {
	successes := 0
	for _, r := range results {
		if r.Status == "success" {
			successes++
		}
	}
	switch successes {
	case len(results):
		return StatusSuccess
	case 0:
		return StatusError
	default:
		return StatusPartialSuccess
	}
}

// readDomain re-reads the domain for the post-edit snapshot; nil if unreadable.
func readDomain(ctx context.Context, client *http.Client, id string) *DomainSummary {
	d, err := clients.GetDomain(ctx, client, id)
	if err != nil {
		return nil
	}
	s := &DomainSummary{ID: d.ID, Name: d.Name, Description: d.Description}
	if d.Community != nil {
		s.CommunityID, s.Community = d.Community.ID, d.Community.Name
	}
	return s
}
