package chip

import (
	"context"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// MCPAppsFeature is the experimental feature name that turns MCP Apps on.
// While it is off, chip's wire surface is byte-for-byte what it was before:
// no extensions capability, no `_meta` on any tool, no UI resources.
const MCPAppsFeature = "mcp-apps"

// UIExtension is the MCP capability extension identifier for MCP Apps. A host
// that understands it reads a tool's `_meta.ui.resourceUri`, fetches that
// resource, and renders it alongside the tool's result.
const UIExtension = "io.modelcontextprotocol/ui"

// UIMimeType is the MIME type an MCP App is served with. It is both what the
// server advertises under the extension and what the card resource reports.
const UIMimeType = "text/html;profile=mcp-app"

// WithMCPApps enables MCP Apps for this server: the UI extension is advertised
// on initialize, and every tool that declares a UIResourceURI also gets its
// card registered as a resource. Off by default — see MCPAppsFeature.
func WithMCPApps() ServerOption {
	return func(s *Server) {
		s.uiApps = true
	}
}

// uiCapabilities returns the capabilities to hand the SDK, or nil to leave the
// SDK's own defaults untouched when MCP Apps are off.
//
// Logging is restated here deliberately. mcp.ServerOptions.Capabilities
// REPLACES the SDK's defaults rather than merging with them, and the SDK's
// default is exactly `{logging:{}}` (tools/resources are then augmented in on
// top). Dropping the line does not fail anything: the server simply stops
// advertising logging, silently. See TestMCPApps_EnabledPreservesLoggingCapability.
func uiCapabilities(enabled bool) *mcp.ServerCapabilities {
	if !enabled {
		return nil
	}
	return &mcp.ServerCapabilities{
		// Deprecated in the SDK, but still what it defaults to and still
		// functional. Drop the line when the SDK stops defaulting to it.
		Logging: &mcp.LoggingCapabilities{}, //nolint:staticcheck // SA1019: see above.
		Extensions: map[string]any{
			UIExtension: map[string]any{"mimeTypes": []string{UIMimeType}},
		},
	}
}

// attachUI wires up both halves of a tool's MCP App in one place: the `_meta`
// pointer a host reads off the tool, and the resource that serves the card at
// the URI that pointer names. They are written together, under one condition,
// so a tool can never advertise a card the server does not serve — or serve one
// no tool points at. A tool author only declares UIResourceURI and UICardHTML;
// they never touch resources.
//
// A no-op unless MCP Apps are enabled and the tool declares a URI.
func attachUI(s *Server, mcpTool *mcp.Tool, resourceURI, cardHTML string) {
	if !s.uiApps || resourceURI == "" {
		return
	}
	mcpTool.Meta = mcp.Meta{"ui": map[string]any{"resourceUri": resourceURI}}
	s.AddResource(&mcp.Resource{
		Name:        mcpTool.Name + "_card",
		Title:       mcpTool.Title,
		Description: fmt.Sprintf("Read-only HTML card an MCP Apps host renders for the %s tool.", mcpTool.Name),
		MIMEType:    UIMimeType,
		URI:         resourceURI,
	}, serveCard(resourceURI, cardHTML))
}

// serveCard returns the resource handler for a single static card. The card is
// embedded in the binary, so reading it cannot fail and needs no caching hint —
// the result carries the SDK's zero ttlMs.
func serveCard(resourceURI, cardHTML string) mcp.ResourceHandler {
	return func(context.Context, *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		return &mcp.ReadResourceResult{
			Contents: []*mcp.ResourceContents{{
				URI:      resourceURI,
				MIMEType: UIMimeType,
				Text:     cardHTML,
			}},
		}, nil
	}
}
