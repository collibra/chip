package tools

import (
	"context"
	"net/http"

	"github.com/collibra/chip/pkg/chip"
	"github.com/collibra/chip/pkg/clients"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type AskDadInput struct {
	Question string `json:"input" jsonschema:"the question to ask the data asset discovery agent"`
}

type AskDadOutput struct {
	Answer string `json:"output" jsonschema:"the answer from the data asset discovery agent"`
}

func NewAskDadTool() *chip.CollibraTool[AskDadInput, AskDadOutput] {
	return &chip.CollibraTool[AskDadInput, AskDadOutput]{
		Tool: &mcp.Tool{
			Name:        "askDad",
			Description: "Ask the data asset discovery agent questions about available data assets",
		},
		ToolHandler: handleAskDad,
	}
}

func handleAskDad(ctx context.Context, collibraHttpClient *http.Client, input AskDadInput) (AskDadOutput, error) {
	response, err := clients.AskDad(ctx, collibraHttpClient, input.Question)
	if err != nil {
		return AskDadOutput{}, err
	}

	return AskDadOutput{Answer: response}, nil
}
