package chip_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/collibra/chip/pkg/chip"
	"github.com/collibra/chip/pkg/tools"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// TestNoToolDeclaresABooleanSubschema keeps every tool free of the degenerate
// schema that an "any" field produces. `true` is a valid JSON Schema, but a
// client that expects an object where a schema belongs rejects it, and one such
// tool can fail the client's whole tool import. A type that needs an "any"
// field must declare its shape through Tool.TypeSchemas instead.
func TestNoToolDeclaresABooleanSubschema(t *testing.T) {
	server := chip.NewServer()
	if err := tools.RegisterAll(server, &http.Client{}, &chip.ServerToolConfig{}); err != nil {
		t.Fatalf("RegisterAll: %v", err)
	}

	t1, t2 := mcp.NewInMemoryTransports()
	if _, err := server.Connect(t.Context(), t1, nil); err != nil {
		t.Fatalf("connect server: %v", err)
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "v0.0.0"}, nil)
	session, err := client.Connect(t.Context(), t2, nil)
	if err != nil {
		t.Fatalf("connect client: %v", err)
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
			for name, schema := range map[string]any{"inputSchema": tool.InputSchema, "outputSchema": tool.OutputSchema} {
				if schema == nil {
					continue
				}
				var found []string
				collectBooleanSubschemas(t, schema, name, &found)
				for _, where := range found {
					t.Errorf("%s declares a boolean subschema at %s", tool.Name, where)
				}
			}
		})
	}
}

func collectBooleanSubschemas(t *testing.T, schema any, path string, found *[]string) {
	t.Helper()
	raw, err := json.Marshal(schema)
	if err != nil {
		t.Fatalf("marshaling %s: %v", path, err)
	}
	var node any
	if err := json.Unmarshal(raw, &node); err != nil {
		t.Fatalf("unmarshaling %s: %v", path, err)
	}
	walkSubschemas(node, path, found)
}

// walkSubschemas visits the positions that hold a schema. It skips
// additionalProperties, where the reflector writes `false` on purpose.
func walkSubschemas(node any, path string, found *[]string) {
	object, ok := node.(map[string]any)
	if !ok {
		return
	}
	visit := func(child any, childPath string) {
		if value, isBool := child.(bool); isBool {
			*found = append(*found, fmt.Sprintf("%s = %v", childPath, value))
			return
		}
		walkSubschemas(child, childPath, found)
	}
	for key, child := range object {
		switch key {
		case "properties", "$defs", "definitions", "patternProperties":
			members, isObject := child.(map[string]any)
			if !isObject {
				continue
			}
			for name, sub := range members {
				visit(sub, path+"."+name)
			}
		case "items", "not", "contains", "propertyNames", "if", "then", "else":
			visit(child, path+"."+key)
		case "oneOf", "anyOf", "allOf", "prefixItems":
			list, isList := child.([]any)
			if !isList {
				continue
			}
			for i, sub := range list {
				visit(sub, fmt.Sprintf("%s.%s[%d]", path, key, i))
			}
		}
	}
}
