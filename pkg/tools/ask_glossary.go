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

func NewAskGlossaryTool(collibraHttpClient *http.Client) *chip.Tool[AskGlossaryInput, AskGlossaryOutput] {
	return &chip.Tool[AskGlossaryInput, AskGlossaryOutput]{
		Tool: &mcp.Tool{
			Name:        "business_glossary_discover",
			Description: "Ask the business glossary agent questions about terms and definitions in Collibra.",
		},
		ToolHandler: handleAskGlossary(collibraHttpClient),
	}
}

func handleAskGlossary(collibraClient *http.Client) chip.ToolHandlerFunc[AskGlossaryInput, AskGlossaryOutput] {
	return func(ctx context.Context, input AskGlossaryInput) (AskGlossaryOutput, error) {
		response, err := clients.AskGlossary(ctx, collibraClient, input.Question)
		if err != nil {
			return AskGlossaryOutput{}, err
		}
		return AskGlossaryOutput{Answer: response}, nil
	}
}
