// Package list_capability_types implements the list_capability_types MCP tool: a
// read-only discovery tool for finding valid typeId values (and their manifests) for
// create_capability and create_connection on a given edge site.
package list_capability_types

import (
	"context"
	"net/http"

	"github.com/collibra/chip/pkg/chip"
	"github.com/collibra/chip/pkg/clients"
	"github.com/collibra/chip/pkg/tools/validation"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type Input struct {
	EdgeSiteID string `json:"edgeSiteId" jsonschema:"UUID of the edge site to list available capability and connection types for. Use the list_edge_sites tool to discover available sites."`
}

type Output struct {
	CapabilityTypes []clients.CapabilityType `json:"capabilityTypes" jsonschema:"Capability types available on this edge site (e.g. 'jdbc-ingestion'), with their manifests describing expected install parameters."`
	ConnectionTypes []clients.ConnectionType `json:"connectionTypes" jsonschema:"Connection types available on this edge site (e.g. 'Generic' or a vendor-specific type), with their manifests describing expected parameters."`
}

func NewTool(collibraClient *http.Client) *chip.Tool[Input, Output] {
	return &chip.Tool[Input, Output]{
		Name:        "list_capability_types",
		Title:       "List Capability and Connection Types",
		Description: "Lists the capability and connection types available on an edge site, including each type's manifest. Use this to find the typeId and expected parameters for create_capability and create_connection.",
		Handler:     handler(collibraClient),
		Permissions: []string{},
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}
}

func handler(collibraClient *http.Client) chip.ToolHandlerFunc[Input, Output] {
	return func(ctx context.Context, input Input) (Output, error) {
		if err := validation.UUID("edgeSiteId", input.EdgeSiteID); err != nil {
			return Output{}, err
		}

		capabilityTypes, err := clients.GetCapabilityTypes(ctx, collibraClient, input.EdgeSiteID)
		if err != nil {
			return Output{}, err
		}

		connectionTypes, err := clients.GetConnectionTypes(ctx, collibraClient, input.EdgeSiteID)
		if err != nil {
			return Output{}, err
		}

		return Output{CapabilityTypes: capabilityTypes, ConnectionTypes: connectionTypes}, nil
	}
}
