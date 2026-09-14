package chip

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	testCardURI  = "ui://collibra/the-tool/card"
	testCardHTML = "<!doctype html><title>card</title>"
)

func newUITool() *Tool[toolInput, toolOutput] {
	tool := newTool()
	tool.UIResourceURI = testCardURI
	tool.UICardHTML = testCardHTML
	return tool
}

func TestMCPApps_DisabledAdvertisesNoExtension(t *testing.T) {
	session := uiSession(t, newUITool())

	caps := session.InitializeResult().Capabilities
	if caps == nil {
		t.Fatal("expected non-nil server capabilities")
	}
	if caps.Extensions != nil {
		t.Errorf("expected no extensions capability with the flag off, got %v", caps.Extensions)
	}
}

func TestMCPApps_DisabledLeavesToolMetaUnset(t *testing.T) {
	session := uiSession(t, newUITool())

	if meta := listTool(t, session, "the_tool").Meta; meta != nil {
		t.Errorf("expected no _meta on the tool with the flag off, got %v", meta)
	}
}

func TestMCPApps_DisabledRegistersNoResources(t *testing.T) {
	session := uiSession(t, newUITool())

	if uris := listResourceURIs(t, session); len(uris) != 0 {
		t.Errorf("expected resources/list to be empty with the flag off, got %v", uris)
	}
}

func TestMCPApps_EnabledAdvertisesUIExtension(t *testing.T) {
	session := uiSession(t, newUITool(), WithMCPApps())

	extensions := session.InitializeResult().Capabilities.Extensions
	settings, ok := extensions[UIExtension]
	if !ok {
		t.Fatalf("expected extension %q to be advertised, got %v", UIExtension, extensions)
	}
	if got, want := mustJSON(t, settings), `{"mimeTypes":["`+UIMimeType+`"]}`; got != want {
		t.Errorf("extension settings = %s, want %s", got, want)
	}
}

// The SDK REPLACES its default capabilities with the ones we pass rather than
// merging, and its default is logging. Omitting Logging from uiCapabilities
// therefore drops the capability silently, with nothing else failing.
func TestMCPApps_EnabledPreservesLoggingCapability(t *testing.T) {
	session := uiSession(t, newUITool(), WithMCPApps())

	// Asserted on the wire form rather than the (deprecated) typed field: what
	// matters is the JSON the client is handed on initialize.
	encoded := mustJSON(t, session.InitializeResult().Capabilities)
	advertised := map[string]json.RawMessage{}
	if err := json.Unmarshal([]byte(encoded), &advertised); err != nil {
		t.Fatalf("decoding advertised capabilities: %v", err)
	}
	if _, ok := advertised["logging"]; !ok {
		t.Errorf("expected logging to survive setting Capabilities, advertised %s", encoded)
	}
}

func TestMCPApps_EnabledSetsToolUIMeta(t *testing.T) {
	session := uiSession(t, newUITool(), WithMCPApps())

	got := mustJSON(t, listTool(t, session, "the_tool").Meta)
	want := `{"ui":{"resourceUri":"` + testCardURI + `"}}`
	if got != want {
		t.Errorf("tool _meta = %s, want %s", got, want)
	}
}

func TestMCPApps_EnabledRegistersCardResource(t *testing.T) {
	session := uiSession(t, newUITool(), WithMCPApps())

	resources := listResources(t, session)
	if len(resources) != 1 {
		t.Fatalf("expected exactly one resource, got %d: %v", len(resources), listResourceURIs(t, session))
	}
	if resources[0].URI != testCardURI {
		t.Errorf("resource URI = %q, want %q", resources[0].URI, testCardURI)
	}
	if resources[0].MIMEType != UIMimeType {
		t.Errorf("resource mimeType = %q, want %q", resources[0].MIMEType, UIMimeType)
	}
}

func TestMCPApps_EnabledServesCardAtItsURI(t *testing.T) {
	session := uiSession(t, newUITool(), WithMCPApps())

	result, err := session.ReadResource(t.Context(), &mcp.ReadResourceParams{URI: testCardURI})
	if err != nil {
		t.Fatalf("ReadResource failed: %v", err)
	}
	if len(result.Contents) != 1 {
		t.Fatalf("expected exactly one content entry, got %d", len(result.Contents))
	}
	if result.Contents[0].Text != testCardHTML {
		t.Errorf("card body = %q, want %q", result.Contents[0].Text, testCardHTML)
	}
	if result.Contents[0].MIMEType != UIMimeType {
		t.Errorf("card mimeType = %q, want %q", result.Contents[0].MIMEType, UIMimeType)
	}
}

// The card is embedded in the binary and never changes for the life of the
// process, so it carries no caching hint of its own.
func TestMCPApps_CardReadHasZeroTTL(t *testing.T) {
	session := uiSession(t, newUITool(), WithMCPApps())

	result, err := session.ReadResource(t.Context(), &mcp.ReadResourceParams{URI: testCardURI})
	if err != nil {
		t.Fatalf("ReadResource failed: %v", err)
	}
	if result.TTLMs != 0 {
		t.Errorf("ttlMs = %d, want 0", result.TTLMs)
	}
}

func TestMCPApps_EnabledIgnoresToolWithoutUIResourceURI(t *testing.T) {
	session := uiSession(t, newTool(), WithMCPApps())

	if meta := listTool(t, session, "the_tool").Meta; meta != nil {
		t.Errorf("expected no _meta on a tool that declares no UI, got %v", meta)
	}
	if uris := listResourceURIs(t, session); len(uris) != 0 {
		t.Errorf("expected no resources for a tool that declares no UI, got %v", uris)
	}
}

func uiSession(t *testing.T, tool *Tool[toolInput, toolOutput], opts ...ServerOption) *mcp.ClientSession {
	t.Helper()
	chipServer := NewServer(opts...)
	RegisterTool(chipServer, tool)
	session := newChipSession(t.Context(), chipServer)
	t.Cleanup(func() { closeSilently(session) })
	return session
}

func listTool(t *testing.T, session *mcp.ClientSession, name string) *mcp.Tool {
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

func listResourceURIs(t *testing.T, session *mcp.ClientSession) []string {
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

func mustJSON(t *testing.T, v any) string {
	t.Helper()
	encoded, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshalling %v: %v", v, err)
	}
	return string(encoded)
}
