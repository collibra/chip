package get_maestro_agent

import (
	"context"
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
	Configuration string `json:"configuration,omitempty" jsonschema:"The agent configuration as a YAML document"`
	Error         string `json:"error,omitempty" jsonschema:"Error message if the configuration could not be retrieved"`
	Found         bool   `json:"found" jsonschema:"Whether the agent was found"`
}

func NewTool(collibraClient *http.Client) *chip.Tool[Input, Output] {
	return &chip.Tool[Input, Output]{
		Name:  "get_maestro_agent",
		Title: "Get Maestro Agent",
		Description: `Read the configuration of an existing AI Maestro agent, returned verbatim as a YAML document. "Agent" here means a Collibra AI Maestro agent — a configured assistant that end users chat with in Collibra Copilot — not the model calling this tool.
					  The YAML contains the agent's handle, name, description, instructions, color, welcome message, sample questions, tool IDs, knowledge base views, sharing configuration, and ownership.
					  Requires the agentId as a UUID. The caller must be an owner of the agent, or hold the permission to manage all AI agents.`,
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

		configuration, err := clients.GetAgentConfigurationFile(ctx, collibraClient, input.AgentID)
		if err != nil {
			return Output{
				Error: fmt.Sprintf("Failed to retrieve agent configuration: %s", err.Error()),
				Found: false,
			}, nil
		}

		return Output{
			Configuration: string(configuration),
			Found:         true,
		}, nil
	}
}
