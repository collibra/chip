package chip

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"strings"
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

// The flag-off path preserves logging by returning nil from uiCapabilities and
// letting the SDK's own default stand. Returning an empty ServerCapabilities
// instead reads as equivalent and is not: it replaces the default, and logging
// silently disappears from the DEFAULT server. Guarded here for that reason.
func TestMCPApps_DisabledPreservesLoggingCapability(t *testing.T) {
	session := uiSession(t, newUITool())

	if !advertisesLogging(t, session) {
		t.Error("expected the SDK's default logging capability with the flag off")
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

	if !advertisesLogging(t, session) {
		t.Error("expected logging to survive setting Capabilities")
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

func TestMCPApps_HalfDeclaredCardIsRejected(t *testing.T) {
	cases := []struct {
		name        string
		resourceURI string
		cardHTML    string
		wantErr     string
	}{
		{name: "neither", wantErr: ""},
		{name: "both", resourceURI: testCardURI, cardHTML: testCardHTML, wantErr: ""},
		{name: "uri without card", resourceURI: testCardURI, wantErr: "no UICardHTML"},
		{name: "card without uri", cardHTML: testCardHTML, wantErr: "no UIResourceURI"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := checkUIDeclaration("the_tool", tc.resourceURI, tc.cardHTML)
			switch {
			case tc.wantErr == "" && err != nil:
				t.Fatalf("expected no error, got %v", err)
			case tc.wantErr != "" && err == nil:
				t.Fatalf("expected an error mentioning %q, got nil", tc.wantErr)
			case tc.wantErr != "" && !strings.Contains(err.Error(), tc.wantErr):
				t.Fatalf("error = %v, want it to mention %q", err, tc.wantErr)
			}
		})
	}
}

// checkUIDeclaration is only worth anything if RegisterTool actually acts on
// it, and log.Fatal cannot be observed in-process — so this re-runs itself as a
// subprocess and asserts the registration dies there, before any resource with
// an empty body can be registered.
func TestMCPApps_HalfDeclaredCardIsFatalAtRegistration(t *testing.T) {
	if mode := os.Getenv(halfDeclaredCaseEnv); mode != "" {
		registerHalfDeclaredTool(mode)
		return
	}
	cases := []struct{ name, mode, want string }{
		{"uri without card", "uri-only", "no UICardHTML"},
		{"card without uri", "card-only", "no UIResourceURI"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cmd := exec.Command(os.Args[0], "-test.run=TestMCPApps_HalfDeclaredCardIsFatalAtRegistration")
			cmd.Env = append(os.Environ(), halfDeclaredCaseEnv+"="+tc.mode)
			out, err := cmd.CombinedOutput()
			if err == nil {
				t.Fatalf("expected registration to exit non-zero, it succeeded: %s", out)
			}
			if !strings.Contains(string(out), tc.want) {
				t.Errorf("expected the failure to mention %q, got: %s", tc.want, out)
			}
		})
	}
}

const halfDeclaredCaseEnv = "CHIP_TEST_HALF_DECLARED_CARD"

// registerHalfDeclaredTool runs in the subprocess spawned above and is expected
// to terminate it.
func registerHalfDeclaredTool(mode string) {
	tool := newTool()
	switch mode {
	case "uri-only":
		tool.UIResourceURI = testCardURI
	case "card-only":
		tool.UICardHTML = testCardHTML
	}
	RegisterTool(NewServer(WithMCPApps()), tool)
}

// advertisesLogging reports whether the initialize response carries a logging
// capability. Read off the wire form rather than the (deprecated) typed field:
// what matters is the JSON the client is handed.
func advertisesLogging(t *testing.T, session *mcp.ClientSession) bool {
	t.Helper()
	encoded := mustJSON(t, session.InitializeResult().Capabilities)
	advertised := map[string]json.RawMessage{}
	if err := json.Unmarshal([]byte(encoded), &advertised); err != nil {
		t.Fatalf("decoding advertised capabilities: %v", err)
	}
	if _, ok := advertised["logging"]; ok {
		return true
	}
	t.Logf("advertised capabilities: %s", encoded)
	return false
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
