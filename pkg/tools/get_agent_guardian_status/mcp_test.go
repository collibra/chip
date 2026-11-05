package get_agent_guardian_status_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/collibra/chip/pkg/chip"
	"github.com/collibra/chip/pkg/tools/get_agent_guardian_status"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Calls the tool through a real MCP session, so the SDK validates the output
// against the generated output schema, which the handler tests skip.
func TestGetAgentGuardianStatus_ThroughMCPSession(t *testing.T) {
	stale := strings.Replace(overview, `["CONTRACT_VERSION_CHANGED"]`, `["FINGERPRINT_CHANGED"]`, 1)
	f := newFakeGuardian(t, http.StatusOK, stale)
	server := chip.NewServer()
	chip.RegisterTool(server, get_agent_guardian_status.NewTool(f.client))

	t1, t2 := mcp.NewInMemoryTransports()
	if _, err := server.Connect(t.Context(), t1, nil); err != nil {
		t.Fatal(err)
	}
	session, err := mcp.NewClient(&mcp.Implementation{Name: "client", Version: "v0.0.1"}, nil).Connect(t.Context(), t2, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = session.Close() }()

	tools, err := session.ListTools(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	definition, _ := json.Marshal(tools.Tools)
	if strings.Contains(strings.ToLower(string(definition)), "fingerprint") {
		t.Errorf("the tool definition must use the tool's own names: %s", definition)
	}

	res, err := session.CallTool(t.Context(), &mcp.CallToolParams{
		Name:      "get_agent_guardian_status",
		Arguments: map[string]any{"aiAgentId": aiAgentID},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if res.IsError {
		t.Fatalf("tool returned an error result: %+v", res.Content)
	}
	result, _ := json.Marshal(res.StructuredContent)
	if strings.Contains(strings.ToLower(string(result)), "fingerprint") {
		t.Errorf("the result must use the tool's own names: %s", result)
	}
	for _, want := range []string{`"scan":`, `"SCAN_CHANGED"`, `"agentScanStateId":`} {
		if !strings.Contains(string(result), want) {
			t.Errorf("%s missing from the result: %s", want, result)
		}
	}
}
