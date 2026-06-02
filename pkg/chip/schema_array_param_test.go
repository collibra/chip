package chip_test

import (
	"encoding/json"
	"log"
	"net/http"
	"testing"

	"github.com/collibra/chip/pkg/chip"
	"github.com/collibra/chip/pkg/tools"
	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Regression test for DEV-182926. Array-typed parameters were advertised as a
// nullable type union (`["null","array"]`), which the Claude desktop app failed
// to recognise as structured — it serialised the argument to a JSON string that
// the server then rejected with `has type "string", want one of "null, array"`.
// Every registered tool's array/object parameters must advertise a single
// concrete type, never a union containing "null".
func TestRegisteredToolParamsHaveNoNullableUnions(t *testing.T) {
	server := chip.NewServer()
	if err := tools.RegisterAll(server, &http.Client{}, &chip.ServerToolConfig{}); err != nil {
		t.Fatalf("RegisterAll: %v", err)
	}

	t1, t2 := mcp.NewInMemoryTransports()
	if _, err := server.Connect(t.Context(), t1, nil); err != nil {
		log.Fatal(err)
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "v0.0.0"}, nil)
	session, err := client.Connect(t.Context(), t2, nil)
	if err != nil {
		log.Fatal(err)
	}
	defer func() { _ = session.Close() }()

	result, err := session.ListTools(t.Context(), nil)
	if err != nil {
		t.Fatalf("list tools: %v", err)
	}
	if len(result.Tools) == 0 {
		t.Fatal("no tools registered")
	}

	sawEditAssetArray, sawSearchFilterArray := false, false
	for _, tool := range result.Tools {
		if tool.InputSchema == nil {
			continue
		}
		// The MCP SDK exposes InputSchema as `any`; round-trip it through JSON
		// to inspect it as a typed schema — the same view a client parses.
		raw, err := json.Marshal(tool.InputSchema)
		if err != nil {
			t.Fatalf("%s: marshal inputSchema: %v", tool.Name, err)
		}
		var schema jsonschema.Schema
		if err := json.Unmarshal(raw, &schema); err != nil {
			t.Fatalf("%s: unmarshal inputSchema: %v", tool.Name, err)
		}

		for name, prop := range schema.Properties {
			path := tool.Name + "." + name
			assertNoNullUnion(t, path, prop)
			if tool.Name == "edit_asset" && name == "operations" {
				sawEditAssetArray = true
				if prop.Type != "array" {
					t.Errorf("%s: got Type=%q, want array", path, prop.Type)
				}
			}
			if tool.Name == "search_asset_keyword" && name == "assetTypeFilter" {
				sawSearchFilterArray = true
				if prop.Type != "array" {
					t.Errorf("%s: got Type=%q, want array", path, prop.Type)
				}
			}
		}
	}

	// Guard against the assertions silently passing because the fields were
	// renamed or the tools stopped being registered.
	if !sawEditAssetArray {
		t.Error("edit_asset.operations not found — regression guard never ran")
	}
	if !sawSearchFilterArray {
		t.Error("search_asset_keyword.assetTypeFilter not found — regression guard never ran")
	}
}

// assertNoNullUnion fails if the schema (or any nested schema) advertises a
// multi-valued type list — the reflector only ever produces those as a
// `["null", T]` union, which is exactly what broke desktop clients.
func assertNoNullUnion(t *testing.T, path string, s *jsonschema.Schema) {
	t.Helper()
	if s == nil {
		return
	}
	if len(s.Types) > 0 {
		t.Errorf("%s advertises a type union %v; expected a single concrete type", path, s.Types)
	}
	assertNoNullUnion(t, path+".items", s.Items)
	assertNoNullUnion(t, path+".additionalProperties", s.AdditionalProperties)
	for name, child := range s.Properties {
		assertNoNullUnion(t, path+"."+name, child)
	}
}
