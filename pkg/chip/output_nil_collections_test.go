package chip

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// buildSchema strips "null" from array/object types (stripNullableTypes) so
// inputs stay portable across MCP clients. That also narrows output schemas, so
// a nil Go slice/map — which marshals to `null` — fails output-schema validation
// with `has type "null", want "array"` (or "object"). normalizeNilCollections
// makes the data conform (nil slice -> [], nil map -> {}) without touching the
// schema. This covers both collection kinds and the omitempty interaction.
func TestNilCollectionOutputValidatesAgainstSchema(t *testing.T) {
	type op struct {
		Status string `json:"status"`
	}
	// Required (non-omitempty) slice + map, plus omitempty variants — the shapes
	// that occur across tool outputs.
	type output struct {
		Status    string            `json:"status"`
		Results   []op              `json:"results"`
		Meta      map[string]string `json:"meta"`
		ExtraList []op              `json:"extraList,omitempty"`
		ExtraMap  map[string]string `json:"extraMap,omitempty"`
	}

	resolved, err := buildSchema[output]().Resolve(nil)
	if err != nil {
		t.Fatalf("resolve schema: %v", err)
	}

	// Zero value: every collection nil, exactly like an error-path early return.
	out := output{Status: "error"}
	normalizeNilCollections(reflect.ValueOf(&out).Elem())

	raw, err := json.Marshal(out)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	// Required collections become []/{}; omitempty ones stay omitted.
	if got, want := string(raw), `{"status":"error","results":[],"meta":{}}`; got != want {
		t.Fatalf("normalized output = %s; want %s", got, want)
	}

	var instance any
	if err := json.Unmarshal(raw, &instance); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if err := resolved.Validate(instance); err != nil {
		t.Fatalf("normalized output failed its own schema: %v\noutput: %s", err, raw)
	}
}

type nilSliceOutput struct {
	Results []string `json:"results" jsonschema:"results list"`
}

// End-to-end guard: a tool handler that returns a nil `results` slice must not
// fail output-schema validation when called through the real server path.
// Before normalizeNilCollections was wired into RegisterTool, this call
// returned isError with `has type "null", want "array"`.
func TestNilSliceToolCallDoesNotFailValidation(t *testing.T) {
	chipServer := NewServer()
	RegisterTool(chipServer, &Tool[toolInput, nilSliceOutput]{
		Name:        "nil_slice_tool",
		Description: "Returns a nil results slice.",
		Handler: func(ctx context.Context, input toolInput) (nilSliceOutput, error) {
			return nilSliceOutput{}, nil // Results left nil
		},
	})
	chipSession := newChipSession(t.Context(), chipServer)
	defer closeSilently(chipSession)

	res, err := chipSession.CallTool(t.Context(), &mcp.CallToolParams{
		Name:      "nil_slice_tool",
		Arguments: map[string]any{"input": "x"},
	})
	if err != nil {
		t.Fatalf("protocol error: %v", err)
	}
	if res.IsError {
		var msg string
		if len(res.Content) > 0 {
			if tc, ok := res.Content[0].(*mcp.TextContent); ok {
				msg = tc.Text
			}
		}
		t.Fatalf("tool call failed output validation for nil slice: %s", msg)
	}
}
