package create_data_access_request

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/collibra/chip/pkg/chip"
	"github.com/collibra/chip/pkg/clients"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// aiDescriptionSuffix is appended to every description so the access request is clearly
// attributed to an AI agent.
const aiDescriptionSuffix = "This access request was created by AI."

// suggestedNameMaxLen caps the length of a name suggestion derived from the purpose.
const suggestedNameMaxLen = 80

// Status values returned in the Output.
const (
	statusNeedsNameConfirmation = "needs_name_confirmation"
	statusCreated               = "created"
)

type Input struct {
	Name    string                                     `json:"name,omitempty" jsonschema:"Optional. Display name of the access request. If omitted, the tool returns a suggested name derived from the purpose and asks the agent to confirm it with the user before retrying."`
	Purpose string                                     `json:"purpose" jsonschema:"Required. The user-supplied purpose / business justification for the access request. This is used verbatim as the description of the access request. The tool always appends a note indicating the request was created by AI."`
	UserIDs []string                                   `json:"userIds" jsonschema:"Required. IDs of the beneficiary users (the WHO of the request). Resolve these via the search_data_access_identities tool before calling."`
	What    []clients.CreateDataAccessRequestWhatInput `json:"what" jsonschema:"Required. The data objects the users are requesting access to (the WHAT of the request). Each item references a data object ID and optional requested permissions. Resolve the data object IDs via the search_data_access_objects tool before calling."`
}

type Output struct {
	Status        string                            `json:"status,omitempty" jsonschema:"Outcome of the call: needs_name_confirmation (no name was supplied — confirm the suggestedName with the user and call again with name set), or created (the request was successfully created)."`
	Message       string                            `json:"message,omitempty" jsonschema:"Human-readable explanation of the status. When status is needs_name_confirmation, this tells the agent to confirm the suggested name with the user."`
	SuggestedName string                            `json:"suggestedName,omitempty" jsonschema:"Name suggestion derived from the purpose. Present only when status is needs_name_confirmation."`
	Request       *clients.DataAccessRequestSummary `json:"request,omitempty" jsonschema:"The created access request, if successful."`
	Error         string                            `json:"error,omitempty" jsonschema:"Error message if the access request could not be created."`
}

func NewTool(collibraClient *http.Client) *chip.Tool[Input, Output] {
	return &chip.Tool[Input, Output]{
		Name:        "create_data_access_request",
		Description: "Create a new Collibra Data Access request. Requires the WHO (beneficiary user IDs, obtained via search_data_access_identities), the WHAT (data objects, obtained via search_data_access_objects), and a user-supplied purpose that is used as the description. If no name is supplied, the tool returns a suggested name derived from the purpose with status needs_name_confirmation — confirm the suggestion (or get a replacement) with the user, then call again with name set. The description always ends with a note stating that the request was created by AI.",
		Handler:     handle(collibraClient),
		Permissions: []string{},
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: false, DestructiveHint: new(false)},
	}
}

func handle(collibraClient *http.Client) chip.ToolHandlerFunc[Input, Output] {
	return func(ctx context.Context, input Input) (Output, error) {
		purpose := strings.TrimSpace(input.Purpose)
		if purpose == "" {
			return Output{Error: "purpose is required — ask the user for the business justification for this access request"}, nil
		}
		if len(input.UserIDs) == 0 {
			return Output{Error: "at least one beneficiary user ID is required — resolve them with search_data_access_identities"}, nil
		}
		if len(input.What) == 0 {
			return Output{Error: "at least one data object is required — resolve them with search_data_access_objects"}, nil
		}
		for i, w := range input.What {
			if strings.TrimSpace(w.DataObjectID) == "" {
				return Output{Error: fmt.Sprintf("what[%d].dataObjectId is required", i)}, nil
			}
		}

		name := strings.TrimSpace(input.Name)
		if name == "" {
			suggested := suggestNameFromPurpose(purpose)
			return Output{
				Status:        statusNeedsNameConfirmation,
				SuggestedName: suggested,
				Message:       fmt.Sprintf("No name was supplied. Suggested name based on the purpose: %q. Confirm this with the user (or ask for a different name), then call create_data_access_request again with the confirmed name in the `name` field.", suggested),
			}, nil
		}

		clientInput := clients.CreateDataAccessRequestInput{
			Name:        &name,
			Description: buildDescription(purpose),
			UserIDs:     input.UserIDs,
			What:        input.What,
		}

		req, err := clients.CreateDataAccessRequest(ctx, collibraClient, clientInput)
		if err != nil {
			return Output{Error: fmt.Sprintf("Failed to create data access request: %s", err.Error())}, nil
		}
		return Output{Status: statusCreated, Request: req}, nil
	}
}

func buildDescription(purpose string) string {
	if strings.Contains(purpose, aiDescriptionSuffix) {
		return purpose
	}
	if !strings.HasSuffix(purpose, ".") {
		purpose = purpose + "."
	}
	return purpose + " " + aiDescriptionSuffix
}

// suggestNameFromPurpose derives a short, human-readable name from the purpose text.
// It takes the first sentence/line, strips the AI-attribution suffix, collapses whitespace,
// truncates to suggestedNameMaxLen characters at a word boundary, and prefixes it.
func suggestNameFromPurpose(purpose string) string {
	summary := strings.ReplaceAll(purpose, aiDescriptionSuffix, "")
	summary = strings.Map(func(r rune) rune {
		if r == '\n' || r == '\r' || r == '\t' {
			return ' '
		}
		return r
	}, summary)
	if idx := strings.IndexAny(summary, ".!?"); idx >= 0 {
		summary = summary[:idx]
	}
	summary = strings.Join(strings.Fields(summary), " ")
	if summary == "" {
		return "Access request"
	}
	if len(summary) > suggestedNameMaxLen {
		truncated := summary[:suggestedNameMaxLen]
		if sp := strings.LastIndex(truncated, " "); sp > suggestedNameMaxLen/2 {
			truncated = truncated[:sp]
		}
		summary = strings.TrimRight(truncated, " ,;:-")
	}
	return "Access request: " + summary
}
