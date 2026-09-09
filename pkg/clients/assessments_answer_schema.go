package clients

import (
	"reflect"

	"github.com/google/jsonschema-go/jsonschema"
)

// AssessmentTypeSchemas gives the schema overrides that a tool which returns an
// Assessment must apply. Pass it as Tool.TypeSchemas.
func AssessmentTypeSchemas() map[reflect.Type]*jsonschema.Schema {
	return map[reflect.Type]*jsonschema.Schema{
		reflect.TypeFor[Answer](): answerSchema(),
	}
}

// answerSchema describes Answer the way the Assessments API defines it: one
// variant per answer type, where "type" selects the shape of "value".
//
// Reflection cannot derive this. Value must stay an "any" to carry all ten
// shapes, and the reflector renders "any" as the boolean schema "true". Strict
// MCP clients reject a boolean where a schema object belongs, and one such tool
// can fail a client's whole tool import.
//
// Type is "object" because the reflector prepends "null" to an override that has
// no type, which gives a schema that accepts null only. With a type present, the
// nullable *Answer field becomes ["null","object"], which stripNullableTypes
// then collapses.
//
// Only "type" is required. Value has "omitempty", so a false, zero or empty
// answer marshals without the "value" property. Variants also permit unknown
// properties, so a new field in a response cannot fail every variant at once.
func answerSchema() *jsonschema.Schema {
	text := &jsonschema.Schema{Type: "string"}
	return &jsonschema.Schema{
		Type:        "object",
		Description: `the current answer. "type" selects the shape of "value": a string for TEXT, HTML, EXPRESSION and DATE, a number for NUMBER, a boolean for BOOLEAN, and a list of objects for ITEMS, ASSETS, USERORGROUPS and ATTACHMENTS.`,
		OneOf: []*jsonschema.Schema{
			answerVariant("TEXT", text),
			answerVariant("HTML", text),
			answerVariant("EXPRESSION", text),
			answerVariant("DATE", &jsonschema.Schema{Type: "string", Format: "date"}),
			answerVariant("NUMBER", &jsonschema.Schema{Type: "number"}),
			answerVariant("BOOLEAN", &jsonschema.Schema{Type: "boolean"}),
			answerVariant("ITEMS", answerList("value")),
			answerVariant("ASSETS", answerList()),
			answerVariant("USERORGROUPS", answerList("type")),
			answerVariant("ATTACHMENTS", answerList("fileName", "createdBy", "createdOn")),
		},
	}
}

func answerVariant(answerType string, value *jsonschema.Schema) *jsonschema.Schema {
	return &jsonschema.Schema{
		Type: "object",
		Properties: map[string]*jsonschema.Schema{
			"type":  {Type: "string", Enum: []any{answerType}},
			"value": value,
		},
		Required: []string{"type"},
	}
}

// answerList describes an array answer. Every entry carries an "id" plus the
// given string properties.
func answerList(properties ...string) *jsonschema.Schema {
	props := map[string]*jsonschema.Schema{"id": {Type: "string"}}
	for _, name := range properties {
		props[name] = &jsonschema.Schema{Type: "string"}
	}
	return &jsonschema.Schema{
		Type: "array",
		Items: &jsonschema.Schema{
			Type:       "object",
			Properties: props,
			Required:   []string{"id"},
		},
	}
}
