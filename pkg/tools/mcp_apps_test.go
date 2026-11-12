package tools_test

import (
	"context"
	"encoding/json"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/collibra/chip/pkg/chip"
	"github.com/collibra/chip/pkg/tools"
	"github.com/collibra/chip/pkg/tools/get_asset_details"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const assetCardURI = "ui://collibra/get-asset-details/card"

func TestMCPApps_DisabledLeavesTheRegisteredSurfaceUntouched(t *testing.T) {
	session := registeredSession(t, nil)

	if extensions := session.InitializeResult().Capabilities.Extensions; extensions != nil {
		t.Errorf("expected no extensions capability with the flag off, got %v", extensions)
	}
	if meta := findTool(t, session, "get_asset_details").Meta; meta != nil {
		t.Errorf("expected no _meta on get_asset_details with the flag off, got %v", meta)
	}
	if uris := resourceURIs(t, session); len(uris) != 0 {
		t.Errorf("expected resources/list to be empty with the flag off, got %v", uris)
	}
}

func TestMCPApps_EnabledPointsGetAssetDetailsAtItsCard(t *testing.T) {
	session := registeredSession(t, []string{chip.MCPAppsFeature})

	got := marshal(t, findTool(t, session, "get_asset_details").Meta)
	want := `{"ui":{"resourceUri":"` + assetCardURI + `"}}`
	if got != want {
		t.Errorf("get_asset_details _meta = %s, want %s", got, want)
	}
}

func TestMCPApps_EnabledServesExactlyTheAssetCard(t *testing.T) {
	session := registeredSession(t, []string{chip.MCPAppsFeature})

	resources := listResources(t, session)
	if len(resources) != 1 {
		t.Fatalf("expected exactly one resource, got %d: %v", len(resources), resourceURIs(t, session))
	}
	if resources[0].URI != assetCardURI {
		t.Fatalf("resource URI = %q, want %q", resources[0].URI, assetCardURI)
	}
	if resources[0].MIMEType != chip.UIMimeType {
		t.Errorf("resource mimeType = %q, want %q", resources[0].MIMEType, chip.UIMimeType)
	}

	result, err := session.ReadResource(t.Context(), &mcp.ReadResourceParams{URI: assetCardURI})
	if err != nil {
		t.Fatalf("ReadResource failed: %v", err)
	}
	if len(result.Contents) != 1 || result.Contents[0].Text == "" {
		t.Fatalf("expected one non-empty content entry, got %+v", result.Contents)
	}
	if result.TTLMs != 0 {
		t.Errorf("ttlMs = %d, want 0", result.TTLMs)
	}
}

// The card is a supplied artefact, verified against a real MCP Apps host. These
// two facts are the ones a well-meaning edit breaks: the host rejects a
// ui/initialize that omits appInfo or appCapabilities, and the tool result
// arrives as params.structuredContent, not as the params themselves.
func TestMCPApps_CardSpeaksTheHostHandshake(t *testing.T) {
	session := registeredSession(t, []string{chip.MCPAppsFeature})

	result, err := session.ReadResource(t.Context(), &mcp.ReadResourceParams{URI: assetCardURI})
	if err != nil {
		t.Fatalf("ReadResource failed: %v", err)
	}
	card := result.Contents[0].Text
	for _, fragment := range []string{"appInfo", "appCapabilities", "params?.structuredContent"} {
		if !strings.Contains(card, fragment) {
			t.Errorf("card no longer contains %q", fragment)
		}
	}
}

func TestMCPApps_EnabledAddsNoTools(t *testing.T) {
	off := toolNames(t, registeredSession(t, nil))
	on := toolNames(t, registeredSession(t, []string{chip.MCPAppsFeature}))

	if !slices.Equal(off, on) {
		t.Errorf("tool set changed with mcp-apps on:\n off = %v\n on  = %v", off, on)
	}
}

// The exported URI is what get_asset_details declares; the tests above assert
// the wire carries it, so this keeps the two from drifting.
func TestMCPApps_CardURIMatchesTheToolDeclaration(t *testing.T) {
	if get_asset_details.UIResourceURI != assetCardURI {
		t.Errorf("get_asset_details.UIResourceURI = %q, want %q", get_asset_details.UIResourceURI, assetCardURI)
	}
}

// registeredSession mirrors cmd/chip/main.go: the experimental list decides the
// server options, then every tool is registered on top.
func registeredSession(t *testing.T, experimental []string) *mcp.ClientSession {
	t.Helper()
	cfg := &chip.ServerToolConfig{Experimental: experimental}

	var opts []chip.ServerOption
	if cfg.IsExperimentalEnabled(chip.MCPAppsFeature) {
		opts = append(opts, chip.WithMCPApps())
	}
	server := chip.NewServer(opts...)
	if err := tools.RegisterAll(server, &http.Client{}, cfg); err != nil {
		t.Fatalf("RegisterAll failed: %v", err)
	}

	t1, t2 := mcp.NewInMemoryTransports()
	if _, err := server.Connect(t.Context(), t1, nil); err != nil {
		t.Fatalf("connecting server: %v", err)
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "client", Version: "v0.0.1"}, nil)
	session, err := client.Connect(t.Context(), t2, nil)
	if err != nil {
		t.Fatalf("connecting client: %v", err)
	}
	t.Cleanup(func() { _ = session.Close() })
	return session
}

func findTool(t *testing.T, session *mcp.ClientSession, name string) *mcp.Tool {
	t.Helper()
	for tool, err := range session.Tools(context.Background(), nil) {
		if err != nil {
			t.Fatalf("listing tools: %v", err)
		}
		if tool.Name == name {
			return tool
		}
	}
	t.Fatalf("tool %q not found", name)
	return nil
}

func toolNames(t *testing.T, session *mcp.ClientSession) []string {
	t.Helper()
	var names []string
	for tool, err := range session.Tools(context.Background(), nil) {
		if err != nil {
			t.Fatalf("listing tools: %v", err)
		}
		names = append(names, tool.Name)
	}
	slices.Sort(names)
	return names
}

func resourceURIs(t *testing.T, session *mcp.ClientSession) []string {
	t.Helper()
	var uris []string
	for _, resource := range listResources(t, session) {
		uris = append(uris, resource.URI)
	}
	return uris
}

func listResources(t *testing.T, session *mcp.ClientSession) []*mcp.Resource {
	t.Helper()
	var resources []*mcp.Resource
	for resource, err := range session.Resources(context.Background(), nil) {
		if err != nil {
			t.Fatalf("listing resources: %v", err)
		}
		resources = append(resources, resource)
	}
	return resources
}

func marshal(t *testing.T, v any) string {
	t.Helper()
	encoded, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshalling %v: %v", v, err)
	}
	return string(encoded)
}
