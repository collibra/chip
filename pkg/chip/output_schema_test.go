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

// Validates all tool output schemas against 2020-12 schema
func TestAllToolsDeclareValid2020_12OutputSchemas(t *testing.T) {
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
