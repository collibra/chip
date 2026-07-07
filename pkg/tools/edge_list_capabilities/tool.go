package edge_list_capabilities

import (
	"context"
	"net/http"
	"strings"

	"github.com/collibra/chip/pkg/chip"
	"github.com/collibra/chip/pkg/clients"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type Input struct {
	NameContains string `json:"nameContains,omitempty" jsonschema:"optional substring to filter capability names (case-insensitive)"`
	TypeId       string `json:"typeId,omitempty" jsonschema:"optional capability type ID to filter by, e.g. databricks-edge-capability"`
	Limit        int    `json:"limit,omitempty" jsonschema:"optional maximum number of results to return"`
}

type Output struct {
	Capabilities []clients.EdgeCapability `json:"capabilities" jsonschema:"list of Edge capabilities"`
	Count        int                  `json:"count" jsonschema:"number of capabilities returned"`
	Error        string               `json:"error,omitempty" jsonschema:"error message if the request failed"`
}

func NewTool(collibraClient *http.Client) *chip.Tool[Input, Output] {
	return &chip.Tool[Input, Output]{
		Name:        "edge_list_capabilities",
		Description: "Gets Edge capabilities with optional filtering by name (substring) and/or type ID. Use nameContains and typeId to avoid fetching the full list, which can be very large.",
		Handler:     handler(collibraClient),
		Permissions: []string{},
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}
}

func handler(collibraClient *http.Client) chip.ToolHandlerFunc[Input, Output] {
	return func(ctx context.Context, input Input) (Output, error) {
		caps, err := clients.ListCapabilities(ctx, collibraClient)
		if err != nil {
			return Output{Capabilities: []clients.EdgeCapability{}, Error: err.Error()}, nil
		}
		if caps == nil {
			caps = []clients.EdgeCapability{}
		}

		filtered := filter(caps, input)
		return Output{Capabilities: filtered, Count: len(filtered)}, nil
	}
}

func filter(caps []clients.EdgeCapability, input Input) []clients.EdgeCapability {
	result := make([]clients.EdgeCapability, 0, len(caps))
	nameFilter := strings.ToLower(input.NameContains)
	typeFilter := strings.ToLower(input.TypeId)

	for _, c := range caps {
		if nameFilter != "" && !strings.Contains(strings.ToLower(c.Name), nameFilter) {
			continue
		}
		if typeFilter != "" {
			typeId := ""
			if c.Type != nil {
				typeId = strings.ToLower(c.Type.Id)
			}
			if typeId != typeFilter {
				continue
			}
		}
		result = append(result, c)
		if input.Limit > 0 && len(result) >= input.Limit {
			break
		}
	}
	return result
}
