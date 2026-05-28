package chip_test

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"testing"

	"github.com/collibra/chip/pkg/chip"
	"github.com/collibra/chip/pkg/tools"
	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Validates all tool output schemas against 2020-12 schema
func TestAllToolsDeclareValid2020_12OutputSchemas(t *testing.T) {
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

	for _, tool := range result.Tools {
		t.Run(tool.Name, func(t *testing.T) {
			if tool.OutputSchema == nil {
				t.Fatalf("tool %q has no outputSchema", tool.Name)
			}

			raw, err := json.Marshal(tool.OutputSchema)
			if err != nil {
				t.Fatalf("marshaling outputSchema: %v", err)
			}
			var schema jsonschema.Schema
			if err := json.Unmarshal(raw, &schema); err != nil {
				t.Fatalf("unmarshaling outputSchema: %v", err)
			}

			// Resolve validates the schema against the 2020-12 meta-schema.
			// A non-nil error means the schema is not valid 2020-12.
			if _, err := schema.Resolve(nil); err != nil {
				t.Fatalf("outputSchema is not valid JSON Schema 2020-12: %v", err)
			}
		})
	}
}

// Regression: jsonschema.For emits `type: ["null", T]` for every Go slice
// and pointer. The Claude Code MCP harness only recognizes a singular
// `type` and stringifies the value otherwise, which then fails server-side
// validation. buildSchema must collapse those unions to a single type.
func TestAllToolSchemas_NoNullTypeUnions(t *testing.T) {
	server := chip.NewServer()
	tools.RegisterAll(server, &http.Client{}, &chip.ServerToolConfig{})

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

	for _, tool := range result.Tools {
		t.Run(tool.Name, func(t *testing.T) {
			assertNoNullTypeUnion(t, "input", tool.InputSchema)
			assertNoNullTypeUnion(t, "output", tool.OutputSchema)
		})
	}
}

func assertNoNullTypeUnion(t *testing.T, label string, root any) {
	t.Helper()
	if root == nil {
		return
	}
	raw, err := json.Marshal(root)
	if err != nil {
		t.Fatalf("marshal %s schema: %v", label, err)
	}
	var anyTree any
	if err := json.Unmarshal(raw, &anyTree); err != nil {
		t.Fatalf("unmarshal %s schema: %v", label, err)
	}
	walkSchemaTree(t, label, "", anyTree)
}

func walkSchemaTree(t *testing.T, label, path string, node any) {
	t.Helper()
	switch n := node.(type) {
	case map[string]any:
		if typ, ok := n["type"]; ok {
			if arr, ok := typ.([]any); ok {
				for _, v := range arr {
					if s, _ := v.(string); s == "null" {
						t.Errorf("%s schema at %q has type union containing \"null\": %v — Claude Code harness will stringify this field", label, path, arr)
						break
					}
				}
			}
		}
		for k, v := range n {
			walkSchemaTree(t, label, path+"/"+k, v)
		}
	case []any:
		for i, v := range n {
			walkSchemaTree(t, label, fmt.Sprintf("%s/%d", path, i), v)
		}
	}
}
