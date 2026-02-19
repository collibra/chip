package tools

import (
	"context"
	"net/http"

	"github.com/collibra/chip/pkg/chip"
	"github.com/collibra/chip/pkg/clients"
)

type AskDadInput struct {
	Question string `json:"input" jsonschema:"the question to ask the data asset discovery agent"`
}

type AskDadOutput struct {
	Answer string `json:"output" jsonschema:"the answer from the data asset discovery agent"`
}

func NewAskDadTool(collibraClient *http.Client) *chip.Tool[AskDadInput, AskDadOutput] {
	return &chip.Tool[AskDadInput, AskDadOutput]{
		Name:        "data_assets_discover",
		Description: "Perform a semantic search across available data assets in Collibra. Ask natural language questions to discover tables, columns, datasets, and other data assets.",
		Handler:     handleAskDad(collibraClient),
		Permissions: []string{"dgc.ai-copilot"},
	}
}

func handleAskDad(collibraClient *http.Client) chip.ToolHandlerFunc[AskDadInput, AskDadOutput] {
	return func(ctx context.Context, input AskDadInput) (AskDadOutput, error) {
		response, err := clients.AskDad(ctx, collibraClient, input.Question)
		if err != nil {
			return AskDadOutput{}, err
		}

		return AskDadOutput{Answer: response}, nil
	}
}
