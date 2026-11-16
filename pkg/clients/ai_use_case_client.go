package clients

import (
	"context"
	"errors"
	"fmt"
	"net/http"
)

// aiUseCaseRegisterPath is the AI Governance endpoint behind the AI Command
// Center registry page. Unlike the generic asset API, it is secured by the
// global permission to register AI use cases, so an AI Business User or AI
// Producer can use it without a role on the domain.
const aiUseCaseRegisterPath = "/rest/aiGovernance/v1/aiUseCases/register"

// RegisterAiUseCaseRequest is the body of POST /aiUseCases/register.
type RegisterAiUseCaseRequest struct {
	Name        string   `json:"name"`
	DomainID    string   `json:"domainId"`
	Description string   `json:"description,omitempty"`
	OwnerIDs    []string `json:"ownerIds,omitempty"`
}

// RegisterAiUseCase creates an AI Use Case asset as the current user and
// returns its id. The caller only becomes Requester; only ownerIds become
// Owner, and without the Owner role an AICC user cannot link the use case
// afterwards. It never retries.
func RegisterAiUseCase(ctx context.Context, c *http.Client, request RegisterAiUseCaseRequest) (string, error) {
	var created struct {
		ID string `json:"id"`
	}
	if err := agentContractJSON(ctx, c, http.MethodPost, aiUseCaseRegisterPath, request, &created); err != nil {
		return "", err
	}
	return created.ID, nil
}

// DescribeUseCaseError turns an error from RegisterAiUseCase into the pieces
// the tool reports, with what to tell the customer next.
func DescribeUseCaseError(err error) (message, guidance, errorCode string, httpStatus int) {
	var apiErr *AgentScanAPIError
	if !errors.As(err, &apiErr) {
		return fmt.Sprintf("The request to Collibra did not complete: %v.", err),
			"It is not known whether the AI use case was created. Search for it by name before trying again.",
			"", 0
	}
	switch apiErr.StatusCode {
	case http.StatusUnauthorized:
		message = "Collibra did not accept the sign-in for this request (HTTP 401)."
		guidance = "Do not retry. Ask the customer to sign in to Collibra again, then start over."
	case http.StatusForbidden:
		message = "Collibra refused to register the AI use case for the current user (HTTP 403)."
		guidance = "Do not retry or try another route. Tell the customer they lack the permission to register AI use cases, held by the AI Business User and AI Producer roles, and to ask their Collibra administrator."
	case http.StatusBadRequest, http.StatusNotFound, http.StatusConflict, http.StatusUnprocessableEntity:
		message = fmt.Sprintf("Collibra rejected the AI use case (HTTP %d).", apiErr.StatusCode)
		guidance = "Nothing was created. Explain the problem to the customer and fix the name or domain with them before trying again."
	default:
		message = fmt.Sprintf("Collibra could not register the AI use case (HTTP %d).", apiErr.StatusCode)
		guidance = "It is not known whether the AI use case was created. Search for it by name before trying again."
	}
	if apiErr.UserMessage != "" {
		message += " Collibra said: " + apiErr.UserMessage
	}
	return message, guidance, apiErr.ErrorCode, apiErr.StatusCode
}
