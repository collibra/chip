package get_maestro_agent

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/collibra/chip/pkg/chip"
	"github.com/collibra/chip/pkg/clients"
	"github.com/collibra/chip/pkg/tools/validation"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type Input struct {
	AgentID string `json:"agentId" jsonschema:"Required. The UUID of the AI Maestro agent to read."`
}

type Output struct {
	Agent     *clients.MaestroAgent `json:"agent,omitempty" jsonschema:"The agent as stored. Its editable fields are name, handle, description, instructions, color, welcomeMessage, sampleQuestions, tools, sharing and ownership — pass any of them to edit_maestro_agent with this agent's id to change them. The rest are read-only: id, status, isValid, createdBy, lastModifiedOn, lastModifiedBy, and knowledgeBase, whose views are configured in AI Maestro. tools is absent when the agent uses none, knowledgeBase when it has no views."`
	ErrorCode string                `json:"errorCode,omitempty" jsonschema:"The API error code when the read failed, when it reported one."`
	Error     string                `json:"error,omitempty" jsonschema:"Error message if the agent could not be retrieved"`
	Found     bool                  `json:"found" jsonschema:"Whether the agent was found"`
}

func NewTool(collibraClient *http.Client) *chip.Tool[Input, Output] {
	return &chip.Tool[Input, Output]{
		Name:  "get_maestro_agent",
		Title: "Get Maestro Agent",
		Description: `Read the configuration of an existing AI Maestro agent. "Agent" here means a Collibra AI Maestro agent — a configured assistant that end users chat with in Collibra Copilot — not the model calling this tool.
					  Returns the agent's handle, name, description, instructions, color, welcome message, sample questions, tools, knowledge base views, sharing configuration and ownership, together with its lifecycle status, whether its definition is complete (isValid), and who created and last modified it.
					  Requires the agentId as a UUID. The caller has to be able to see the agent — being one of its owners is not required to read it, but it is to change it with edit_maestro_agent.`,
		Handler:     handler(collibraClient),
		Permissions: []string{},
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, DestructiveHint: chip.Ptr(false), IdempotentHint: true, OpenWorldHint: chip.Ptr(false)},
	}
}

func handler(collibraClient *http.Client) chip.ToolHandlerFunc[Input, Output] {
	return func(ctx context.Context, input Input) (Output, error) {
		if err := validation.UUID("agentId", input.AgentID); err != nil {
			return Output{}, err
		}

		agent, err := clients.GetAgent(ctx, collibraClient, input.AgentID)
		if err != nil {
			return readFailed(err), nil
		}

		return Output{
			Agent: agent,
			Found: true,
		}, nil
	}
}

// readFailed hands the failed read to the caller with its error code intact, so a
// missing agent can be told apart from one the caller may not see.
func readFailed(err error) Output {
	output := Output{
		Error: fmt.Sprintf("Failed to retrieve agent: %s", err.Error()),
		Found: false,
	}

	var agentErr *clients.MaestroAgentError
	if errors.As(err, &agentErr) {
		output.ErrorCode = agentErr.ErrorCode
	}

	return output
}
