package discover_data_assets

import (
	"context"
	"net/http"

	"github.com/collibra/chip/pkg/chip"
	"github.com/collibra/chip/pkg/clients"
)

type Input struct {
	Question string `json:"input" jsonschema:"the question to ask the data asset discovery agent"`
}

type Output struct {
	Answer string `json:"output" jsonschema:"the answer from the data asset discovery agent"`
}

func NewTool(collibraClient *http.Client) *chip.Tool[Input, Output] {
	return &chip.Tool[Input, Output]{
		Name:        "discover_data_assets",
		Description: "Perform a semantic search across available data assets in Collibra. Ask natural language questions to discover tables, columns, datasets, and other data assets.",
		Handler:     handler(collibraClient),
		Permissions: []string{"dgc.ai-copilot"},
	}
}

func handler(collibraClient *http.Client) chip.ToolHandlerFunc[Input, Output] {
	return func(ctx context.Context, input Input) (Output, error) {
		response, err := clients.AskDad(ctx, collibraClient, input.Question)
		if err != nil {
			return Output{}, err
		}

		return Output{Answer: response}, nil
	}
}
