// Package register_ai_use_case implements the register_ai_use_case MCP tool:
// it creates an AI Use Case asset (POST
// /rest/aiGovernance/v1/aiUseCases/register) as the calling user, the way the
// AI Command Center registry page does. It never retries.
package register_ai_use_case

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

// OutputStatus is the outcome of a register_ai_use_case call.
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
	Name        string `json:"name" jsonschema:"Required. Name of the AI use case."`
	DomainID    string `json:"domainId" jsonschema:"Required. UUID of the domain for the AI use case (find it with prepare_create_asset, assetType 'AI Use Case')."`
	Description string `json:"description,omitempty" jsonschema:"Optional. Plain-language description of the AI use case."`
	Confirm     bool   `json:"confirm,omitempty" jsonschema:"Safety checkpoint. false (default) returns a PREVIEW of exactly what would be created and sends NOTHING. Set true only after the user has clearly agreed to create this exact AI use case."`
}

// Preview echoes every field that confirm=true will send.
type Preview struct {
	Name        string `json:"name" jsonschema:"The name that will be sent."`
	DomainID    string `json:"domainId" jsonschema:"The domain that will be sent."`
	Description string `json:"description,omitempty" jsonschema:"The description that will be sent."`
}

// Output is the typed response.
type Output struct {
	Status      OutputStatus `json:"status" jsonschema:"'preview' when confirm was not set (nothing sent; show it to the user, then call again with confirm=true); 'success' when the AI use case was created; 'validation_error' for bad inputs (nothing sent); 'error' when it was not created."`
	Message     string       `json:"message" jsonschema:"Plain-language summary that can be relayed to the customer."`
	Guidance    string       `json:"guidance,omitempty" jsonschema:"What to tell or ask the customer next, on error."`
	ErrorCode   string       `json:"errorCode,omitempty" jsonschema:"Machine-readable error code from Collibra, when it rejected the request."`
	HTTPStatus  int          `json:"httpStatus,omitempty" jsonschema:"HTTP status of the rejection, when Collibra answered with one."`
	Preview     *Preview     `json:"preview,omitempty" jsonschema:"What would be sent, returned when confirm=false."`
	AiUseCaseID string       `json:"aiUseCaseId,omitempty" jsonschema:"Asset id of the created AI use case, on success."`
}

// NewTool returns the registered tool.
func NewTool(collibraClient *http.Client) *chip.Tool[Input, Output] {
	return &chip.Tool[Input, Output]{
		Name:  "register_ai_use_case",
		Title: "Register AI Use Case",
		Description: "Create an AI use case in Collibra as the current user, the way the AI Command Center registry page does. " +
			"Use it instead of create_asset for AI use cases: it needs the global permission to register AI use cases (held by the AI Business User and AI Producer roles), not a role on the domain. " +
			"Required: name and domainId (from prepare_create_asset with assetType 'AI Use Case'); optional: description. The current user becomes the Owner, so they can link the use case to an AI agent afterwards with edit_asset. " +
			"Built around a confirm checkpoint: confirm=false (default) returns a PREVIEW of exactly what would be created and sends nothing: show it to the user and ask; call again with confirm=true only after a clear yes to that exact content. " +
			"Never retry a failed create blindly: on a network failure, search for the use case by name first. Permission and sign-in errors (HTTP 401, 403) are not transient: do not retry, do not look for another route.",
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
				Message: fmt.Sprintf("Preview only: nothing was created. confirm=true will create the AI use case %q in domain %s. Show the user the name and description, ask whether to create it, and call again with confirm=true only after a clear yes.",
					request.Name, request.DomainID),
				Preview: &Preview{Name: request.Name, DomainID: request.DomainID, Description: request.Description},
			}, nil
		}

		// The current user becomes Owner, the way the registry page does it:
		// without it, linking the use case to an agent is refused.
		user, err := clients.GetCurrentUser(ctx, collibraClient)
		if err != nil {
			return Output{Status: StatusError, Message: fmt.Sprintf("Nothing was created: the current user could not be read: %v.", err),
				Guidance: "Ask the customer to check they are signed in to Collibra, then try again."}, nil
		}
		request.OwnerIDs = []string{user.ID}

		id, err := clients.RegisterAiUseCase(ctx, collibraClient, request)
		if err != nil {
			message, guidance, code, status := clients.DescribeUseCaseError(err)
			return Output{Status: StatusError, Message: message, Guidance: guidance, ErrorCode: code, HTTPStatus: status}, nil
		}
		return Output{
			Status:      StatusSuccess,
			Message:     fmt.Sprintf("Created the AI use case %q, with you as its Owner.", request.Name),
			AiUseCaseID: id,
		}, nil
	}
}

func compose(input Input) (clients.RegisterAiUseCaseRequest, string) {
	request := clients.RegisterAiUseCaseRequest{
		Name:        strings.TrimSpace(input.Name),
		DomainID:    strings.TrimSpace(input.DomainID),
		Description: strings.TrimSpace(input.Description),
	}
	if request.Name == "" {
		return request, "name is required."
	}
	if _, err := uuid.Parse(request.DomainID); err != nil {
		return request, fmt.Sprintf("domainId %q is not a valid UUID. Find the domain with prepare_create_asset (assetType 'AI Use Case').", input.DomainID)
	}
	return request, ""
}
