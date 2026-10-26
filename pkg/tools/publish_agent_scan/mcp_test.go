package publish_agent_scan_test

import (
	"encoding/json"
	"net/http"
	"reflect"
	"testing"

	"github.com/collibra/chip/pkg/chip"
	"github.com/collibra/chip/pkg/tools/publish_agent_scan"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Calls the tool through a real MCP session so the generated input schema and
// the SDK's decoding of the raw-JSON document field are exercised, not just
// the handler.
func TestPublishAgentScan_ThroughMCPSession(t *testing.T) {
	f := newFakeGuardian(t, http.StatusCreated, `{"id":"scan-1","fingerprintId":"fp-1","createdOn":"c","generatedOn":"g"}`)
	server := chip.NewServer()
	chip.RegisterTool(server, publish_agent_scan.NewTool(f.client))

	t1, t2 := mcp.NewInMemoryTransports()
	if _, err := server.Connect(t.Context(), t1, nil); err != nil {
		t.Fatal(err)
	}
	session, err := mcp.NewClient(&mcp.Implementation{Name: "client", Version: "v0.0.1"}, nil).Connect(t.Context(), t2, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = session.Close() }()

	call := func(confirm bool) *mcp.CallToolResult {
		res, err := session.CallTool(t.Context(), &mcp.CallToolParams{
			Name: "publish_agent_scan",
			Arguments: map[string]any{
				"document":   json.RawMessage(document),
				"externalId": "catalog.schema.support_triage_agent",
				"confirm":    confirm,
			},
		})
		if err != nil {
			t.Fatalf("CallTool: %v", err)
		}
		if res.IsError {
			t.Fatalf("tool returned an error result: %+v", res.Content)
		}
		return res
	}

	call(false)
	if f.calls.Load() != 0 {
		t.Fatalf("preview sent %d request(s)", f.calls.Load())
	}
	res := call(true)
	if f.calls.Load() != 1 {
		t.Fatalf("confirm=true sent %d request(s), want 1", f.calls.Load())
	}
	var sent struct {
		Document json.RawMessage `json:"document"`
	}
	if err := json.Unmarshal(f.lastBody, &sent); err != nil {
		t.Fatal(err)
	}
	// The MCP SDK decodes and re-encodes tool arguments before the handler
	// runs, so key order is not preserved end to end; the content must be.
	var got, want any
	if err := json.Unmarshal(sent.Document, &got); err != nil {
		t.Fatal(err)
	}
	_ = json.Unmarshal([]byte(document), &want)
	if !reflect.DeepEqual(got, want) {
		t.Errorf("document content changed in transit:\n got %s\nwant %s", sent.Document, document)
	}
	structured, _ := json.Marshal(res.StructuredContent)
	if !json.Valid(structured) || len(structured) == 0 {
		t.Error("no structured content")
	}
}
