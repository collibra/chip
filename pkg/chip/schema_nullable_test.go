package chip

import "testing"

// nullableProbe exercises every nilable Go kind the reflector marks as a
// `["null", T]` union: slice, map, and pointer, plus a plain scalar control.
type nullableProbe struct {
	List    []string          `json:"list" jsonschema:"a list"`
	Mapping map[string]string `json:"mapping,omitempty" jsonschema:"a map"`
	Ptr     *string           `json:"ptr,omitempty" jsonschema:"a pointer"`
	Scalar  string            `json:"scalar" jsonschema:"a scalar"`
}

func TestBuildSchemaCollapsesNullableUnions(t *testing.T) {
	s := buildSchema[nullableProbe]()

	cases := map[string]string{
		"list":    "array",
		"mapping": "object",
		"ptr":     "string",
		"scalar":  "string",
	}
	for field, want := range cases {
		prop, ok := s.Properties[field]
		if !ok {
			t.Fatalf("property %q missing from generated schema", field)
		}
		if len(prop.Types) != 0 {
			t.Errorf("property %q still carries a type union %v; want collapsed single type %q", field, prop.Types, want)
		}
		if prop.Type != want {
			t.Errorf("property %q: got Type=%q, want %q", field, prop.Type, want)
		}
	}

	// The collapse must recurse into array item schemas too.
	if items := s.Properties["list"].Items; items == nil || items.Type != "string" || len(items.Types) != 0 {
		t.Errorf("list.items: got %+v, want plain string", items)
	}
}
