package create_maestro_agent

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/collibra/chip/pkg/chip"
	"github.com/collibra/chip/pkg/clients"
	"github.com/collibra/chip/pkg/tools/maestro"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type Input struct {
	Name            string                    `json:"name" jsonschema:"Required. The display name of the agent, up to 255 characters, for example 'FinRep Assistant'."`
	Handle          string                    `json:"handle" jsonschema:"Required. The agent's unique identifier: letters, digits and underscores only, up to 50 characters, for example 'finrep'. A handle already in use fails with errorCode duplicatedHandle — pick another one, or edit the existing agent with edit_maestro_agent instead."`
	Description     string                    `json:"description,omitempty" jsonschema:"Optional. What the agent is for, up to 1000 characters. Shown to end users choosing an agent."`
	Instructions    string                    `json:"instructions,omitempty" jsonschema:"Optional. The system instructions that shape how the agent answers — its role, tone and rules — up to 16000 characters."`
	Color           string                    `json:"color,omitempty" jsonschema:"Optional. The agent's hex color as #rrggbb, for example '#005ce8'. Defaults to #005ce8. Some colors are reserved and are rejected by the API."`
	WelcomeMessage  string                    `json:"welcomeMessage,omitempty" jsonschema:"Optional. The greeting shown by default in the agent's chat window, up to 500 characters."`
	SampleQuestions []string                  `json:"sampleQuestions,omitempty" jsonschema:"Optional. Up to 5 example questions offered to end users, each up to 255 characters."`
	Tools           []string                  `json:"tools,omitempty" jsonschema:"Optional. Up to 50 tool names the agent may call, for example 'get_asset_details'. These are Collibra Copilot tool names, resolved by the server — an unknown one is rejected."`
	Sharing         *clients.MaestroSharing   `json:"sharing,omitempty" jsonschema:"Optional. Who besides the creator can see and use the agent in Collibra Copilot. All three lists (roles, groups, users) have to be given once sharing is set; use empty lists for the ones you do not need. Three empty lists mean only the creator sees the agent; to share with everyone include the Everyone group id 00000000-0000-0000-0000-000001000001 in groups. Defaults to creator-only when omitted."`
	Ownership       *clients.MaestroOwnership `json:"ownership,omitempty" jsonschema:"Optional. The users, besides the creator, allowed to edit the agent in AI Maestro, as a list of user UUIDs in 'users'. The creator is always an owner and must not be listed. Defaults to creator-only when omitted."`
}

type Output struct {
	Agent     *clients.MaestroAgent `json:"agent,omitempty" jsonschema:"The agent as stored, including its id, handle, status (always DRAFT) and isValid flag. Use the id with get_maestro_agent to read it back or with edit_maestro_agent to change it."`
	ErrorCode string                `json:"errorCode,omitempty" jsonschema:"The API error code when creation failed, when it reported one. duplicatedHandle means the handle is taken; BAD_REQUEST_UNSUPPORTED_TOOL means one of the tools does not exist; BAD_REQUEST_RESERVED_HANDLE and BAD_REQUEST_RESERVED_COLOR mean the handle or color may not be used; BAD_REQUEST_CREATOR_IN_OWNERSHIP_USERS means ownership.users names the creator."`
	Error     string                `json:"error,omitempty" jsonschema:"Error message if the agent could not be created"`
	Created   bool                  `json:"created" jsonschema:"Whether the agent was created"`
}

func NewTool(collibraClient *http.Client) *chip.Tool[Input, Output] {
	return &chip.Tool[Input, Output]{
		Name:  "create_maestro_agent",
		Title: "Create Maestro Agent",
		Description: `Create a Collibra AI Maestro agent. "Agent" here means a Maestro agent — a configured assistant that end users chat with in Collibra Copilot — not the model calling this tool.
					  Only name and handle are required; everything else can be filled in later with edit_maestro_agent. The agent is always created in DRAFT status with the connecting user as its creator and owner, and has to be submitted and approved in AI Maestro before end users can use it.
					  The returned isValid flag says whether the definition is complete enough to be submitted, which takes five fields: name, handle, instructions, welcomeMessage and at least one sampleQuestions entry. Creating an agent with only a name and a handle therefore comes back isValid false; pass all five to get a submittable agent in one call.
					  This tool cannot set the agent's knowledge base or its status: knowledge base views are configured in AI Maestro. An agent without them can still be valid and submitted — the knowledge base does not affect isValid.
					  References in the request (tool names, and the users, groups and roles in sharing and ownership) are resolved against this environment, and an unknown one rejects the whole request without creating anything. Read the returned errorCode and decide — asking the user when the choice is theirs — whether to correct the request or leave it.`,
		Handler:     handler(collibraClient),
		Permissions: []string{},
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: false, DestructiveHint: chip.Ptr(false), IdempotentHint: false, OpenWorldHint: chip.Ptr(false)},
	}
}

func handler(collibraClient *http.Client) chip.ToolHandlerFunc[Input, Output] {
	return func(ctx context.Context, input Input) (Output, error) {
		if strings.TrimSpace(input.Name) == "" {
			return Output{}, errors.New("name is required: pass the agent's display name")
		}
		if strings.TrimSpace(input.Handle) == "" {
			return Output{}, errors.New("handle is required: pass the agent's unique identifier, letters, digits and underscores only")
		}
		if err := maestro.ValidateReferences(input.Sharing, input.Ownership); err != nil {
			return Output{}, err
		}

		agent, err := clients.CreateAgent(ctx, collibraClient, clients.CreateAgentRequest{
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
		})
		if err != nil {
			return createFailed(err), nil
		}

		return Output{
			Agent:   agent,
			Created: true,
		}, nil
	}
}

// createFailed hands the rejected request to the caller with its error code
// intact, so it can tell a taken handle from an unknown tool and correct itself
// without a round trip. A rejected request creates nothing at all.
func createFailed(err error) Output {
	output := Output{
		Error:   fmt.Sprintf("Failed to create agent: %s", err.Error()),
		Created: false,
	}

	var agentErr *clients.MaestroAgentError
	if errors.As(err, &agentErr) {
		output.ErrorCode = agentErr.ErrorCode
	}

	return output
}
