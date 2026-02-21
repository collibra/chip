package ask_glossary

import (
	"context"
	"net/http"

	"github.com/collibra/chip/pkg/chip"
	"github.com/collibra/chip/pkg/clients"
)

type Input struct {
	Question string `json:"input" jsonschema:"the question to ask the business glossary agent"`
}

type Output struct {
	Answer string `json:"output" jsonschema:"the answer from the business glossary agent"`
}

func NewTool(collibraHttpClient *http.Client) *chip.Tool[Input, Output] {
	return &chip.Tool[Input, Output]{
		Name:        "business_glossary_discover",
		Description: "Ask the business glossary agent questions about terms and definitions in Collibra.",
		Handler:     handler(collibraHttpClient),
		Permissions: []string{"dgc.ai-copilot"},
	}
}

func handler(collibraClient *http.Client) chip.ToolHandlerFunc[Input, Output] {
	return func(ctx context.Context, input Input) (Output, error) {
		response, err := clients.AskGlossary(ctx, collibraClient, input.Question)
		if err != nil {
			return Output{}, err
		}
		return Output{Answer: response}, nil
	}
}
