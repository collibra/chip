package list_managed_control_attributes

import (
	"context"
	"net/http"

	"github.com/collibra/chip/pkg/chip"
	"github.com/collibra/chip/pkg/clients"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type Input struct {
	// No input — the tool always returns the full ManagedControl attribute set.
}

type Output struct {
	Attributes map[string]Attribute `json:"attributes" jsonschema:"AttributeTypes assigned to the ManagedControl asset type, keyed by publicId. The save-time control fields (Severity, ControlType, ControlCategory) draw their candidate values from .allowedValues here — the OAS-documented enums are not stable, so this live response is the source of truth."`
}

type Attribute struct {
	ID            string   `json:"id" jsonschema:"AttributeType UUID"`
	Name          string   `json:"name" jsonschema:"AttributeType display name"`
	PublicID      string   `json:"publicId" jsonschema:"AttributeType publicId (key)"`
	Kind          string   `json:"kind" jsonschema:"e.g. SingleValueListAttributeType, StringAttributeType, BooleanAttributeType, NumericAttributeType, DateAttributeType"`
	AllowedValues []string `json:"allowedValues" jsonschema:"Populated for SingleValueList / MultiValueList kinds; empty otherwise"`
}

func NewTool(collibraClient *http.Client) *chip.Tool[Input, Output] {
	return &chip.Tool[Input, Output]{
		Name:        "list_managed_control_attributes",
		Description: "Return the AttributeTypes assigned to the ManagedControl asset type, with allowed values. Used by the create-control save flow to populate the Severity, ControlType, and ControlCategory candidate values without hardcoding the OAS enums.",
		Handler:     handler(collibraClient),
		Permissions: []string{},
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}
}

func handler(collibraClient *http.Client) chip.ToolHandlerFunc[Input, Output] {
	return func(ctx context.Context, _ Input) (Output, error) {
		raw, err := clients.GetManagedControlAttributes(ctx, collibraClient)
		if err != nil {
			return Output{}, err
		}
		out := Output{Attributes: make(map[string]Attribute, len(raw))}
		for k, v := range raw {
			out.Attributes[k] = Attribute{
				ID:            v.ID,
				Name:          v.Name,
				PublicID:      v.PublicID,
				Kind:          v.Kind,
				AllowedValues: v.AllowedValues,
			}
		}
		return out, nil
	}
}
