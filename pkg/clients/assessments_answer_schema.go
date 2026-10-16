package clients

import (
	"reflect"

	"github.com/google/jsonschema-go/jsonschema"
)

func AssessmentTypeSchemas() map[reflect.Type]*jsonschema.Schema {
	return map[reflect.Type]*jsonschema.Schema{
		reflect.TypeFor[Answer](): answerSchema(),
	}
}

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
