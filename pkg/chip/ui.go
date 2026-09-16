package chip

import (
	"context"
	"fmt"
	"log"

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
// On what basis the server advertises the extension at all: MCP Apps defines
// `io.modelcontextprotocol/ui`, with its `mimeTypes` settings object, as a
// CLIENT capability — sent on the initialize request to declare what the host
// can render. The draft spec shows no server-side counterpart
// (modelcontextprotocol/ext-apps, specification/draft/apps.mdx, "Client<>Server
// Capability Negotiation"). chip mirrors the identifier and settings object back
// on initialize anyway, as a deliberate signal that this server serves MCP App
// cards. It has NOT been observed to be required by any host — this repo has no
// host to test against — and a host that ignores it loses nothing: what a host
// actually consumes is the tool's `_meta.ui.resourceUri` and the ui:// resource.
//
// That choice is what costs the rest of this function. Setting
// ServerOptions.Capabilities at all is what forces the Logging restatement
// below, and with it the //nolint. Dropping the advertisement would delete
// uiCapabilities entirely and leave the consumed surface unchanged — the first
// thing to revisit when the PRD settles this feature's shape.
//
// Logging is restated deliberately. mcp.ServerOptions.Capabilities REPLACES the
// SDK's defaults rather than merging with them, and the SDK's default is exactly
// `{logging:{}}` (tools/resources are then augmented in on top). Dropping the
// line does not fail anything: the server simply stops advertising logging,
// silently. Guarded on both flag states, by
// TestMCPApps_EnabledPreservesLoggingCapability and
// TestMCPApps_DisabledPreservesLoggingCapability.
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
// so a tool cannot advertise a card the server does not serve — or serve one no
// tool points at. Declaring one of UIResourceURI/UICardHTML without the other is
// a programming error and is fatal at registration; see checkUIDeclaration. A
// tool author declares those two fields and nothing else; they never touch
// resources.
//
// A no-op unless MCP Apps are enabled and the tool declares a URI.
//
// Known limitation — chip does not gate UI metadata per client. The spec asks
// servers to check the connecting client's io.modelcontextprotocol/ui capability
// before registering UI-enabled tools, and to register a text-only variant
// otherwise (ext-apps, apps.mdx, "Server Behavior"). chip cannot do that today:
// tools are registered once at startup and one *mcp.Server is shared by every
// session (cmd/chip/main.go hands &server.Server to both HTTP handler
// factories), so per-client tool variants are not expressible. The experimental
// flag is an operator-level gate in its place. The consequence is that a host
// with no MCP Apps support still receives the `_meta` pointer and still sees the
// ui:// resource in resources/list, and has to ignore both.
func attachUI(s *Server, mcpTool *mcp.Tool, resourceURI, cardHTML string) {
	if !s.uiApps {
		return
	}
	if err := checkUIDeclaration(mcpTool.Name, resourceURI, cardHTML); err != nil {
		log.Fatal(err)
	}
	if resourceURI == "" {
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

// checkUIDeclaration rejects a tool that declares half an MCP App: either both
// UIResourceURI and UICardHTML are set, or neither is. A URI without a card
// serves a zero-byte body that the host renders as a blank pane; a card without
// a URI is dropped and never reaches a host. Both are mistakes in the tool's own
// declaration — a forgotten //go:embed, a renamed variable — and both are
// otherwise silent, so they are fatal where they are made rather than puzzling
// at the host. This matches how buildSchema treats a tool it cannot build a
// schema for.
func checkUIDeclaration(toolName, resourceURI, cardHTML string) error {
	if (resourceURI == "") == (cardHTML == "") {
		return nil
	}
	if resourceURI == "" {
		return fmt.Errorf("tool %q sets UICardHTML but no UIResourceURI: the card would never be served", toolName)
	}
	return fmt.Errorf("tool %q sets UIResourceURI %q but no UICardHTML: the card would be served empty", toolName, resourceURI)
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
