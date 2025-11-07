package tools

import (
	"context"
	"net/http"

	"github.com/collibra/chip/pkg/chip"
	"github.com/collibra/chip/pkg/clients"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type AskGlossaryInput struct {
	Question string `json:"input" jsonschema:"the question to ask the business glossary agent"`
}

type AskGlossaryOutput struct {
	Answer string `json:"output" jsonschema:"the answer from the business glossary agent"`
}

func NewAskGlossaryTool() *chip.CollibraTool[AskGlossaryInput, AskGlossaryOutput] {
	return &chip.CollibraTool[AskGlossaryInput, AskGlossaryOutput]{
		Tool: &mcp.Tool{
			Name:        "askGlossary",
			Description: "Ask the business glossary agent questions about terms and definitions",
		},
		ToolHandler: handleAskGlossary,
	}
}

func handleAskGlossary(ctx context.Context, collibraHttpClient *http.Client, input AskGlossaryInput) (AskGlossaryOutput, error) {
	response, err := clients.AskGlossary(ctx, collibraHttpClient, input.Question)
	if err != nil {
		return AskGlossaryOutput{}, err
	}

	return AskGlossaryOutput{Answer: response}, nil
}
