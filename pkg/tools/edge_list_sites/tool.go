// Package edge_list_sites implements the edge_list_sites MCP tool: a read-only
// discovery tool for finding the edgeSiteId needed by edge_create_connection and
// edge_create_capability.
package edge_list_sites

import (
	"context"
	"net/http"

	"github.com/collibra/chip/pkg/chip"
	"github.com/collibra/chip/pkg/clients"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type Input struct{}

type Output struct {
	Sites []clients.EdgeSite `json:"sites" jsonschema:"The list of available edge sites."`
}

func NewTool(collibraClient *http.Client) *chip.Tool[Input, Output] {
	return &chip.Tool[Input, Output]{
		Name:        "edge_list_sites",
		Title:       "List Edge Sites",
		Description: "Lists available Edge sites, including their id, name, and status. Use this to find the edgeSiteId needed by edge_create_connection and edge_create_capability.",
		Handler:     handler(collibraClient),
		Permissions: []string{"dgc.edge-view-connections-and-capabilities"},
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}
}

func handler(collibraClient *http.Client) chip.ToolHandlerFunc[Input, Output] {
	return func(ctx context.Context, _ Input) (Output, error) {
		sites, err := clients.GetEdgeSites(ctx, collibraClient)
		if err != nil {
			return Output{}, err
		}
		return Output{Sites: sites}, nil
	}
}
