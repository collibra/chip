package finalize_agent_scan_test

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/collibra/chip/pkg/chip"
	"github.com/collibra/chip/pkg/tools/finalize_agent_scan"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Calls the tool through a real MCP session: the SDK decodes and re-encodes the
// arguments before the handler runs, so this checks the hashes survive that.
func TestFinalizeAgentScan_ThroughMCPSession(t *testing.T) {
	server := chip.NewServer()
	chip.RegisterTool(server, finalize_agent_scan.NewTool())

	t1, t2 := mcp.NewInMemoryTransports()
	if _, err := server.Connect(t.Context(), t1, nil); err != nil {
		t.Fatal(err)
	}
	session, err := mcp.NewClient(&mcp.Implementation{Name: "client", Version: "v0.0.1"}, nil).Connect(t.Context(), t2, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = session.Close() }()

	extraction, err := os.ReadFile("testdata/v4-extraction.json")
	if err != nil {
		t.Fatal(err)
	}
	res, err := session.CallTool(t.Context(), &mcp.CallToolParams{
		Name:      "finalize_agent_scan",
		Arguments: map[string]any{"extraction": json.RawMessage(extraction)},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if res.IsError {
		t.Fatalf("tool returned an error result: %+v", res.Content)
	}
	structured, _ := json.Marshal(res.StructuredContent)
	var out finalize_agent_scan.Output
	if err := json.Unmarshal(structured, &out); err != nil {
		t.Fatal(err)
	}
	if out.Status != finalize_agent_scan.StatusSuccess {
		t.Fatalf("status %s, problems %v", out.Status, out.Problems)
	}
	if want := "c978f550b9892199d07d51fc14a323bc2d8c08ccb69d94314d7c264f9bfbf32f"; out.ContentHash != want {
		t.Errorf("V4 document hash through MCP %s, want %s", out.ContentHash, want)
	}
}
