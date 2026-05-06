// Package find_attribute_type looks up a single attribute type by
// publicId or exact name from the cached DGC catalog. Returns a
// resolve_domain-style envelope (match | candidates | notFound).
package find_attribute_type

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/collibra/chip/pkg/chip"
	"github.com/collibra/chip/pkg/clients"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type Input struct {
	PublicID string `json:"publicId,omitempty" jsonschema:"Optional. Attribute type publicId. Exact match. At least one of publicId or name must be provided."`
	Name     string `json:"name,omitempty" jsonschema:"Optional. Attribute type display name. Exact match (case-sensitive). At least one of publicId or name must be provided."`
}

type Output struct {
	Match      *AttributeType  `json:"match,omitempty" jsonschema:"Set when exactly one attribute type matches"`
	Candidates []AttributeType `json:"candidates,omitempty" jsonschema:"Set when multiple attribute types match the criteria; the caller must pick one"`
	NotFound   bool            `json:"notFound,omitempty" jsonschema:"True when no attribute type matches"`
	Reason     string          `json:"reason,omitempty" jsonschema:"Explanation when match is empty (notFound or multi-match)"`
}

type AttributeType struct {
	ID            string   `json:"id" jsonschema:"The unique identifier of the attribute type"`
	Name          string   `json:"name" jsonschema:"The name of the attribute type"`
	Description   string   `json:"description,omitempty" jsonschema:"The description of the attribute type"`
	PublicID      string   `json:"publicId,omitempty" jsonschema:"The public id of the attribute type"`
	Kind          string   `json:"kind,omitempty" jsonschema:"The attributeTypeDiscriminator: e.g. StringAttributeType, BooleanAttributeType, SingleValueListAttributeType"`
	AllowedValues []string `json:"allowedValues,omitempty" jsonschema:"Populated for SingleValueList / MultiValueList kinds; empty otherwise"`
	System        bool     `json:"system" jsonschema:"Whether this is a system attribute type"`
}

func NewTool(collibraClient *http.Client) *chip.Tool[Input, Output] {
	return &chip.Tool[Input, Output]{
		Name:        "find_attribute_type",
		Description: "Find a single attribute type by publicId or exact name. Returns match / candidates / notFound. Reads from the same one-hour cache as list_attribute_types.",
		Handler:     handler(collibraClient),
		Permissions: []string{},
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}
}

func handler(collibraClient *http.Client) chip.ToolHandlerFunc[Input, Output] {
	return func(ctx context.Context, input Input) (Output, error) {
		if input.PublicID == "" && input.Name == "" {
			return Output{}, errors.New("at least one of publicId or name must be provided")
		}
		all, err := clients.ListAllAttributeTypes(ctx, collibraClient)
		if err != nil {
			return Output{}, err
		}
		var matches []AttributeType
		for _, at := range all {
			if input.PublicID != "" && at.PublicID != input.PublicID {
				continue
			}
			if input.Name != "" && at.Name != input.Name {
				continue
			}
			matches = append(matches, AttributeType{
				ID:            at.ID,
				Name:          at.Name,
				Description:   at.Description,
				PublicID:      at.PublicID,
				Kind:          at.AttributeTypeDiscriminator,
				AllowedValues: at.AllowedValues,
				System:        at.System,
			})
		}
		switch len(matches) {
		case 0:
			return Output{NotFound: true, Reason: notFoundReason(input)}, nil
		case 1:
			m := matches[0]
			return Output{Match: &m}, nil
		default:
			return Output{Candidates: matches, Reason: "multiple matches; pick one by id and re-call with the publicId"}, nil
		}
	}
}

func notFoundReason(in Input) string {
	switch {
	case in.PublicID != "" && in.Name != "":
		return fmt.Sprintf("no attribute type matches publicId=%q and name=%q", in.PublicID, in.Name)
	case in.PublicID != "":
		return fmt.Sprintf("no attribute type matches publicId=%q", in.PublicID)
	default:
		return fmt.Sprintf("no attribute type matches name=%q", in.Name)
	}
}