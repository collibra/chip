// Package edit_maestro_agent implements the edit_maestro_agent MCP tool: a
// partial update of one Maestro Agent. The AI Maestro API takes the whole edit as
// a single PATCH, so this tool validates its input up front and makes ONE call —
// either everything applies or nothing does.
//
// Only the fields the caller passes are sent, and only those change. Two fields
// are deliberately absent from the input: status, whose omission is what tells the
// server to return the agent to DRAFT for re-review, and knowledgeBase, whose
// views are configured in AI Maestro.
package edit_maestro_agent

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/collibra/chip/pkg/chip"
	"github.com/collibra/chip/pkg/clients"
	"github.com/collibra/chip/pkg/tools/maestro"
	"github.com/collibra/chip/pkg/tools/validation"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Input is the tool's typed input. Every field but agentId is optional and
// pointer-typed, so leaving a field out ("keep what is stored") stays distinct
// from sending an empty value ("clear it").
type Input struct {
	AgentID         string                    `json:"agentId" jsonschema:"Required. The UUID of the AI Maestro agent to edit."`
	Name            *string                   `json:"name,omitempty" jsonschema:"Optional. The new display name, up to 255 characters. Omit to leave it unchanged."`
	Handle          *string                   `json:"handle,omitempty" jsonschema:"Optional. The new unique identifier: letters, digits and underscores only, up to 50 characters. A handle in use by another agent fails with errorCode duplicatedHandle, but sending this agent's own handle back unchanged is accepted, so a read-modify-write caller can pass through the handle it just read. Omit to leave it unchanged."`
	Description     *string                   `json:"description,omitempty" jsonschema:"Optional. The new description, up to 1000 characters. Omit to leave it unchanged, or send an empty string to clear it."`
	Instructions    *string                   `json:"instructions,omitempty" jsonschema:"Optional. The new system instructions, up to 16000 characters. This replaces the stored instructions rather than adding to them, so send the full text. isValid depends on this field, so clearing it leaves the agent unsubmittable until it is set again. Omit to leave it unchanged."`
	Color           *string                   `json:"color,omitempty" jsonschema:"Optional. The new hex color as #rrggbb. Some colors are reserved and are rejected by the API. Omit to leave it unchanged."`
	WelcomeMessage  *string                   `json:"welcomeMessage,omitempty" jsonschema:"Optional. The new greeting shown in the agent's chat window, up to 500 characters. isValid depends on this field, so clearing it with an empty string leaves the agent unsubmittable until it is set again. Omit to leave it unchanged."`
	SampleQuestions *[]string                 `json:"sampleQuestions,omitempty" jsonschema:"Optional. Up to 5 example questions, each up to 255 characters. Replaces the whole list rather than adding to it, so send every question you want to keep. isValid depends on this field, so sending an empty list to remove them all leaves the agent unsubmittable until at least one is set again. Omit to leave the list unchanged."`
	Tools           *[]string                 `json:"tools,omitempty" jsonschema:"Optional. Up to 50 Collibra Copilot tool names the agent may call. Replaces the whole list rather than adding to it, so send every tool you want to keep — read the agent with get_maestro_agent first if you only mean to add one. Omit to leave the list unchanged, or send an empty list to remove all of them."`
	Sharing         *clients.MaestroSharing   `json:"sharing,omitempty" jsonschema:"Optional. Who besides the creator can see and use the agent in Collibra Copilot. Replaces the whole configuration, and all three lists (roles, groups, users) have to be given once sharing is set. Three empty lists make the agent visible to the creator only; to share with everyone include the Everyone group id 00000000-0000-0000-0000-000001000001 in groups. Omit to leave sharing unchanged."`
	Ownership       *clients.MaestroOwnership `json:"ownership,omitempty" jsonschema:"Optional. The users, besides the creator, allowed to edit the agent in AI Maestro, as a list of user UUIDs in 'users'. Replaces the whole list; an empty list revokes edit rights from everyone but the creator, who is always an owner and must not be listed. Omit to leave ownership unchanged."`
}

type Output struct {
	Agent     *clients.MaestroAgent `json:"agent,omitempty" jsonschema:"The agent as stored after the update, so the caller can confirm what changed. Its status is DRAFT, and isValid says whether its definition is complete enough to be submitted for review — true only while name, handle, instructions, welcomeMessage and at least one sampleQuestions entry are all set. The knowledge base does not affect it. Check it after an edit that cleared any of those fields."`
	ErrorCode string                `json:"errorCode,omitempty" jsonschema:"The API error code when the update failed, when it reported one. duplicatedHandle means the new handle is taken; BAD_REQUEST_UNSUPPORTED_TOOL means one of the tools does not exist; BAD_REQUEST_RESERVED_HANDLE and BAD_REQUEST_RESERVED_COLOR mean the handle or color may not be used; BAD_REQUEST_CREATOR_IN_OWNERSHIP_USERS means ownership.users names the creator."`
	Error     string                `json:"error,omitempty" jsonschema:"Error message if the agent could not be updated"`
	Updated   bool                  `json:"updated" jsonschema:"Whether the agent was updated"`
}

func NewTool(collibraClient *http.Client) *chip.Tool[Input, Output] {
	return &chip.Tool[Input, Output]{
		Name:  "edit_maestro_agent",
		Title: "Edit Maestro Agent",
		Description: `Change an existing Collibra AI Maestro agent. "Agent" here means a Maestro agent — a configured assistant that end users chat with in Collibra Copilot — not the model calling this tool.
					  This is a partial update: pass the agentId plus only the fields you want to change, and everything else keeps its stored value. Lists and nested blocks (sampleQuestions, tools, sharing, ownership) are replaced wholesale rather than merged, so send the complete list — read the agent with get_maestro_agent first when you only mean to add or remove one entry. An empty list clears that field.
					  IMPORTANT: any edit returns the agent to DRAFT status. A published agent stops being available to end users in Collibra Copilot until it is submitted and approved again in AI Maestro. Tell the user this before editing a published agent. This tool cannot set the status itself, and it cannot change the agent's knowledge base — its views stay as they are and are configured in AI Maestro.
					  The caller has to be an owner of the agent, or hold the permission to manage all AI agents. References in the request (tool names, and the users, groups and roles in sharing and ownership) are resolved against this environment, and an unknown one rejects the whole update without changing anything.`,
		Handler:     handler(collibraClient),
		Permissions: []string{},
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: false, DestructiveHint: chip.Ptr(true), IdempotentHint: true, OpenWorldHint: chip.Ptr(false)},
	}
}

func handler(collibraClient *http.Client) chip.ToolHandlerFunc[Input, Output] {
	return func(ctx context.Context, input Input) (Output, error) {
		if err := validation.UUID("agentId", input.AgentID); err != nil {
			return Output{}, err
		}

		request := patchRequest(input)
		// An update with nothing in it would still demote the agent to DRAFT, which
		// is never what the caller meant to ask for.
		if request == (clients.PatchAgentRequest{}) {
			return Output{}, errors.New("nothing to edit: pass at least one of name, handle, description, instructions, color, welcomeMessage, sampleQuestions, tools, sharing or ownership")
		}

		if err := maestro.ValidateReferences(input.Sharing, input.Ownership); err != nil {
			return Output{}, err
		}

		agent, err := clients.UpdateAgent(ctx, collibraClient, input.AgentID, request)
		if err != nil {
			return updateFailed(err), nil
		}

		return Output{
			Agent:   agent,
			Updated: true,
		}, nil
	}
}

// patchRequest copies the fields the caller supplied straight through. The
// pointers are passed on as they arrived: a nil field is left out of the request
// body, which is how the API is told to keep the stored value.
func patchRequest(input Input) clients.PatchAgentRequest {
	return clients.PatchAgentRequest{
		Name:            input.Name,
		Handle:          input.Handle,
		Color:           input.Color,
		Description:     input.Description,
		Instructions:    input.Instructions,
		WelcomeMessage:  input.WelcomeMessage,
		SampleQuestions: input.SampleQuestions,
		Tools:           input.Tools,
		Sharing:         input.Sharing,
		Ownership:       input.Ownership,
	}
}

// updateFailed hands the rejected update to the caller with its error code intact,
// so it can tell a taken handle from an unknown tool and correct itself without a
// round trip. A rejected update changes nothing at all.
func updateFailed(err error) Output {
	output := Output{
		Error:   fmt.Sprintf("Failed to update agent: %s", err.Error()),
		Updated: false,
	}

	var agentErr *clients.MaestroAgentError
	if errors.As(err, &agentErr) {
		output.ErrorCode = agentErr.ErrorCode
	}

	return output
}
