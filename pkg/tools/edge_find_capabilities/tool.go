package edge_find_capabilities

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
	EdgeSiteId string            `json:"edgeSiteId,omitempty" jsonschema:"optional UUID of the edge site to filter capabilities by"`
	Labels     map[string]string `json:"labels,omitempty" jsonschema:"optional label key-value pairs to filter by; multiple labels are ANDed"`
	Parameters map[string]string `json:"parameters,omitempty" jsonschema:"optional parameter key-value pairs to filter by (e.g. connection UUID)"`
}

type Output struct {
	Capabilities []clients.EdgeCapability `json:"capabilities" jsonschema:"list of matching capabilities"`
	Count        int                      `json:"count" jsonschema:"number of matching capabilities"`
	Error        string                   `json:"error,omitempty" jsonschema:"error message if the request failed"`
}

func NewTool(collibraClient *http.Client) *chip.Tool[Input, Output] {
	return &chip.Tool[Input, Output]{
		Name:        "edge_find_capabilities",
		Description: "Finds Edge capabilities matching the given criteria. All filter fields are ANDed; map/list field values are ORed. Useful for finding capabilities by edge site or parameter value.",
		Handler:     handler(collibraClient),
		Permissions: []string{},
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}
}

func handler(collibraClient *http.Client) chip.ToolHandlerFunc[Input, Output] {
	return func(ctx context.Context, input Input) (Output, error) {
		if err := validation.UUIDOptional("edgeSiteId", input.EdgeSiteId); err != nil {
			return Output{}, err
		}
		caps, err := clients.FindCapabilities(ctx, collibraClient, clients.CapabilityFindRequest{
			EdgeSiteId: input.EdgeSiteId,
			Labels:     input.Labels,
			Parameters: input.Parameters,
		})
		if err != nil {
			return Output{Capabilities: []clients.EdgeCapability{}, Error: fmt.Sprintf("failed to find capabilities: %s", err.Error())}, nil
		}
		if caps == nil {
			caps = []clients.EdgeCapability{}
		}
		return Output{Capabilities: caps, Count: len(caps)}, nil
	}
}
